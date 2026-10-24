// Package mcsm talks to an MCSManager panel and synchronizes a local
// Minecraft instance with a remote MCSM instance.
package mcsm

import "time"

// Sync directories supported by the launcher. They match the server pack
// layout and the directories Minecraft servers actually consume.
const (
	DirMods          = "mods"
	DirConfig        = "config"
	DirDefaultConfig = "defaultconfigs"
	DirScripts       = "scripts"
	DirKubeJS        = "kubejs"
	DirDatapacks     = "datapacks"
	DirResourcepacks = "resourcepacks"
	DirShaderpacks   = "shaderpacks"
)

// AllowedSyncDirs is the canonical order used by the UI and validation.
var AllowedSyncDirs = []string{
	DirMods, DirConfig, DirDefaultConfig, DirScripts,
	DirKubeJS, DirDatapacks, DirResourcepacks, DirShaderpacks,
}

// Profile is one local-instance-to-MCSM-instance binding. It never contains
// the API key; that lives in the encrypted secret store.
type Profile struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	PanelURL        string   `json:"panelUrl"`
	DaemonID        string   `json:"daemonId"`
	InstanceUUID    string   `json:"instanceUuid"`
	LocalInstanceID string   `json:"localInstanceId"`
	RemoteRoot      string   `json:"remoteRoot"`
	SyncDirs        []string `json:"syncDirs"`
	DeleteExtras    bool     `json:"deleteExtras"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	LastSync        string   `json:"lastSync,omitempty"`
}

// ProfileView adds whether an API key is stored without exposing it.
type ProfileView struct {
	Profile
	APIKeySet bool `json:"apiKeySet"`
}

// SaveProfileRequest is sent by the UI. An empty APIKey keeps the stored key
// when ID is an existing profile; ClearAPIKey removes it.
type SaveProfileRequest struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	PanelURL        string   `json:"panelUrl"`
	DaemonID        string   `json:"daemonId"`
	InstanceUUID    string   `json:"instanceUuid"`
	LocalInstanceID string   `json:"localInstanceId"`
	RemoteRoot      string   `json:"remoteRoot"`
	SyncDirs        []string `json:"syncDirs"`
	DeleteExtras    bool     `json:"deleteExtras"`
	APIKey          string   `json:"apiKey"`
	ClearAPIKey     bool     `json:"clearApiKey"`
}

// ConnectRequest is used for testing credentials and listing instances before
// a profile is saved. When ProfileID is set and APIKey is empty, the stored
// key is used.
type ConnectRequest struct {
	ProfileID string `json:"profileId"`
	PanelURL  string `json:"panelUrl"`
	APIKey    string `json:"apiKey"`
}

// RemoteInstance is the safe subset of an MCSM instance list entry.
type RemoteInstance struct {
	DaemonID     string `json:"daemonId"`
	InstanceUUID string `json:"instanceUuid"`
	Nickname     string `json:"nickname"`
	Status       int    `json:"status"`
}

// ServerStatus is reported for one configured profile.
type ServerStatus struct {
	ProfileID  string `json:"profileId"`
	Online     bool   `json:"online"`
	Status     int    `json:"status"`
	StatusText string `json:"statusText"`
	Nickname   string `json:"nickname"`
	Error      string `json:"error,omitempty"`
}

// RemoteFile is one entry returned by the MCSM file manager.
type RemoteFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	Type int    `json:"type"` // 0 directory, 1 file
	Time string `json:"time"`
	Mode int    `json:"mode"`
}

// SyncResult summarizes one synchronization run.
type SyncResult struct {
	Uploaded int   `json:"uploaded"`
	Skipped  int   `json:"skipped"`
	Deleted  int   `json:"deleted"`
	Bytes    int64 `json:"bytes"`
}

// SyncOptions controls one synchronization run.
type SyncOptions struct {
	LocalDir     string
	RemoteRoot   string
	Dirs         []string
	DeleteExtras bool
	Progress     ProgressFunc
}

// ProgressFunc reports long-running sync progress. current/total refer to
// bytes during the upload stage and to files during other stages.
type ProgressFunc func(stage, message string, current, total int64)

// ProfileSecretName returns the secret store key for a profile's API key.
func ProfileSecretName(id string) string {
	return "mcsm.apikey." + id
}

// StatusText converts MCSM's numeric status into a stable identifier the UI
// can translate.
func StatusText(status int) string {
	switch status {
	case -1:
		return "busy"
	case 0:
		return "stopped"
	case 1:
		return "stopping"
	case 2:
		return "starting"
	case 3:
		return "running"
	default:
		return "unknown"
	}
}

// IsRunning reports whether the MCSM status means the instance is active.
func IsRunning(status int) bool {
	return status == 3 || status == 2
}

// Now returns the timestamp format used in profiles.
func Now() string {
	return time.Now().Format(time.RFC3339)
}
