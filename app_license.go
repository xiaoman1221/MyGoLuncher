package main

import (
	"context"
	"errors"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	licensepkg "MyMinecraftGoLuncher/internal/license"
)

// GetLicenseStatus returns the cached activation state without touching the
// network. It is allowed before activation so the activation screen can render.
func (a *App) GetLicenseStatus() licensepkg.Status {
	if a.license == nil {
		return licensepkg.Status{
			State: "error", Message: "授权模块未初始化", Features: []string{}, GraceDays: 7,
		}
	}
	return a.license.Status()
}

// ActivateLicense validates an activation code against LicensingGo and stores
// the binding in the OS secret store. The activation code is not written to MGL.
func (a *App) ActivateLicense(code string) (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	status, err := a.license.Activate(ctx, code)
	a.emitLicenseChanged(status)
	a.syncLanEntitlement()
	return status, err
}

// StartTrial begins a seven-day local trial. Three independent trials are
// allowed; the trial record uses the same DPAPI/AES secret store as activation.
func (a *App) StartTrial() (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	if a.license == nil {
		return a.GetLicenseStatus(), errors.New("授权模块未初始化")
	}
	status, err := a.license.StartTrial()
	a.emitLicenseChanged(status)
	a.syncLanEntitlement()
	return status, err
}

// ValidateLicense performs an online validation and applies the seven-day
// offline grace period. A network failure returns the grace status without
// discarding the local activation.
func (a *App) ValidateLicense() (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	ctx, cancel := context.WithTimeout(a.context(), 25*time.Second)
	defer cancel()
	status, err := a.license.Validate(ctx)
	a.emitLicenseChanged(status)
	a.syncLanEntitlement()
	return status, err
}

// UnbindLicense releases this machine and removes the local activation record.
func (a *App) UnbindLicense() (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	status, err := a.license.Unbind(ctx)
	a.emitLicenseChanged(status)
	a.syncLanEntitlement()
	return status, err
}

func (a *App) validateLicenseOnStartup() {
	if a.license == nil || !a.license.Status().Activated {
		return
	}
	ctx, cancel := context.WithTimeout(a.context(), 20*time.Second)
	defer cancel()
	status, err := a.license.Validate(ctx)
	a.emitLicenseChanged(status)
	_ = err
}

func (a *App) emitLicenseChanged(status licensepkg.Status) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "license:changed", status)
}
