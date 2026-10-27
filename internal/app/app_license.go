package app

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
func (a *App) ActivateLicense(code, email, customer string) (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	status, err := a.license.Activate(ctx, code, email, customer)
	if err != nil {
		err = licensepkg.DescribeError(err)
	}
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
	if err != nil {
		err = licensepkg.DescribeError(err)
	}
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
	if err != nil {
		err = licensepkg.DescribeError(err)
	}
	a.emitLicenseChanged(status)
	a.syncLanEntitlement()
	return status, err
}

// RequestEmailUnbind sends a verification code to the email bound to the
// activation code. It is used when the local unbind key is unavailable.
func (a *App) RequestEmailUnbind(email string) (string, error) {
	if err := a.baseReady(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	message, err := a.license.RequestEmailUnbind(ctx, email)
	if err != nil {
		return "", licensepkg.DescribeError(err)
	}
	return message, nil
}

// ConfirmEmailUnbind releases this machine after email verification and
// removes the local activation record.
func (a *App) ConfirmEmailUnbind(email, verificationCode string) (licensepkg.Status, error) {
	if err := a.baseReady(); err != nil {
		return a.GetLicenseStatus(), err
	}
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	defer cancel()
	status, err := a.license.ConfirmEmailUnbind(ctx, email, verificationCode)
	if err != nil {
		err = licensepkg.DescribeError(err)
	}
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
