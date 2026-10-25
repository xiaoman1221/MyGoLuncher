package license

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	trialSecretName = "mygoluncher.trial.v1"
	TrialTotal      = 3
	TrialDays       = 7
	trialDuration   = TrialDays * 24 * time.Hour
)

// trialState is encrypted in the same platform secret store as activation
// credentials. It is machine/user-local; clearing the store can reset it, so
// stronger enforcement requires a server-side trial record.
type trialState struct {
	Starts    int       `json:"starts"`
	StartedAt time.Time `json:"startedAt,omitempty"`
	EndsAt    time.Time `json:"endsAt,omitempty"`
	LastSeen  time.Time `json:"lastSeen,omitempty"`
}

func (m *Manager) loadTrial() {
	raw, err := m.secrets.Get(trialSecretName)
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	var trial trialState
	if err := json.Unmarshal([]byte(raw), &trial); err != nil {
		return
	}
	if trial.Starts < 0 {
		trial.Starts = 0
	}
	if trial.Starts > TrialTotal {
		trial.Starts = TrialTotal
	}
	m.trial = trial
}

func (m *Manager) saveTrialLocked() error {
	raw, err := json.Marshal(m.trial)
	if err != nil {
		return err
	}
	return m.secrets.Set(trialSecretName, string(raw))
}

// StartTrial begins one seven-day trial. Three independent trials are allowed;
// an active trial must expire before the next one can start.
func (m *Manager) StartTrial() (Status, error) {
	if m == nil || m.client == nil {
		return m.Status(), errors.New("当前构建未配置商业授权服务，不能开始试用")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.mu.Lock()
	now := m.now().UTC()
	if m.status.State == "active" || m.status.State == "grace" {
		m.mu.Unlock()
		return m.Status(), errors.New("当前已有正式授权，无需试用")
	}
	if m.trial.Starts >= TrialTotal {
		m.mu.Unlock()
		return m.Status(), errors.New("试用次数已用完")
	}
	if m.trial.EndsAt.After(now) {
		m.mu.Unlock()
		return m.Status(), errors.New("试用正在进行中")
	}
	if !m.trial.LastSeen.IsZero() && now.Add(12*time.Hour).Before(m.trial.LastSeen) {
		m.mu.Unlock()
		return m.Status(), errors.New("检测到系统时间异常，暂时不能开始试用")
	}
	m.trial.Starts++
	m.trial.StartedAt = now
	m.trial.EndsAt = now.Add(trialDuration)
	m.trial.LastSeen = now
	if err := m.saveTrialLocked(); err != nil {
		m.mu.Unlock()
		return m.Status(), fmt.Errorf("保存试用状态失败: %w", err)
	}
	m.mu.Unlock()
	return m.Status(), nil
}

func (m *Manager) trialStatusLocked(now time.Time) (Status, bool) {
	if m.trial.Starts == 0 {
		return Status{}, false
	}
	status := Status{
		State: "trial_expired", Configured: m.client != nil, Trial: true,
		MachineCode: MachineCode(Collect()), Features: []string{},
		TrialRemaining: TrialTotal - m.trial.Starts, TrialDays: TrialDays,
		Message: "试用已结束",
	}
	if !m.trial.EndsAt.IsZero() {
		ends := m.trial.EndsAt
		status.TrialEndsAt = formatTime(&ends)
	}
	if m.trial.Starts >= TrialTotal {
		status.Message = "三次试用已用完"
	}
	if m.trial.EndsAt.After(now) {
		status.State = "trial"
		status.Activated = true
		status.Features = []string{"*"}
		status.TrialDaysLeft = int(math.Ceil(m.trial.EndsAt.Sub(now).Hours() / 24))
		if status.TrialDaysLeft < 1 {
			status.TrialDaysLeft = 1
		}
		status.Message = fmt.Sprintf("试用中，剩余 %d 天", status.TrialDaysLeft)
	}
	return status, true
}
