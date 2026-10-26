package main

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"MyMinecraftGoLuncher/internal/lan"
	licensepkg "MyMinecraftGoLuncher/internal/license"
)

func (a *App) lanIdentity(instanceID string) lan.Identity {
	identity := lan.Identity{Name: "Steve"}
	if a.store != nil {
		settings := a.store.Settings()
		if settings.PlayerName != "" {
			identity.Name = settings.PlayerName
		}
		if instance, ok := a.store.Instance(instanceID); ok {
			identity.Instance = instance.Name
			identity.MCVersion = instance.MCVersion
			identity.InstanceID = instance.ID
			identity.Loader = instance.Loader
			identity.LoaderVersion = instance.LoaderVersion
		}
	}
	return identity
}

func (a *App) handleLANChange(snapshot lan.Snapshot) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "lan:changed", snapshot)
	}
}

// syncLanEntitlement starts or stops background LAN discovery as the paid
// entitlement changes. Free builds never advertise LAN presence.
func (a *App) syncLanEntitlement() {
	if a.lanSvc == nil {
		return
	}
	if a.hasFeature(licensepkg.FeatureLAN) {
		if !a.lanSvc.Snapshot().Running {
			a.startLanBackground()
		}
		return
	}
	a.lanSvc.Stop()
}

// startLanBackground starts continuous LAN discovery at application startup.
// Discovery stays active until shutdown; the LAN page only refreshes the
// advertised identity and broadcast preference.
func (a *App) startLanBackground() {
	if a.lanSvc == nil || a.store == nil {
		return
	}
	instanceID := ""
	if instances := a.store.Instances(); len(instances) > 0 {
		instanceID = instances[0].ID
	}
	_ = a.lanSvc.Start(a.context(), lan.Config{
		Broadcast: false,
		Identity:  a.applyLanShareIdentity(a.lanIdentity(instanceID)),
		OnChange:  a.handleLANChange,
	})
}

// StartLanDiscovery ensures LAN discovery is running and updates the
// advertised identity. It is safe to call while discovery is already running.
func (a *App) StartLanDiscovery(instanceID string, broadcast bool) (lan.Snapshot, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return a.GetLanSnapshot(), err
	}
	if err := a.ready(); err != nil {
		return lan.Snapshot{}, err
	}
	if a.lanSvc == nil {
		return lan.Snapshot{}, errors.New("局域网发现尚未初始化")
	}
	err := a.lanSvc.Start(a.context(), lan.Config{
		Broadcast: broadcast,
		Identity:  a.applyLanShareIdentity(a.lanIdentity(instanceID)),
		OnChange:  a.handleLANChange,
	})
	if err != nil {
		return lan.Snapshot{}, fmt.Errorf("启动局域网发现失败: %w", err)
	}
	return a.lanSvc.Snapshot(), nil
}

// StopLanDiscovery stops all LAN listeners and clears discovered state.
func (a *App) StopLanDiscovery() error {
	if a.lanSvc != nil {
		a.lanSvc.Stop()
	}
	return nil
}

// SetLanBroadcast enables or disables advertising this launcher on the LAN.
func (a *App) SetLanBroadcast(enabled bool) lan.Snapshot {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return a.GetLanSnapshot()
	}
	if a.lanSvc == nil {
		return lan.Snapshot{}
	}
	a.lanSvc.SetBroadcast(enabled)
	return a.lanSvc.Snapshot()
}

// GetLanSnapshot returns the current discovery state.
func (a *App) GetLanSnapshot() lan.Snapshot {
	if a.lanSvc == nil || !a.hasFeature(licensepkg.FeatureLAN) {
		return lan.Snapshot{Running: false, Worlds: []lan.World{}, Peers: []lan.Peer{}}
	}
	return a.lanSvc.Snapshot()
}

// ScanLanSubnets probes the local /24 networks for Minecraft servers.
func (a *App) ScanLanSubnets() (lan.Snapshot, error) {
	if err := a.requireFeature(licensepkg.FeatureLAN); err != nil {
		return a.GetLanSnapshot(), err
	}
	if a.lanSvc == nil {
		return lan.Snapshot{}, errors.New("局域网发现尚未初始化")
	}
	task := a.newTask("扫描局域网服务器")
	err := a.lanSvc.ScanSubnets(task.context(), func(done, total int) {
		task.update("scan", fmt.Sprintf("已扫描 %d / %d", done, total), int64(done), int64(total))
	})
	task.finish(err)
	if err != nil {
		return a.lanSvc.Snapshot(), err
	}
	return a.lanSvc.Snapshot(), nil
}
