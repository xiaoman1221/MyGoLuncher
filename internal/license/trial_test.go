package license

import (
	"testing"
	"time"

	"MyMinecraftGoLuncher/internal/secretstore"
)

func TestTrialSevenDaysThreeTimes(t *testing.T) {
	useTestProduct(t)
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	manager, err := NewManager(secretstore.NewMemory(), "test")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return now }

	status, err := manager.StartTrial()
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "trial" || !status.Trial || !manager.Active() || !manager.Entitled(FeatureServer) {
		t.Fatalf("trial not active: %#v", status)
	}
	if status.TrialRemaining != 2 || status.TrialDaysLeft != 7 || status.TrialDays != 7 {
		t.Fatalf("trial counters = %#v", status)
	}

	now = now.Add(8 * 24 * time.Hour)
	status = manager.Status()
	if status.State != "trial_expired" || status.Activated || manager.Active() {
		t.Fatalf("trial did not expire: %#v", status)
	}
	if status.TrialRemaining != 2 {
		t.Fatalf("remaining attempts = %d", status.TrialRemaining)
	}

	if _, err := manager.StartTrial(); err != nil {
		t.Fatal(err)
	}
	now = now.Add(8 * 24 * time.Hour)
	if _, err := manager.StartTrial(); err != nil {
		t.Fatal(err)
	}
	if manager.trial.Starts != 3 {
		t.Fatalf("starts = %d", manager.trial.Starts)
	}
	now = now.Add(8 * 24 * time.Hour)
	if _, err := manager.StartTrial(); err == nil {
		t.Fatal("fourth trial was allowed")
	}
	status = manager.Status()
	if status.TrialRemaining != 0 || status.State != "trial_expired" {
		t.Fatalf("final trial status = %#v", status)
	}
}
