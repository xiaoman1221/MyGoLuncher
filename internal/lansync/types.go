// Package lansync shares a local launcher instance with other clients on the
// LAN over a short-lived, token-protected HTTP endpoint.
package lansync

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	DirMods          = "mods"
	DirShaderpacks   = "shaderpacks"
	DirResourcepacks = "resourcepacks"
	DirConfig        = "config"
)

// AllowedDirs is the complete set of directories that may be shared.
var AllowedDirs = []string{DirMods, DirShaderpacks, DirResourcepacks, DirConfig}

// DefaultDirs is the safe default: mods, shaders and resource packs.
var DefaultDirs = []string{DirMods, DirShaderpacks, DirResourcepacks}

// InstanceInfo identifies the shared instance.
type InstanceInfo struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	MCVersion       string `json:"mcVersion"`
	Loader          string `json:"loader"`
	LoaderVersion   string `json:"loaderVersion"`
	LaunchVersionID string `json:"launchVersionId"`
}

// FileEntry is one file in a shared instance manifest.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest describes a shared instance.
type Manifest struct {
	Instance  InstanceInfo `json:"instance"`
	Dirs      []string     `json:"dirs"`
	Files     []FileEntry  `json:"files"`
	CreatedAt string       `json:"createdAt"`
}

// SyncResult summarizes a receiver-side sync.
type SyncResult struct {
	Downloaded int   `json:"downloaded"`
	Skipped    int   `json:"skipped"`
	Bytes      int64 `json:"bytes"`
}

// ProgressFunc reports long-running progress. current/total are bytes for
// downloads and files for scanning.
type ProgressFunc func(stage, message string, current, total int64)

// NormalizeDirs validates, deduplicates and canonicalizes shared directories.
func NormalizeDirs(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return append([]string(nil), DefaultDirs...), nil
	}
	allowed := map[string]bool{}
	for _, dir := range AllowedDirs {
		allowed[dir] = true
	}
	seen := map[string]bool{}
	for _, dir := range dirs {
		dir = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")))
		dir = strings.Trim(dir, "/")
		if dir == "" {
			continue
		}
		if !allowed[dir] {
			return nil, fmt.Errorf("不支持共享的目录: %s", dir)
		}
		seen[dir] = true
	}
	if len(seen) == 0 {
		return nil, errors.New("请至少选择一个共享目录")
	}
	out := make([]string, 0, len(seen))
	for _, dir := range AllowedDirs {
		if seen[dir] {
			out = append(out, dir)
		}
	}
	return out, nil
}

func safeRelativePath(rel string, dirs []string) (string, error) {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" || len(rel) > 1024 || strings.HasPrefix(rel, "/") {
		return "", errors.New("共享路径无效")
	}
	for _, r := range rel {
		if r < 0x20 || r == 0x7f {
			return "", errors.New("共享路径包含控制字符")
		}
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", errors.New("共享路径包含非法片段")
		}
	}
	cleaned := path.Clean(rel)
	if cleaned == "." || cleaned != rel {
		return "", errors.New("共享路径未规范化")
	}
	top := strings.SplitN(cleaned, "/", 2)[0]
	for _, dir := range dirs {
		if top == dir {
			return cleaned, nil
		}
	}
	return "", errors.New("共享路径不在允许的目录内")
}

func localPath(root, rel string) (string, error) {
	rel = filepath.FromSlash(rel)
	full := filepath.Join(root, rel)
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	within, err := filepath.Rel(rootAbs, fullAbs)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", errors.New("共享路径越界")
	}
	return fullAbs, nil
}

func skipSyncName(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db", "desktop.ini", ".gitkeep":
		return true
	}
	return strings.HasSuffix(strings.ToLower(name), ".tmp")
}

func hashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func nowString() string {
	return time.Now().Format(time.RFC3339)
}
