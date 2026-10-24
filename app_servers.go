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
	ctx, cancel := context.WithTimeout(a.context(), 25*time.Second)
	defer cancel()
	instances, permission, listErr := client.UserInstances(ctx)
	// Admin API keys often have no instances assigned to their user, so the
	// instance picker must also query the admin-wide list. Do that only when it
	// can help; normal API keys are not allowed to call it.
	if listErr != nil || permission >= 10 || len(instances) == 0 {
		if all, allErr := client.ListAllInstances(ctx); allErr == nil {
			instances = mergeMcsmInstances(instances, all)
		} else if listErr == nil && len(instances) == 0 {
			listErr = fmt.Errorf("当前 API Key 没有可访问的实例，且无法读取管理员实例列表: %w", allErr)
		}
	}
	if listErr != nil && len(instances) == 0 {
		return nil, fmt.Errorf("连接 MCSM 失败: %w", listErr)
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

func mergeMcsmInstances(primary, extra []mcsm.RemoteInstance) []mcsm.RemoteInstance {
	seen := map[string]bool{}
	out := make([]mcsm.RemoteInstance, 0, len(primary)+len(extra))
	add := func(instance mcsm.RemoteInstance) {
		key := strings.ToLower(strings.TrimSpace(instance.DaemonID)) + "\x00" +
			strings.ToLower(strings.TrimSpace(instance.InstanceUUID))
		if instance.InstanceUUID == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, instance)
	}
	for _, instance := range primary {
		add(instance)
	}
	for _, instance := range extra {
		add(instance)
	}
	return out
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
			ctx, cancel := context.WithTimeout(a.context(), 20*time.Second)
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

// GetServerBackups returns the rollback history stored for one profile.
func (a *App) GetServerBackups(id string) ([]mcsm.Backup, error) {
	if err := a.requireServers(); err != nil {
		return nil, err
	}
	profile, ok := a.servers.Get(id)
	if !ok {
		return nil, errors.New("服务器配置不存在")
	}
	if profile.Backups == nil {
		return []mcsm.Backup{}, nil
	}
	return append([]mcsm.Backup(nil), profile.Backups...), nil
}

// RollbackServer restores a remote snapshot. The server must be stopped; a
// safety snapshot of the current files is taken first unless automatic backups
// are disabled.
func (a *App) RollbackServer(id, backupID string) (err error) {
	if err := a.ready(); err != nil {
		return err
	}
	profile, client, err := a.resolveServerClient(id)
	if err != nil {
		return err
	}
	profile, backup, ok := a.servers.Backup(id, backupID)
	if !ok {
		return errors.New("回滚快照不存在")
	}
	a.syncMu.Lock()
	defer a.syncMu.Unlock()

	task := a.newTask("回滚服务器 · " + profile.Name)
	defer func() { task.finish(err) }()
	task.update("status", "正在检查服务器状态…", 0, 0)
	statusCtx, cancel := context.WithTimeout(a.context(), 15*time.Second)
	status, statusErr := client.Status(statusCtx, profile.DaemonID, profile.InstanceUUID)
	cancel()
	if statusErr != nil {
		return fmt.Errorf("无法确认服务器状态，请先手动停止服务器: %w", statusErr)
	}
	if mcsm.IsRemoteRunning(status.Status) {
		return errors.New("请先停止服务器再回滚，避免覆盖正在使用的模组与配置")
	}
	if !a.store.Settings().ServerBackupDisabled {
		task.update("backup", "正在保存当前状态…", 0, 0)
		backupCtx, cancelBackup := context.WithTimeout(a.context(), 5*time.Minute)
		safety, safetyErr := client.CreateBackup(backupCtx, profile.DaemonID, profile.InstanceUUID,
			profile.RemoteRoot, profile.ID, profile.SyncDirs, "before-rollback", nil)
		cancelBackup()
		if safetyErr != nil {
			return fmt.Errorf("保存当前状态失败，已取消回滚: %w", safetyErr)
		}
		// Keep one extra slot while restoring: the selected snapshot may be the
		// oldest entry and must not lose its remote archive before it is used.
		if _, addErr := a.servers.AddBackup(id, safety, 6); addErr != nil {
			return addErr
		}
	}
	restoreCtx, cancelRestore := context.WithTimeout(a.context(), 10*time.Minute)
	defer cancelRestore()
	if err := client.RestoreBackup(restoreCtx, profile.DaemonID, profile.InstanceUUID,
		profile.RemoteRoot, profile.ID, backup, func(stage, message string, current, total int64) {
			task.update(stage, message, current, total)
		}); err != nil {
		return err
	}
	if removed, trimErr := a.servers.TrimBackups(id, 5); trimErr == nil {
		a.pruneServerBackups(client, profile, removed)
	}
	a.servers.MarkRolledBack(id)
	a.emitServersChanged()
	return nil
}

func (a *App) pruneServerBackups(client *mcsm.Client, profile mcsm.Profile, removed []mcsm.Backup) {
	if len(removed) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(a.context(), time.Minute)
	defer cancel()
	for _, backup := range removed {
		_ = client.DeleteBackup(ctx, profile.DaemonID, profile.InstanceUUID, profile.RemoteRoot, profile.ID, backup)
	}
}

// PreviewServerSync lists the local files that a full sync would consider. The
// result marks client-only mods so the UI can leave them unselected.
func (a *App) PreviewServerSync(id string) ([]mcsm.SyncFile, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	profile, ok := a.servers.Get(id)
	if !ok {
		return nil, errors.New("服务器配置不存在")
	}
	instance, ok := a.store.Instance(profile.LocalInstanceID)
	if !ok {
		return nil, errors.New("绑定的本地版本不存在")
	}
	files, err := mcsm.ScanFiles(a.instanceDir(instance), profile.SyncDirs)
	if err != nil {
		return nil, err
	}
	if files == nil {
		files = []mcsm.SyncFile{}
	}
	return files, nil
}

// SyncServerProfile incrementally uploads the bound local instance to MCSM.
func (a *App) SyncServerProfile(id string) (mcsm.SyncResult, error) {
	return a.syncServerProfile(id, nil)
}

// SyncServerProfileSelected uploads only the selected instance-relative files.
// Client-only mods are still rejected by the backend even if the UI sends them.
func (a *App) SyncServerProfileSelected(id string, files []string) (mcsm.SyncResult, error) {
	if len(files) == 0 {
		return mcsm.SyncResult{}, errors.New("请至少选择一个要更新的文件")
	}
	return a.syncServerProfile(id, files)
}

func (a *App) syncServerProfile(id string, files []string) (mcsm.SyncResult, error) {
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
	if !a.store.Settings().ServerBackupDisabled {
		task.update("backup", "正在创建回滚快照…", 0, 0)
		backupCtx, cancel := context.WithTimeout(a.context(), 5*time.Minute)
		backup, backupErr := client.CreateBackup(backupCtx, profile.DaemonID, profile.InstanceUUID,
			profile.RemoteRoot, profile.ID, profile.SyncDirs, "sync", func(stage, message string, current, total int64) {
				task.update(stage, message, current, total)
			})
		cancel()
		if backupErr != nil {
			task.finish(backupErr)
			return mcsm.SyncResult{}, fmt.Errorf("创建回滚快照失败，已取消同步: %w", backupErr)
		}
		removed, addErr := a.servers.AddBackup(id, backup, 5)
		if addErr != nil {
			task.finish(addErr)
			return mcsm.SyncResult{}, addErr
		}
		a.pruneServerBackups(client, profile, removed)
	}
	result, err := client.Sync(a.context(), profile.DaemonID, profile.InstanceUUID, mcsm.SyncOptions{
		LocalDir:     a.instanceDir(instance),
		RemoteRoot:   profile.RemoteRoot,
		Dirs:         profile.SyncDirs,
		Files:        files,
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
