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
	licensepkg "MyMinecraftGoLuncher/internal/license"
	"MyMinecraftGoLuncher/internal/store"
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
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return a.GetLanSnapshot(), err
	}
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

// LanSyncPreview describes the differences between a peer's shared version and
// a local instance. The digest must be echoed back when applying the sync so a
// changed manifest cannot be applied without a new confirmation.
type LanSyncPreview struct {
	Peer                  lansync.InstanceInfo `json:"peer"`
	Target                lansync.InstanceInfo `json:"target"`
	Dirs                  []string             `json:"dirs"`
	Plan                  lansync.Plan         `json:"plan"`
	Digest                string               `json:"digest"`
	VersionMismatch       bool                 `json:"versionMismatch"`
	LoaderMismatch        bool                 `json:"loaderMismatch"`
	LoaderVersionMismatch bool                 `json:"loaderVersionMismatch"`
	Running               bool                 `json:"running"`
	Blocking              bool                 `json:"blocking"`
	BlockingReason        string               `json:"blockingReason,omitempty"`
	Warnings              []string             `json:"warnings"`
}

// PreviewLanSync compares a peer's shared instance with a local target without
// writing files. The frontend uses this to show the version and file diff before
// the user confirms the update.
func (a *App) PreviewLanSync(peerID, targetInstanceID string) (LanSyncPreview, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return LanSyncPreview{}, err
	}
	if err := a.ready(); err != nil {
		return LanSyncPreview{}, err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	prepared, err := a.prepareLanSync(ctx, peerID, targetInstanceID)
	if err != nil {
		return LanSyncPreview{}, err
	}
	return prepared.preview, nil
}

type preparedLanSync struct {
	target   lan.SyncTarget
	instance store.Instance
	manifest lansync.Manifest
	baseURL  string
	preview  LanSyncPreview
}

func (a *App) prepareLanSync(ctx context.Context, peerID, targetInstanceID string) (preparedLanSync, error) {
	var prepared preparedLanSync
	if a.lanSvc == nil {
		return prepared, errors.New("局域网发现尚未初始化")
	}
	target, ok := a.lanSvc.SyncTarget(peerID)
	if !ok {
		return prepared, errors.New("该玩家没有共享版本，或已经离线")
	}
	instance, ok := a.store.Instance(targetInstanceID)
	if !ok {
		return prepared, errors.New("目标本地版本不存在")
	}
	baseURL := "http://" + net.JoinHostPort(target.Address, strconv.Itoa(target.Port))
	manifest, err := lansync.FetchManifest(ctx, baseURL, target.Token)
	if err != nil {
		return prepared, err
	}
	advertised, err := lansync.NormalizeDirs(target.Dirs)
	if err != nil {
		return prepared, err
	}
	allowed := map[string]bool{}
	for _, dir := range advertised {
		allowed[dir] = true
	}
	for _, dir := range manifest.Dirs {
		if !allowed[dir] {
			return prepared, errors.New("共享目录信息与广播不一致")
		}
	}
	plan, err := lansync.BuildPlan(a.instanceDir(instance), manifest)
	if err != nil {
		return prepared, err
	}
	a.procMu.Lock()
	_, running := a.procs[targetInstanceID]
	a.procMu.Unlock()

	preview := LanSyncPreview{
		Peer: manifest.Instance,
		Target: lansync.InstanceInfo{
			ID: instance.ID, Name: instance.Name, MCVersion: instance.MCVersion,
			Loader: instance.Loader, LoaderVersion: instance.LoaderVersion,
			LaunchVersionID: instance.LaunchVersionID,
		},
		Dirs:     append([]string(nil), manifest.Dirs...),
		Plan:     plan,
		Digest:   plan.Digest,
		Running:  running,
		Warnings: []string{},
	}
	preview.VersionMismatch = strings.TrimSpace(manifest.Instance.MCVersion) != "" &&
		strings.TrimSpace(manifest.Instance.MCVersion) != strings.TrimSpace(instance.MCVersion)
	preview.LoaderMismatch = normalizeLoader(manifest.Instance.Loader) != normalizeLoader(instance.Loader)
	preview.LoaderVersionMismatch = strings.TrimSpace(manifest.Instance.LoaderVersion) != "" &&
		strings.TrimSpace(manifest.Instance.LoaderVersion) != strings.TrimSpace(instance.LoaderVersion)
	if preview.VersionMismatch {
		preview.Blocking = true
		preview.BlockingReason = fmt.Sprintf("Minecraft 版本不一致：共享为 %s，本地为 %s", manifest.Instance.MCVersion, instance.MCVersion)
	} else if preview.LoaderMismatch {
		preview.Blocking = true
		preview.BlockingReason = fmt.Sprintf("模组加载器不一致：共享为 %s，本地为 %s", loaderDisplay(manifest.Instance.Loader), loaderDisplay(instance.Loader))
	}
	if preview.LoaderVersionMismatch && !preview.Blocking {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("加载器版本不一致：共享为 %s，本地为 %s；确认同步后会自动更新本地加载器", manifest.Instance.LoaderVersion, instance.LoaderVersion))
	}
	if preview.Running {
		preview.Warnings = append(preview.Warnings, "目标版本正在运行，同步前需要先关闭游戏")
	}
	if plan.ExtraFiles > 0 {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("本地有 %d 个文件不在共享清单中，同步不会删除它们", plan.ExtraFiles))
	}
	if plan.NewFiles == 0 && plan.ChangedFiles == 0 {
		preview.Warnings = append(preview.Warnings, "本地文件与共享版本一致，无需更新")
	}
	prepared.target = target
	prepared.instance = instance
	prepared.manifest = manifest
	prepared.baseURL = baseURL
	prepared.preview = preview
	return prepared, nil
}

// SyncFromLanPeer downloads a peer's shared instance content into a local
// instance. expectedDigest comes from PreviewLanSync; it prevents applying a
// changed manifest without a fresh confirmation.
func (a *App) SyncFromLanPeer(peerID, targetInstanceID, expectedDigest string) (lansync.SyncResult, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return lansync.SyncResult{}, err
	}
	var result lansync.SyncResult
	if err := a.ready(); err != nil {
		return result, err
	}
	if strings.TrimSpace(expectedDigest) == "" {
		return result, errors.New("请先确认局域网版本差异")
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Minute)
	defer cancel()
	prepared, err := a.prepareLanSync(ctx, peerID, targetInstanceID)
	if err != nil {
		return result, err
	}
	if prepared.preview.Digest != expectedDigest {
		return result, errors.New("共享版本已变化，请重新确认版本差异")
	}
	if prepared.preview.Blocking {
		return result, errors.New(prepared.preview.BlockingReason)
	}
	if prepared.preview.Running {
		return result, errors.New("请先关闭目标版本再同步")
	}
	a.lanSyncMu.Lock()
	defer a.lanSyncMu.Unlock()

	task := a.newTask("局域网同步 · " + prepared.target.Instance)
	if prepared.preview.Plan.NewFiles > 0 || prepared.preview.Plan.ChangedFiles > 0 {
		result, err = lansync.Sync(ctx, prepared.baseURL, prepared.target.Token, a.instanceDir(prepared.instance), prepared.manifest, func(stage, message string, current, total int64) {
			task.update(stage, message, current, total)
		})
		if err != nil {
			task.finish(err)
			return result, err
		}
	} else {
		result.Skipped = prepared.preview.Plan.SameFiles
	}
	if prepared.preview.LoaderVersionMismatch && strings.TrimSpace(prepared.preview.Peer.LoaderVersion) != "" {
		// The shared pack may require a different loader build. Update the local
		// loader only after the user explicitly confirmed the diff digest.
		task.update("loader", "正在更新本地加载器版本…", 0, 0)
		updated := prepared.instance
		updated.LoaderVersion = prepared.preview.Peer.LoaderVersion
		if err := a.installInstanceRuntime(ctx, &updated, task); err != nil {
			wrapped := fmt.Errorf("文件已同步，但加载器更新失败: %w", err)
			task.finish(wrapped)
			return result, wrapped
		}
	}
	task.finish(nil)
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
