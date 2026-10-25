package main

import (
	"fmt"
	"strings"

	licensepkg "MyMinecraftGoLuncher/internal/license"
)

// requireFeature is used only by paid APIs. Free features (launching, version
// management, basic mod installation) must not call it.
func (a *App) requireFeature(feature string) error {
	if a.license != nil && a.license.Entitled(feature) {
		return nil
	}
	label := licensepkg.FeatureLabel(feature)
	if a.license == nil || !a.license.Status().Activated {
		return fmt.Errorf("此功能需要购买授权：%s", label)
	}
	status := a.license.Status()
	if status.State == "expired" {
		return fmt.Errorf("授权已过期，续费后可使用：%s", label)
	}
	if status.State == "grace" {
		return fmt.Errorf("离线校验期间暂不可用：%s", label)
	}
	return fmt.Errorf("当前授权不包含「%s」", label)
}

// FeatureEntitlements is exposed to the UI so paid pages can show a clear
// upgrade state without treating the license as an application-wide gate.
type FeatureEntitlements struct {
	Active   bool     `json:"active"`
	Features []string `json:"features"`
	State    string   `json:"state"`
	Message  string   `json:"message"`
}

func (a *App) GetFeatureEntitlements() FeatureEntitlements {
	result := FeatureEntitlements{Features: []string{}}
	if a.license == nil {
		return result
	}
	status := a.license.Status()
	result.Active = status.State == "active" || status.State == "grace"
	result.State = status.State
	result.Message = status.Message
	result.Features = append([]string{}, status.Features...)
	return result
}

func (a *App) hasFeature(feature string) bool {
	return a.license != nil && a.license.Entitled(strings.TrimSpace(feature))
}
