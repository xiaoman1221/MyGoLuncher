package license

import (
	"testing"
	"time"
)

func TestEntitledFeatures(t *testing.T) {
	cases := []struct {
		name     string
		state    string
		features []string
		feature  string
		want     bool
	}{
		{"exact", "active", []string{"server", "lan", "ai"}, "ai", true},
		{"missing", "active", []string{"server"}, "ai", false},
		{"wildcard", "active", []string{"*"}, "cloud", true},
		{"pro", "grace", []string{"pro"}, "hosting", true},
		{"expired", "expired", []string{"*"}, "server", false},
		{"empty", "active", nil, "server", false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			manager := &Manager{client: &Client{}, now: time.Now, status: Status{State: test.state, Features: test.features}}
			if got := manager.Entitled(test.feature); got != test.want {
				t.Fatalf("Entitled(%q) = %v, want %v", test.feature, got, test.want)
			}
		})
	}
}
