package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"MyMinecraftGoLuncher/internal/mcsm"
)

func (a *App) requireServers() error {
	if a.serverErr != "" {
		return errors.New(a.serverErr)
	}
	if a.servers == nil {
		return errors.New("服务器管理尚未初始化")
	}
	return nil
}

func (a *App) resolveServerClient(id string) (mcsm.Profile, *mcsm.Client, error) {
	if err := a.requireServers(); err != nil {
		return mcsm.Profile{}, nil, err
	}
	profile, ok := a.servers.Get(id)
	if !ok {
		return mcsm.Profile{}, nil, errors.New("服务器配置不存在")
	}
	key, err := a.servers.APIKey(id)
	if err != nil {
		return mcsm.Profile{}, nil, fmt.Errorf("读取 API Key 失败: %w", err)
	}
	if strings.TrimSpace(key) == "" {
		return mcsm.Profile{}, nil, errors.New("请先为服务器配置 MCSM API Key")
	}
	client, err := mcsm.NewClient(profile.PanelURL, key)
	if err != nil {
		return mcsm.Profile{}, nil, err
	}
	return profile, client, nil
}

func (a *App) emitServersChanged() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "servers:changed", nil)
	}
}

// ListServerProfiles returns every configured MCSM server without API keys.
func (a *App) ListServerProfiles() ([]mcsm.ProfileView, error) {
	if err := a.requireServers(); err != nil {
		return nil, err
	}
	return a.servers.List(), nil
}

// SaveServerProfile creates or updates an MCSM server profile.
func (a *App) SaveServerProfile(req mcsm.SaveProfileRequest) (mcsm.ProfileView, error) {
	if err := a.requireServers(); err != nil {
		return mcsm.ProfileView{}, err
	}
	if err := a.ready(); err != nil {
		return mcsm.ProfileView{}, err
	}
	if _, ok := a.store.Instance(req.LocalInstanceID); !ok {
		return mcsm.ProfileView{}, errors.New("绑定的本地版本不存在")
	}
	profile, err := a.servers.Save(req)
	if err != nil {
		return mcsm.ProfileView{}, err
	}
	a.emitServersChanged()
	return profile, nil
}

// DeleteServerProfile removes a profile and its encrypted API key.
func (a *App) DeleteServerProfile(id string) error {
	if err := a.requireServers(); err != nil {
		return err
	}
	if err := a.servers.Delete(id); err != nil {
		return err
	}
	a.emitServersChanged()
	return nil
}

// ListMcsmInstances tests credentials and lists the instances they can access.
// When ProfileID is set, its stored API key is used if APIKey is empty.
func (a *App) ListMcsmInstances(req mcsm.ConnectRequest) ([]mcsm.RemoteInstance, error) {
	panelURL := strings.TrimSpace(req.PanelURL)
	apiKey := strings.TrimSpace(req.APIKey)
	if req.ProfileID != "" && a.servers != nil {
		if profile, ok := a.servers.Get(req.ProfileID); ok {
			if panelURL == "" {
				panelURL = profile.PanelURL
			}
			if apiKey == "" {
				stored, err := a.servers.APIKey(req.ProfileID)
				if err != nil {
					return nil, fmt.Errorf("读取 API Key 失败: %w", err)
				}
				apiKey = stored
			}
		}
	}
	if apiKey == "" {
		return nil, errors.New("请填写 MCSM API Key")
	}
	client, err := mcsm.NewClient(panelURL, apiKey)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.context(), 20*time.Second)
	defer cancel()
	instances, err := client.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("连接 MCSM 失败: %w", err)
	}
	if instances == nil {
		instances = []mcsm.RemoteInstance{}
	}
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Nickname == instances[j].Nickname {
			return instances[i].InstanceUUID < instances[j].InstanceUUID
		}
		return instances[i].Nickname < instances[j].Nickname
	})
	return instances, nil
}

// GetServerStatuses reports the remote status of every configured profile.
func (a *App) GetServerStatuses() ([]mcsm.ServerStatus, error) {
	if err := a.requireServers(); err != nil {
		return nil, err
	}
	profiles := a.servers.List()
	statuses := make([]mcsm.ServerStatus, len(profiles))
	var wg sync.WaitGroup
	for index, view := range profiles {
		statuses[index] = mcsm.ServerStatus{
			ProfileID: view.ID, StatusText: "unknown",
		}
		wg.Add(1)
		go func(index int, profile mcsm.Profile) {
			defer wg.Done()
			statuses[index].ProfileID = profile.ID
			_, client, err := a.resolveServerClient(profile.ID)
			if err != nil {
				statuses[index].Error = err.Error()
				return
			}
			ctx, cancel := context.WithTimeout(a.context(), 10*time.Second)
			defer cancel()
			remote, err := client.Status(ctx, profile.DaemonID, profile.InstanceUUID)
			if err != nil {
				statuses[index].Error = err.Error()
				return
			}
			statuses[index].Online = mcsm.IsRunning(remote.Status)
			statuses[index].Status = remote.Status
			statuses[index].StatusText = mcsm.StatusText(remote.Status)
			statuses[index].Nickname = remote.Nickname
		}(index, view.Profile)
	}
	wg.Wait()
	return statuses, nil
}

// StartServer starts the remote MCSM instance.
func (a *App) StartServer(id string) error {
	return a.serverAction(id, "start")
}

// StopServer stops the remote MCSM instance.
func (a *App) StopServer(id string) error {
	return a.serverAction(id, "stop")
}

// RestartServer restarts the remote MCSM instance.
func (a *App) RestartServer(id string) error {
	return a.serverAction(id, "restart")
}

func (a *App) serverAction(id, action string) error {
	profile, client, err := a.resolveServerClient(id)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.context(), 45*time.Second)
	defer cancel()
	switch action {
	case "start":
		err = client.Start(ctx, profile.DaemonID, profile.InstanceUUID)
	case "stop":
		err = client.Stop(ctx, profile.DaemonID, profile.InstanceUUID)
	case "restart":
		err = client.Restart(ctx, profile.DaemonID, profile.InstanceUUID)
	default:
		err = errors.New("未知的服务器操作")
	}
	if err != nil {
		return fmt.Errorf("MCSM 操作失败: %w", err)
	}
	a.emitServersChanged()
	return nil
}

// SyncServerProfile incrementally uploads the bound local instance to MCSM.
func (a *App) SyncServerProfile(id string) (mcsm.SyncResult, error) {
	if err := a.ready(); err != nil {
		return mcsm.SyncResult{}, err
	}
	profile, client, err := a.resolveServerClient(id)
	if err != nil {
		return mcsm.SyncResult{}, err
	}
	instance, ok := a.store.Instance(profile.LocalInstanceID)
	if !ok {
		return mcsm.SyncResult{}, errors.New("绑定的本地版本不存在")
	}

	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	task := a.newTask("同步到服务器 · " + profile.Name)
	result, err := client.Sync(a.context(), profile.DaemonID, profile.InstanceUUID, mcsm.SyncOptions{
		LocalDir:     a.instanceDir(instance),
		RemoteRoot:   profile.RemoteRoot,
		Dirs:         profile.SyncDirs,
		DeleteExtras: profile.DeleteExtras,
		Progress: func(stage, message string, current, total int64) {
			task.update(stage, message, current, total)
		},
	})
	task.finish(err)
	if err != nil {
		return result, err
	}
	a.servers.MarkSynced(id)
	a.emitServersChanged()
	return result, nil
}
