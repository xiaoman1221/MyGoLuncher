package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/lan"
	"MyMinecraftGoLuncher/internal/lansync"
)

type lanShareState struct {
	mu         sync.Mutex
	server     *lansync.Server
	instanceID string
	dirs       []string
}

func (a *App) defaultLanInstanceID() string {
	if a.store == nil {
		return ""
	}
	if instances := a.store.Instances(); len(instances) > 0 {
		return instances[0].ID
	}
	return ""
}

// applyLanShareIdentity keeps an active share in the presence packet even if
// another caller refreshes the advertised identity.
func (a *App) applyLanShareIdentity(identity lan.Identity) lan.Identity {
	a.lanShare.mu.Lock()
	defer a.lanShare.mu.Unlock()
	if a.lanShare.server == nil || a.lanShare.instanceID == "" {
		return identity
	}
	shared := a.lanIdentity(a.lanShare.instanceID)
	shared.SyncPort = a.lanShare.server.Port()
	shared.SyncToken = a.lanShare.server.Token()
	shared.SyncDirs = append([]string(nil), a.lanShare.dirs...)
	return shared
}

func (a *App) updateLanIdentity(identity lan.Identity) error {
	if a.lanSvc == nil {
		return errors.New("局域网发现尚未初始化")
	}
	if snapshot := a.lanSvc.Snapshot(); snapshot.Running {
		a.lanSvc.SetIdentity(identity)
		return nil
	}
	return a.lanSvc.Start(a.context(), lan.Config{
		Broadcast: true,
		Identity:  identity,
		OnChange:  a.handleLANChange,
	})
}

// StartLanShare shares the selected local instance directories with LAN peers.
func (a *App) StartLanShare(instanceID string, dirs []string) (lan.Snapshot, error) {
	if err := a.ready(); err != nil {
		return lan.Snapshot{}, err
	}
	instance, ok := a.store.Instance(instanceID)
	if !ok {
		return lan.Snapshot{}, errors.New("要共享的本地版本不存在")
	}
	dirs, err := lansync.NormalizeDirs(dirs)
	if err != nil {
		return lan.Snapshot{}, err
	}

	a.lanShare.mu.Lock()
	previous := a.lanShare.server
	a.lanShare.server = nil
	a.lanShare.instanceID = ""
	a.lanShare.dirs = nil
	a.lanShare.mu.Unlock()
	if previous != nil {
		previous.Stop()
	}

	task := a.newTask("准备局域网共享 · " + instance.Name)
	server, err := lansync.StartServer(a.context(), a.instanceDir(instance), lansync.InstanceInfo{
		ID:              instance.ID,
		Name:            instance.Name,
		MCVersion:       instance.MCVersion,
		Loader:          instance.Loader,
		LoaderVersion:   instance.LoaderVersion,
		LaunchVersionID: instance.LaunchVersionID,
	}, dirs, func(stage, message string, current, total int64) {
		task.update(stage, message, current, total)
	})
	task.finish(err)
	if err != nil {
		if a.lanSvc != nil {
			a.lanSvc.SetIdentity(a.lanIdentity(a.defaultLanInstanceID()))
		}
		return lan.Snapshot{}, err
	}

	a.lanShare.mu.Lock()
	a.lanShare.server = server
	a.lanShare.instanceID = instanceID
	a.lanShare.dirs = append([]string(nil), dirs...)
	a.lanShare.mu.Unlock()

	identity := a.lanIdentity(instanceID)
	identity.SyncPort = server.Port()
	identity.SyncToken = server.Token()
	identity.SyncDirs = append([]string(nil), dirs...)
	if err := a.updateLanIdentity(identity); err != nil {
		server.Stop()
		a.lanShare.mu.Lock()
		a.lanShare.server = nil
		a.lanShare.instanceID = ""
		a.lanShare.dirs = nil
		a.lanShare.mu.Unlock()
		return lan.Snapshot{}, fmt.Errorf("共享已准备，但局域网广播启动失败: %w", err)
	}
	return a.lanSvc.Snapshot(), nil
}

// StopLanShare stops sharing and clears the sync advertisement.
func (a *App) StopLanShare() (lan.Snapshot, error) {
	a.lanShare.mu.Lock()
	server := a.lanShare.server
	a.lanShare.server = nil
	a.lanShare.instanceID = ""
	a.lanShare.dirs = nil
	a.lanShare.mu.Unlock()
	if server != nil {
		server.Stop()
	}
	if a.lanSvc != nil {
		a.lanSvc.SetIdentity(a.lanIdentity(a.defaultLanInstanceID()))
		return a.lanSvc.Snapshot(), nil
	}
	return lan.Snapshot{}, nil
}

// SyncFromLanPeer downloads a peer's shared instance content into a local
// instance. The MC version and loader must match.
func (a *App) SyncFromLanPeer(peerID, targetInstanceID string) (lansync.SyncResult, error) {
	var result lansync.SyncResult
	if err := a.ready(); err != nil {
		return result, err
	}
	if a.lanSvc == nil {
		return result, errors.New("局域网发现尚未初始化")
	}
	target, ok := a.lanSvc.SyncTarget(peerID)
	if !ok {
		return result, errors.New("该玩家没有共享版本，或已经离线")
	}
	instance, ok := a.store.Instance(targetInstanceID)
	if !ok {
		return result, errors.New("目标本地版本不存在")
	}
	a.procMu.Lock()
	_, running := a.procs[targetInstanceID]
	a.procMu.Unlock()
	if running {
		return result, errors.New("请先关闭目标版本再同步")
	}
	if strings.TrimSpace(instance.MCVersion) != strings.TrimSpace(target.MCVersion) ||
		normalizeLoader(instance.Loader) != normalizeLoader(target.Loader) {
		return result, fmt.Errorf("目标版本与共享版本不一致：共享为 %s / %s，本地为 %s / %s",
			target.MCVersion, loaderDisplay(target.Loader),
			instance.MCVersion, loaderDisplay(instance.Loader))
	}

	a.lanSyncMu.Lock()
	defer a.lanSyncMu.Unlock()

	baseURL := "http://" + net.JoinHostPort(target.Address, strconv.Itoa(target.Port))
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	task := a.newTask("局域网同步 · " + target.Instance)
	manifest, err := lansync.FetchManifest(ctx, baseURL, target.Token)
	if err != nil {
		task.finish(err)
		return result, err
	}
	if manifest.Instance.MCVersion != "" {
		if strings.TrimSpace(manifest.Instance.MCVersion) != strings.TrimSpace(instance.MCVersion) ||
			(manifest.Instance.Loader != "" && normalizeLoader(manifest.Instance.Loader) != normalizeLoader(instance.Loader)) {
			err := errors.New("共享版本的元数据与目标版本不一致")
			task.finish(err)
			return result, err
		}
	}
	advertised, err := lansync.NormalizeDirs(target.Dirs)
	if err != nil {
		task.finish(err)
		return result, err
	}
	allowed := map[string]bool{}
	for _, dir := range advertised {
		allowed[dir] = true
	}
	for _, dir := range manifest.Dirs {
		if !allowed[dir] {
			err := errors.New("共享目录信息与广播不一致")
			task.finish(err)
			return result, err
		}
	}

	result, err = lansync.Sync(ctx, baseURL, target.Token, a.instanceDir(instance), manifest, func(stage, message string, current, total int64) {
		task.update(stage, message, current, total)
	})
	task.finish(err)
	if err != nil {
		return result, err
	}
	a.emitInstancesChanged()
	return result, nil
}

func normalizeLoader(loader string) string {
	loader = strings.ToLower(strings.TrimSpace(loader))
	if loader == "" {
		return "vanilla"
	}
	return loader
}

func loaderDisplay(loader string) string {
	if normalizeLoader(loader) == "vanilla" {
		return "Vanilla"
	}
	return loader
}
