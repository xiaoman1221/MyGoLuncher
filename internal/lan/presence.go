package lan

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	presenceApp     = "MyGoLuncher"
	presenceVersion = 1
	presenceGroup   = "224.0.2.61"
	presencePort    = 4446
	lanGroup        = "224.0.2.60"
	lanPort         = 4445
	maxPresenceSize = 2048
)

type presencePacket struct {
	App           string   `json:"app"`
	Version       int      `json:"v"`
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Instance      string   `json:"instance,omitempty"`
	MCVersion     string   `json:"mcVersion,omitempty"`
	InstanceID    string   `json:"instanceId,omitempty"`
	Loader        string   `json:"loader,omitempty"`
	LoaderVersion string   `json:"loaderVersion,omitempty"`
	SyncPort      int      `json:"syncPort,omitempty"`
	SyncToken     string   `json:"syncToken,omitempty"`
	SyncDirs      []string `json:"syncDirs,omitempty"`
	ToolPort      int      `json:"toolPort,omitempty"`
	ToolToken     string   `json:"toolToken,omitempty"`
	Time          int64    `json:"ts"`
}

func encodePresence(selfID string, identity Identity) ([]byte, error) {
	packet := presencePacket{
		App:           presenceApp,
		Version:       presenceVersion,
		ID:            selfID,
		Name:          strings.TrimSpace(identity.Name),
		Instance:      strings.TrimSpace(identity.Instance),
		MCVersion:     strings.TrimSpace(identity.MCVersion),
		InstanceID:    strings.TrimSpace(identity.InstanceID),
		Loader:        strings.TrimSpace(identity.Loader),
		LoaderVersion: strings.TrimSpace(identity.LoaderVersion),
		SyncPort:      identity.SyncPort,
		SyncToken:     strings.TrimSpace(identity.SyncToken),
		SyncDirs:      cleanDirs(identity.SyncDirs),
		ToolPort:      identity.ToolPort,
		ToolToken:     strings.TrimSpace(identity.ToolToken),
		Time:          time.Now().Unix(),
	}
	if packet.Name == "" {
		packet.Name = "Steve"
	}
	if len([]rune(packet.Name)) > 32 {
		packet.Name = string([]rune(packet.Name)[:32])
	}
	if len([]rune(packet.Instance)) > 64 {
		packet.Instance = string([]rune(packet.Instance)[:64])
	}
	return json.Marshal(packet)
}

func decodePresence(data []byte, selfID string) (presencePacket, error) {
	if len(data) == 0 || len(data) > maxPresenceSize {
		return presencePacket{}, errors.New("启动器玩家广播长度无效")
	}
	var packet presencePacket
	if err := json.Unmarshal(data, &packet); err != nil {
		return presencePacket{}, err
	}
	if packet.App != presenceApp || packet.Version != presenceVersion {
		return presencePacket{}, errors.New("不是本启动器的玩家广播")
	}
	if packet.ID == "" || packet.ID == selfID {
		return presencePacket{}, errors.New("忽略自身广播")
	}
	packet.Name = strings.TrimSpace(packet.Name)
	if packet.Name == "" {
		packet.Name = "Steve"
	}
	packet.Instance = strings.TrimSpace(packet.Instance)
	packet.MCVersion = strings.TrimSpace(packet.MCVersion)
	packet.InstanceID = strings.TrimSpace(packet.InstanceID)
	packet.Loader = strings.TrimSpace(packet.Loader)
	packet.LoaderVersion = strings.TrimSpace(packet.LoaderVersion)
	if packet.SyncPort < 0 || packet.SyncPort > 65535 {
		packet.SyncPort = 0
	}
	packet.SyncToken = strings.TrimSpace(packet.SyncToken)
	if len(packet.SyncToken) > 128 {
		packet.SyncToken = ""
	}
	packet.SyncDirs = cleanDirs(packet.SyncDirs)
	if packet.ToolPort < 0 || packet.ToolPort > 65535 {
		packet.ToolPort = 0
	}
	packet.ToolToken = strings.TrimSpace(packet.ToolToken)
	if len(packet.ToolToken) > 128 {
		packet.ToolToken = ""
	}
	return packet, nil
}

func cleanDirs(dirs []string) []string {
	out := make([]string, 0, len(dirs))
	seen := map[string]bool{}
	for _, dir := range dirs {
		dir = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")))
		dir = strings.Trim(dir, "/")
		if dir == "" || seen[dir] || len(dir) > 32 {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
		if len(out) >= 8 {
			break
		}
	}
	return out
}
