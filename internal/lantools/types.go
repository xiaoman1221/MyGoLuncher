package lantools

import "time"

const (
	KindChat  = "chat"
	KindVoice = "voice"
	KindFile  = "file"
)

const (
	MaxChatTextBytes = 4096
	MaxVoiceBytes    = 20 << 20
	MaxFileBytes     = 512 << 20
	MaxMessages      = 200
)

// Message is one chat, voice or file item exchanged through the LAN toolbox.
type Message struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	FromID     string `json:"fromId"`
	FromName   string `json:"fromName"`
	Text       string `json:"text,omitempty"`
	FileName   string `json:"fileName,omitempty"`
	Size       int64  `json:"size,omitempty"`
	MimeType   string `json:"mimeType,omitempty"`
	DurationMS int    `json:"durationMs,omitempty"`
	Path       string `json:"path,omitempty"`
	CreatedAt  string `json:"createdAt"`
	Incoming   bool   `json:"incoming"`
}

// State is the data required by the LAN toolbox UI.
type State struct {
	SelfID   string    `json:"selfId"`
	SelfName string    `json:"selfName"`
	Port     int       `json:"port"`
	Inbox    string    `json:"inbox"`
	Messages []Message `json:"messages"`
}

// FileInfo describes a file selected for sending.
type FileInfo struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Target is the connection information for one peer's LAN toolbox server.
type Target struct {
	Address string
	Port    int
	Token   string
}

func (t Target) Valid() bool {
	return t.Address != "" && t.Port > 0 && t.Port <= 65535 && t.Token != ""
}

func nowString() string { return time.Now().Format(time.RFC3339) }
