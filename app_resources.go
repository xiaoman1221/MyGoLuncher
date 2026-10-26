package main

import (
	"errors"
	"strings"

	"MyMinecraftGoLuncher/internal/resourcemanager"
)

func (a *App) instanceResourceDir(instanceID string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	instance, ok := a.store.Instance(strings.TrimSpace(instanceID))
	if !ok {
		return "", errors.New("版本不存在")
	}
	a.procMu.Lock()
	_, running := a.procs[instance.ID]
	a.procMu.Unlock()
	if running {
		return "", errors.New("请先关闭游戏再修改资源")
	}
	return a.instanceDir(instance), nil
}

// GetInstanceResources returns all mods, resource packs and shader packs,
// including disabled entries.
func (a *App) GetInstanceResources(instanceID string) (resourcemanager.Inventory, error) {
	dir, err := a.instanceResourceDir(instanceID)
	if err != nil {
		return resourcemanager.Inventory{}, err
	}
	return resourcemanager.List(dir), nil
}

// SetInstanceResourceEnabled enables or disables one resource by renaming it
// with a .disabled suffix. The updated inventory is returned for the UI.
func (a *App) SetInstanceResourceEnabled(instanceID, kind, fileName string, enabled bool) (resourcemanager.Inventory, error) {
	dir, err := a.instanceResourceDir(instanceID)
	if err != nil {
		return resourcemanager.Inventory{}, err
	}
	if err := resourcemanager.SetEnabled(dir, kind, fileName, enabled); err != nil {
		return resourcemanager.Inventory{}, err
	}
	return resourcemanager.List(dir), nil
}
