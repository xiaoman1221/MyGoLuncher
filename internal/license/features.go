package license

import "strings"

// Paid feature identifiers. The LicensingGo admin console puts these names in
// an activation code's features array. The official client only checks the
// server-issued entitlement; community builds have no paid backend.
const (
	FeatureServer   = "server"   // MCSM server management
	FeatureLAN      = "lan"      // LAN multiplayer/share/sync
	FeatureAI       = "ai"       // AI-assisted resource search
	FeatureCloud    = "cloud"    // cloud sync/backup (server-side)
	FeatureCDN      = "cdn"      // premium modpack CDN
	FeatureHosting  = "hosting"  // managed server hosting
	FeaturePriority = "priority" // priority support
)

var featureLabels = map[string]string{
	FeatureServer:   "服务器管理",
	FeatureLAN:      "局域网联机",
	FeatureAI:       "AI 搜索",
	FeatureCloud:    "云同步与云备份",
	FeatureCDN:      "整合包 CDN",
	FeatureHosting:  "服务器托管",
	FeaturePriority: "优先支持",
}

// FeatureLabel returns a user-facing label.
func FeatureLabel(feature string) string {
	if label, ok := featureLabels[feature]; ok {
		return label
	}
	return feature
}

// Entitled reports whether the current activation includes a paid feature.
// Wildcards are convenient for staff/internal codes. Free features do not
// call this method at all.
func (m *Manager) Entitled(feature string) bool {
	if m == nil {
		return false
	}
	status := m.Status()
	if !status.Configured {
		return false
	}
	if status.State != "active" && status.State != "grace" && status.State != "trial" {
		return false
	}
	feature = strings.ToLower(strings.TrimSpace(feature))
	if feature == "" {
		return false
	}
	for _, granted := range status.Features {
		switch strings.ToLower(strings.TrimSpace(granted)) {
		case "*", "all", "pro", "commercial":
			return true
		case feature:
			return true
		}
	}
	return false
}
