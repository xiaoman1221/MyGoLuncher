package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"MyMinecraftGoLuncher/internal/aisearch"
)

// BeginOrcaRouterLogin runs the OrcaRouter PKCE one-click login. It opens the
// system browser, waits for the loopback callback, exchanges the code, and
// stores the returned API key in the encrypted secret store.
func (a *App) BeginOrcaRouterLogin() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.orcaMu.Lock()
	if a.orcaCancel != nil {
		a.orcaCancel()
		a.orcaCancel = nil
	}
	a.orcaLoginID++
	loginID := a.orcaLoginID
	ctx, cancel := context.WithTimeout(a.context(), 5*time.Minute)
	a.orcaCancel = cancel
	a.orcaMu.Unlock()
	defer func() {
		cancel()
		a.orcaMu.Lock()
		if a.orcaLoginID == loginID {
			a.orcaCancel = nil
		}
		a.orcaMu.Unlock()
	}()

	result, err := aisearch.BeginOrcaLogin(ctx, aisearch.OrcaLoginOptions{
		AuthURL:  aisearch.OrcaAuthURL,
		TokenURL: aisearch.OrcaTokenURL,
		AppName:  aisearch.OrcaAppName,
		Ref:      aisearch.OrcaRef,
		OpenBrowser: func(authURL string) error {
			if a.ctx == nil {
				return errors.New("启动器尚未完成初始化")
			}
			runtime.BrowserOpenURL(a.ctx, authURL)
			return nil
		},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("OrcaRouter 授权超时，请重试")
		}
		if errors.Is(err, context.Canceled) {
			return errors.New("已取消 OrcaRouter 授权")
		}
		return err
	}

	settings := a.store.Settings()
	settings.AISearchProvider = aisearch.ProviderOrcaRouter
	settings.AISearchAPIKey = result.APIKey
	settings.AISearchModel = aisearch.DefaultOrcaRouterModel
	settings.AISearchBaseURL = ""
	if _, err := a.store.SaveSettings(settings); err != nil {
		return fmt.Errorf("保存 OrcaRouter 密钥失败: %w", err)
	}
	return nil
}

// CancelOrcaRouterLogin cancels an in-flight OrcaRouter authorization.
func (a *App) CancelOrcaRouterLogin() error {
	a.orcaMu.Lock()
	if a.orcaCancel != nil {
		a.orcaCancel()
		a.orcaCancel = nil
	}
	a.orcaMu.Unlock()
	return nil
}
