// Package launcherprofiles keeps the portable .minecraft folder usable by the
// official Minecraft launcher and other launchers that read
// launcher_profiles.json. Each MyGoLuncher instance is mirrored as a profile
// whose gameDir points at the instance directory.
package launcherprofiles

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/store"
)

const profilePrefix = "mygoluncher-"

// Sync merges the current instances into <mcDir>/launcher_profiles.json.
// Unknown profiles and launcher settings are preserved so this file can be
// shared with a normal Minecraft installation.
func Sync(mcDir string, instances []store.Instance, isolation bool) error {
	mcDir = strings.TrimSpace(mcDir)
	if mcDir == "" {
		return errors.New("游戏目录为空")
	}
	if err := os.MkdirAll(mcDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(mcDir, "launcher_profiles.json")
	doc := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(raw))) > 0 {
		if jsonErr := json.Unmarshal(raw, &doc); jsonErr != nil {
			// A malformed file is replaced, but keep a portable .bak copy so the
			// user does not lose their previous profiles silently.
			_ = os.WriteFile(path+".bak", raw, 0o644)
			doc = map[string]any{}
		}
	}

	profiles := map[string]any{}
	if existing, ok := doc["profiles"].(map[string]any); ok {
		profiles = existing
	}
	for key := range profiles {
		if strings.HasPrefix(key, profilePrefix) {
			delete(profiles, key)
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, inst := range instances {
		if strings.TrimSpace(inst.ID) == "" {
			continue
		}
		launchID := strings.TrimSpace(inst.LaunchVersionID)
		if launchID == "" {
			launchID = strings.TrimSpace(inst.MCVersion)
		}
		if launchID == "" {
			continue
		}
		gameDir := "."
		lastVersionID := launchID
		if isolation {
			versionID := strings.TrimSpace(inst.VersionID)
			if versionID == "" {
				versionID = inst.ID
			}
			gameDir = "versions/" + versionID
			customJSON := filepath.Join(mcDir, "versions", versionID, versionID+".json")
			if _, err := os.Stat(customJSON); err == nil {
				lastVersionID = versionID
			}
		}
		created := now
		if !inst.CreatedAt.IsZero() {
			created = inst.CreatedAt.UTC().Format(time.RFC3339)
		}
		lastUsed := created
		if !inst.LastPlayed.IsZero() {
			lastUsed = inst.LastPlayed.UTC().Format(time.RFC3339)
		}
		profile := map[string]any{
			"name":          inst.Name,
			"type":          "custom",
			"created":       created,
			"lastUsed":      lastUsed,
			"lastVersionId": lastVersionID,
			"gameDir":       gameDir,
		}
		// Deliberately do not write javaDir: the launcher's managed Java path
		// is absolute and would break as soon as the .minecraft folder is
		// copied to another user or computer. Other launchers can auto-detect
		// Java, and MyGoLuncher keeps using its own setting.
		if inst.MemoryMB >= 512 {
			profile["javaArgs"] = fmt.Sprintf("-Xms%dM -Xmx%dM", min(inst.MemoryMB/2, 1024), inst.MemoryMB)
		}
		profiles[profilePrefix+inst.ID] = profile
	}
	doc["profiles"] = profiles
	if _, ok := doc["launcherVersion"]; !ok {
		doc["launcherVersion"] = map[string]any{
			"name": "MyGoLuncher", "format": 21, "profilesFormat": 3,
		}
	}
	if _, ok := doc["settings"]; !ok {
		doc["settings"] = map[string]any{
			"enableSnapshots": false, "enableAdvanced": true,
			"keepLauncherOpen": false, "showGameLog": false,
		}
	}
	if _, ok := doc["selectedUser"]; !ok {
		doc["selectedUser"] = map[string]any{"account": "", "profile": ""}
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp, err := os.CreateTemp(mcDir, ".launcher_profiles-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// os.Rename cannot replace an existing file on every Windows Go version.
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(tmpName, path)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
