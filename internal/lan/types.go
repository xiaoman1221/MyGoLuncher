// Package lan discovers Minecraft LAN worlds and other launcher users on the
// local network.
package lan

import "time"

// PlayerInfo is one player returned by a server list ping.
type PlayerInfo struct {
	Name string `json:"name"`
}

// World is a discovered Minecraft LAN world or server.
type World struct {
	ID       string       `json:"id"`
	Host     string       `json:"host"`
	Port     int          `json:"port"`
	Address  string       `json:"address"`
	MOTD     string       `json:"motd"`
	Version  string       `json:"version"`
	Online   int          `json:"online"`
	Max      int          `json:"max"`
	Players  []PlayerInfo `json:"players"`
	Source   string       `json:"source"` // lan or scan
	HostName string       `json:"hostName,omitempty"`
	LastSeen string       `json:"lastSeen"`
}

// Peer is another launcher user discovered through the presence protocol.
type Peer struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Instance      string   `json:"instance"`
	MCVersion     string   `json:"mcVersion"`
	Address       string   `json:"address"`
	InstanceID    string   `json:"instanceId,omitempty"`
	Loader        string   `json:"loader,omitempty"`
	LoaderVersion string   `json:"loaderVersion,omitempty"`
	SyncPort      int      `json:"syncPort,omitempty"`
	SyncDirs      []string `json:"syncDirs,omitempty"`
	CanSync       bool     `json:"canSync"`
	CanChat       bool     `json:"canChat"`
	LastSeen      string   `json:"lastSeen"`
}

// Snapshot is the complete LAN state sent to the frontend.
type Snapshot struct {
	Running          bool     `json:"running"`
	Broadcast        bool     `json:"broadcast"`
	Sharing          bool     `json:"sharing"`
	SharedInstanceID string   `json:"sharedInstanceId,omitempty"`
	ShareDirs        []string `json:"shareDirs,omitempty"`
	Worlds           []World  `json:"worlds"`
	Peers            []Peer   `json:"peers"`
	Warnings         []string `json:"warnings,omitempty"`
}

// Identity is the launcher's advertisement on the LAN. SyncPort and SyncToken
// are only set while the instance is being shared.
type Identity struct {
	Name          string
	Instance      string
	MCVersion     string
	InstanceID    string
	Loader        string
	LoaderVersion string
	SyncPort      int
	SyncToken     string
	SyncDirs      []string
	ToolPort      int
	ToolToken     string
}

// SyncTarget is the backend-only connection information for a peer that
// shares an instance.
type SyncTarget struct {
	Address       string
	Port          int
	Token         string
	InstanceID    string
	Instance      string
	MCVersion     string
	Loader        string
	LoaderVersion string
	Dirs          []string
}

// ToolTarget is the backend-only connection information for a peer's LAN
// toolbox server.
type ToolTarget struct {
	Address string
	Port    int
	Token   string
	Name    string
}

// Config controls one discovery session.
type Config struct {
	Broadcast bool
	Identity  Identity
	OnChange  func(Snapshot)
}

type worldEntry struct {
	World
	lastSeen     time.Time
	pingFailures int
}

type peerEntry struct {
	Peer
	syncToken string
	toolPort  int
	toolToken string
	lastSeen  time.Time
}
