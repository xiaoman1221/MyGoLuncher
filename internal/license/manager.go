package license

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/secretstore"
)

const (
	secretName       = "mygoluncher.license.v1"
	gracePeriod      = 7 * 24 * time.Hour
	offlineGraceDays = 7
)

// Credentials is the encrypted local activation record. It never contains the
// product API key; that key is encrypted separately in product_key.go.
type Credentials struct {
	Code          string     `json:"code"`
	MachineCode   string     `json:"machine_code"`
	Email         string     `json:"email,omitempty"`
	Customer      string     `json:"customer,omitempty"`
	DeviceID      string     `json:"device_id,omitempty"`
	UnbindKey     string     `json:"unbind_key,omitempty"`
	ActivatedAt   time.Time  `json:"activated_at"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	Features      []string   `json:"features,omitempty"`
	LastValidated time.Time  `json:"last_validated,omitempty"`
	// Revoked records a server verdict that the code is no longer usable.
	// It must survive restarts, otherwise the offline grace window would
	// re-activate a revoked license for up to seven days.
	Revoked       bool   `json:"revoked,omitempty"`
	RevokedReason string `json:"revoked_reason,omitempty"`
}

// Status is the serializable state consumed by the activation screen.
type Status struct {
	State          string   `json:"state"`
	Configured     bool     `json:"configured"`
	Activated      bool     `json:"activated"`
	Code           string   `json:"code,omitempty"`
	MachineCode    string   `json:"machineCode"`
	Email          string   `json:"email,omitempty"`
	Customer       string   `json:"customer,omitempty"`
	CanSelfUnbind  bool     `json:"canSelfUnbind"`
	ExpiresAt      string   `json:"expiresAt,omitempty"`
	Features       []string `json:"features"`
	LastChecked    string   `json:"lastChecked,omitempty"`
	GraceUntil     string   `json:"graceUntil,omitempty"`
	Message        string   `json:"message,omitempty"`
	GraceDays      int      `json:"graceDays"`
	Trial          bool     `json:"trial"`
	TrialRemaining int      `json:"trialRemaining"`
	TrialDaysLeft  int      `json:"trialDaysLeft"`
	TrialDays      int      `json:"trialDays"`
	TrialEndsAt    string   `json:"trialEndsAt,omitempty"`
}

// Manager owns local activation state. Credentials are stored through
// secretstore, which uses Windows DPAPI and an encrypted file elsewhere.
type Manager struct {
	secrets secretstore.Store
	client  *Client
	now     func() time.Time

	mu      sync.RWMutex
	opMu    sync.Mutex
	creds   Credentials
	trial   trialState
	status  Status
	machine string
	loadErr error
}

// NewManager loads and protects the local activation record.
func NewManager(secrets secretstore.Store, version string) (*Manager, error) {
	if secrets == nil {
		secrets = secretstore.NewMemory()
	}
	if strings.TrimSpace(version) == "" {
		version = "1.0.4"
	}
	AppVersion = version
	machine := MachineCode(Collect())
	manager := &Manager{
		secrets: secrets,
		now:     time.Now,
		machine: machine,
		status: Status{
			State: "inactive", MachineCode: machine, Features: []string{}, GraceDays: offlineGraceDays,
			Message: "请输入激活码激活本机", TrialRemaining: TrialTotal, TrialDays: TrialDays,
		},
	}
	manager.loadTrial()
	if manager.trial.Starts > 0 {
		// Record the latest observed wall-clock time so a later clock
		// rollback can be detected instead of reviving an expired trial.
		now := manager.now().UTC()
		if now.After(manager.trial.LastSeen) {
			manager.trial.LastSeen = now
			_ = manager.saveTrialLocked()
		}
	}
	config, err := loadProductConfig()
	if errors.Is(err, ErrProductNotConfigured) {
		manager.status.State = "inactive"
		manager.status.Message = "社区构建：商业授权服务未配置"
		manager.loadErr = err
		return manager, nil
	}
	if err != nil {
		manager.status.State = "error"
		manager.status.Message = "授权配置不可用：" + err.Error()
		manager.loadErr = err
		return manager, err
	}
	baseURL := strings.TrimSpace(ServerURL)
	if baseURL == "" {
		baseURL = config.Server
	}
	manager.client = NewClient(baseURL, config.APIKey)
	raw, err := secrets.Get(secretName)
	if err != nil {
		manager.loadErr = err
		return manager, err
	}
	if strings.TrimSpace(raw) != "" {
		var creds Credentials
		if err := json.Unmarshal([]byte(raw), &creds); err != nil {
			manager.status.State = "error"
			manager.status.Message = "本地授权记录损坏，请重新激活"
			manager.loadErr = errors.New("授权记录损坏")
		} else {
			manager.creds = creds
			manager.status = manager.statusFor(creds)
		}
	}
	return manager, nil
}

// Status returns a copy of the cached activation state.
func (m *Manager) Status() Status {
	if m == nil {
		return Status{State: "error", Message: "授权模块未初始化", Features: []string{}, GraceDays: offlineGraceDays}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	status := m.status
	status.Configured = m.client != nil
	status.TrialRemaining = TrialTotal - m.trial.Starts
	status.TrialDays = TrialDays
	if status.Features == nil {
		status.Features = []string{}
	}
	status.Features = append([]string{}, status.Features...)
	if status.State == "active" || status.State == "grace" {
		// Re-check the clock on every status read: rolling the system clock
		// back must not extend a license indefinitely.
		if !m.creds.LastValidated.IsZero() && m.now().UTC().Add(12*time.Hour).Before(m.creds.LastValidated) {
			status.State = "error"
			status.Message = "检测到系统时间异常，请校准系统时间后重新校验授权"
		}
		return status
	}
	if m.client != nil {
		if trial, ok := m.trialStatusLocked(m.now().UTC()); ok {
			return trial
		}
	}
	return status
}

// Active reports whether the cached state allows normal application use.
func (m *Manager) Active() bool {
	status := m.Status()
	return status.State == "active" || status.State == "grace" || status.State == "trial"
}

// Activate binds this machine and persists the returned unbind key.
func (m *Manager) Activate(ctx context.Context, code, email, customer string) (Status, error) {
	if m == nil || m.client == nil {
		return m.Status(), errors.New("授权服务未配置")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	code = strings.ToUpper(strings.TrimSpace(code))
	email = strings.ToLower(strings.TrimSpace(email))
	customer = strings.TrimSpace(customer)
	if code == "" {
		return m.Status(), errors.New("请输入激活码")
	}
	if email == "" || !strings.Contains(email, "@") {
		return m.Status(), errors.New("请输入激活码绑定的邮箱")
	}
	if customer == "" {
		return m.Status(), errors.New("请输入激活码绑定的客户名称")
	}
	if status := m.Status(); status.State == "machine_changed" {
		return status, errors.New("机器码已变化，请先在原机器解绑，或联系管理员释放名额")
	}
	machine := m.Status().MachineCode
	if machine == "" {
		return m.Status(), errors.New("无法计算本机机器码")
	}
	host, _ := osHostname()
	result, err := m.client.Activate(ctx, ActivateRequest{
		Code: code, MachineCode: machine, MachineName: host,
		OS: runtime.GOOS, Email: email, Customer: customer, Extra: map[string]any{"app_version": AppVersion},
	})
	if err != nil {
		return m.Status(), err
	}
	if !strings.EqualFold(strings.TrimSpace(result.Status), "active") {
		return m.Status(), errors.New(firstNonEmpty(result.Message, "激活未成功"))
	}
	m.mu.Lock()
	previous := m.creds
	m.mu.Unlock()
	if previous.MachineCode == machine && strings.EqualFold(previous.Code, code) && result.UnbindKey == "" {
		// Idempotent activation does not return a second unbind key.
		result.UnbindKey = previous.UnbindKey
	}
	creds := Credentials{
		Code:          code,
		MachineCode:   machine,
		Email:         email,
		Customer:      customer,
		DeviceID:      result.DeviceID,
		UnbindKey:     result.UnbindKey,
		ActivatedAt:   m.now().UTC(),
		ExpiresAt:     result.ExpiresAt,
		Features:      cleanFeatures(result.Features),
		LastValidated: m.now().UTC(),
	}
	if err := m.save(creds); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	now := m.now().UTC()
	if m.trial.EndsAt.After(now) {
		// A paid activation supersedes an active trial; it must not resume
		// later when the paid license expires.
		m.trial.EndsAt = now
		m.trial.LastSeen = now
		_ = m.saveTrialLocked()
	}
	m.creds = creds
	m.status = m.statusFor(creds)
	m.mu.Unlock()
	return m.Status(), nil
}

// Validate performs the online startup check. Invalid licenses are returned as
// state, not as a network error. Network failures fall back to the 7-day grace
// window when there was a previously successful validation.
func (m *Manager) Validate(ctx context.Context) (Status, error) {
	if m == nil || m.client == nil {
		return m.Status(), errors.New("授权服务未配置")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if strings.TrimSpace(creds.Code) == "" {
		return m.Status(), nil
	}
	now := m.now().UTC()
	// Always ask the server, even when the local expiry has passed: the
	// operator may have renewed the code in the meantime.
	locallyExpired := creds.ExpiresAt != nil && !creds.ExpiresAt.After(now)
	machine := m.machine
	result, err := m.client.Validate(ctx, creds.Code, machine)
	if err != nil {
		if locallyExpired {
			m.setStatus(Status{
				State: "expired", Activated: true, Code: maskCode(creds.Code),
				MachineCode: machine, Email: creds.Email, Customer: creds.Customer,
				CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "", ExpiresAt: formatTime(creds.ExpiresAt),
				Features: cleanFeatures(creds.Features), Message: "激活码已过期，请联网续费或等待续费生效",
				LastChecked: now.Format(time.RFC3339), GraceDays: offlineGraceDays,
			})
			return m.Status(), nil
		}
		// A definitive client error (bad product key, disabled product,
		// revoked token) must not be softened into an offline grace period.
		// 408/429 are transient and still qualify for grace.
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 &&
			apiErr.StatusCode != http.StatusRequestTimeout && apiErr.StatusCode != http.StatusTooManyRequests {
			apiErrText := DescribeError(err)
			m.setStatus(Status{
				State: "error", Activated: true, Code: maskCode(creds.Code),
				MachineCode: machine, Email: creds.Email, Customer: creds.Customer,
				CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "",
				Message:       "授权校验失败：" + apiErrText.Error(),
				LastChecked:   now.Format(time.RFC3339), GraceDays: offlineGraceDays,
				Features: cleanFeatures(creds.Features),
			})
			return m.Status(), apiErrText
		}
		grace := creds.LastValidated.IsZero() || now.Sub(creds.LastValidated) > gracePeriod
		if !grace {
			until := creds.LastValidated.Add(gracePeriod)
			m.setStatus(Status{
				State: "grace", Activated: true, Code: maskCode(creds.Code),
				MachineCode: machine, Email: creds.Email, Customer: creds.Customer,
				CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "", ExpiresAt: formatTime(creds.ExpiresAt),
				Features: cleanFeatures(creds.Features), Message: "授权校验失败，正在宽限期运行",
				LastChecked: now.Format(time.RFC3339), GraceUntil: formatTime(&until), GraceDays: offlineGraceDays,
			})
			return m.Status(), nil
		}
		m.setStatus(Status{
			State: "error", Activated: true, Code: maskCode(creds.Code),
			MachineCode: machine, Email: creds.Email, Customer: creds.Customer,
			CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "",
			Message:       "无法校验授权：" + err.Error(),
			LastChecked:   now.Format(time.RFC3339), GraceDays: offlineGraceDays,
			Features: cleanFeatures(creds.Features),
		})
		return m.Status(), err
	}
	if !result.Valid {
		state := "invalid"
		if result.Reason == "code_expired" {
			state = "expired"
		} else if result.Reason == "not_activated" {
			state = "inactive"
		}
		// Persist the verdict so a restart cannot fall back to "active" just
		// because LastValidated is still inside the grace window.
		creds.Revoked = true
		creds.RevokedReason = firstNonEmpty(result.Reason, result.Message)
		if err := m.save(creds); err != nil {
			return m.Status(), err
		}
		m.mu.Lock()
		m.creds = creds
		m.mu.Unlock()
		m.setStatus(Status{
			State: state, Activated: true, Code: maskCode(creds.Code),
			MachineCode: machine, Email: creds.Email, Customer: creds.Customer,
			CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "", ExpiresAt: formatTime(creds.ExpiresAt),
			Features: cleanFeatures(creds.Features),
			Message:  firstNonEmpty(result.Message, "授权已失效"), LastChecked: now.Format(time.RFC3339),
			GraceDays: offlineGraceDays,
		})
		return m.Status(), nil
	}
	creds.Revoked = false
	creds.RevokedReason = ""
	creds.MachineCode = machine
	creds.ExpiresAt = result.ExpiresAt
	creds.Features = cleanFeatures(result.Features)
	creds.LastValidated = now
	if err := m.save(creds); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.creds = creds
	m.status = m.statusFor(creds)
	m.mu.Unlock()
	return m.Status(), nil
}

// Unbind releases the seat and removes local credentials.
func (m *Manager) Unbind(ctx context.Context) (Status, error) {
	if m == nil || m.client == nil {
		return m.Status(), errors.New("授权服务未配置")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if strings.TrimSpace(creds.Code) == "" || strings.TrimSpace(creds.UnbindKey) == "" {
		return m.Status(), errors.New("没有可用的本地解绑凭证")
	}
	result, err := m.client.Unbind(ctx, creds.Code, creds.MachineCode, creds.UnbindKey)
	if err != nil {
		return m.Status(), err
	}
	if !result.Valid {
		return m.Status(), errors.New(firstNonEmpty(result.Message, "解绑失败"))
	}
	if err := m.secrets.Delete(secretName); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.creds = Credentials{}
	m.status = Status{
		State: "inactive", MachineCode: m.status.MachineCode, Features: []string{},
		Email: creds.Email, Customer: creds.Customer,
		Message: "本机已解绑，可以输入新的激活码", LastChecked: m.now().UTC().Format(time.RFC3339),
		GraceDays: offlineGraceDays,
	}
	m.mu.Unlock()
	return m.Status(), nil
}

// RequestEmailUnbind sends a verification code to the email bound to the
// activation code. It is the fallback when the local unbind key is missing.
func (m *Manager) RequestEmailUnbind(ctx context.Context, email string) (string, error) {
	if m == nil || m.client == nil {
		return "", errors.New("授权服务未配置")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if strings.TrimSpace(creds.Code) == "" || strings.TrimSpace(creds.MachineCode) == "" {
		return "", errors.New("本机尚未激活")
	}
	email = normalizeEmail(email)
	if email == "" {
		email = normalizeEmail(creds.Email)
	}
	if email == "" {
		return "", errors.New("请填写激活码绑定的邮箱")
	}
	result, err := m.client.RequestEmailUnbind(ctx, creds.Code, email, creds.MachineCode)
	if err != nil {
		return "", err
	}
	return firstNonEmpty(result.Message, "如果信息匹配，验证码已发送到绑定邮箱"), nil
}

// ConfirmEmailUnbind releases the seat with the emailed verification code and
// removes the local activation record on success.
func (m *Manager) ConfirmEmailUnbind(ctx context.Context, email, verificationCode string) (Status, error) {
	if m == nil || m.client == nil {
		return m.Status(), errors.New("授权服务未配置")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.RLock()
	creds := m.creds
	m.mu.RUnlock()
	if strings.TrimSpace(creds.Code) == "" || strings.TrimSpace(creds.MachineCode) == "" {
		return m.Status(), errors.New("本机尚未激活")
	}
	email = normalizeEmail(email)
	if email == "" {
		email = normalizeEmail(creds.Email)
	}
	if email == "" {
		return m.Status(), errors.New("请填写激活码绑定的邮箱")
	}
	result, err := m.client.ConfirmEmailUnbind(ctx, creds.Code, email, creds.MachineCode, verificationCode)
	if err != nil {
		return m.Status(), err
	}
	if !result.Valid {
		return m.Status(), errors.New(firstNonEmpty(result.Message, "解绑失败"))
	}
	if err := m.secrets.Delete(secretName); err != nil {
		return m.Status(), err
	}
	m.mu.Lock()
	m.creds = Credentials{}
	m.status = Status{
		State: "inactive", MachineCode: m.status.MachineCode, Features: []string{},
		Email: email, Customer: creds.Customer,
		Message: "本机已解绑，可以输入新的激活码", LastChecked: m.now().UTC().Format(time.RFC3339),
		GraceDays: offlineGraceDays,
	}
	m.mu.Unlock()
	return m.Status(), nil
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (m *Manager) save(creds Credentials) error {
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return m.secrets.Set(secretName, string(raw))
}

func (m *Manager) setStatus(status Status) {
	m.mu.Lock()
	m.status = status
	m.mu.Unlock()
}

func (m *Manager) statusFor(creds Credentials) Status {
	status := Status{
		State: "active", Activated: true, Code: maskCode(creds.Code),
		MachineCode: m.machine, Email: creds.Email, Customer: creds.Customer,
		CanSelfUnbind: strings.TrimSpace(creds.UnbindKey) != "",
		ExpiresAt:     formatTime(creds.ExpiresAt),
		Features:      cleanFeatures(creds.Features), GraceDays: offlineGraceDays,
	}
	if !creds.LastValidated.IsZero() {
		status.LastChecked = creds.LastValidated.UTC().Format(time.RFC3339)
	}
	if status.MachineCode == "" {
		status.MachineCode = creds.MachineCode
	}
	now := m.now().UTC()
	if creds.Revoked {
		status.State = "invalid"
		if creds.RevokedReason == "code_expired" {
			status.State = "expired"
		} else if creds.RevokedReason == "not_activated" {
			status.State = "inactive"
		}
		status.Message = "授权已失效，请重新激活或联系管理员"
		return status
	}
	if creds.MachineCode != "" && !strings.EqualFold(creds.MachineCode, status.MachineCode) {
		status.State = "machine_changed"
		status.Message = "机器码已变化，请先解绑原机器或联系管理员"
		return status
	}
	if !creds.LastValidated.IsZero() && now.Add(12*time.Hour).Before(creds.LastValidated) {
		status.State = "error"
		status.Message = "检测到系统时间异常，请校准系统时间后重新校验授权"
		return status
	}
	if creds.ExpiresAt != nil && !creds.ExpiresAt.After(now) {
		status.State = "expired"
		status.Message = "激活码已过期，请续费"
		return status
	}
	if creds.LastValidated.IsZero() {
		status.Message = "已激活，等待联网校验"
	} else if now.Sub(creds.LastValidated) > gracePeriod {
		status.State = "error"
		status.Message = "授权校验已超过 7 天，请联网重新校验"
	} else {
		status.Message = "授权有效"
	}
	return status
}

func maskCode(code string) string {
	code = strings.TrimSpace(code)
	if len(code) <= 8 {
		return "****"
	}
	return code[:4] + "-****-" + code[len(code)-4:]
}

func cleanFeatures(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func formatTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func osHostname() (string, error) {
	return os.Hostname()
}
