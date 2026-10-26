// Package lansync shares a local launcher instance with other clients on the
// LAN over a short-lived, token-protected HTTP endpoint.
package lansync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	DirAll           = "*"
)

// AllowedDirs is the complete set of directories that may be shared. DirAll
// shares the complete isolated version directory, including saves and options.
var AllowedDirs = []string{DirMods, DirShaderpacks, DirResourcepacks, DirConfig, DirAll}

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
	Full      bool         `json:"full,omitempty"`
	Files     []FileEntry  `json:"files"`
	CreatedAt string       `json:"createdAt"`
}

// FileChange describes one file that a LAN sync would write.
type FileChange struct {
	Path       string `json:"path"`
	State      string `json:"state"` // new or changed
	Reason     string `json:"reason,omitempty"`
	LocalSize  int64  `json:"localSize"`
	RemoteSize int64  `json:"remoteSize"`
}

// Plan is a read-only comparison between a peer manifest and a local instance.
type Plan struct {
	TotalFiles   int          `json:"totalFiles"`
	NewFiles     int          `json:"newFiles"`
	ChangedFiles int          `json:"changedFiles"`
	SameFiles    int          `json:"sameFiles"`
	ExtraFiles   int          `json:"extraFiles"`
	NewBytes     int64        `json:"newBytes"`
	ChangedBytes int64        `json:"changedBytes"`
	Changes      []FileChange `json:"changes"`
	Extras       []string     `json:"extras"`
	Digest       string       `json:"digest"`
}

// BuildPlan compares a manifest with localDir without writing anything.
func BuildPlan(localDir string, manifest Manifest) (Plan, error) {
	var plan Plan
	stat, err := os.Stat(localDir)
	if err != nil || !stat.IsDir() {
		return plan, errors.New("目标游戏目录不存在")
	}
	dirs, err := NormalizeDirs(manifest.Dirs)
	if err != nil {
		return plan, err
	}
	manifest.Dirs = dirs
	plan.Digest = manifestDigest(manifest)
	plan.TotalFiles = len(manifest.Files)
	plan.Changes = []FileChange{}
	plan.Extras = []string{}
	known := map[string]bool{}
	for _, entry := range manifest.Files {
		rel, err := safeRelativePath(entry.Path, manifest.Dirs)
		if err != nil {
			return plan, err
		}
		known[rel] = true
		full, err := localPath(localDir, rel)
		if err != nil {
			return plan, err
		}
		localInfo, statErr := os.Stat(full)
		if errors.Is(statErr, os.ErrNotExist) {
			plan.NewFiles++
			plan.NewBytes += entry.Size
			plan.appendChange(FileChange{Path: rel, State: "new", RemoteSize: entry.Size})
			continue
		}
		if statErr != nil {
			return plan, fmt.Errorf("读取本地文件 %s 失败: %w", rel, statErr)
		}
		if localInfo.IsDir() {
			return plan, fmt.Errorf("本地路径是目录: %s", rel)
		}
		if localInfo.Size() != entry.Size {
			plan.ChangedFiles++
			plan.ChangedBytes += entry.Size
			plan.appendChange(FileChange{
				Path: rel, State: "changed", Reason: "size",
				LocalSize: localInfo.Size(), RemoteSize: entry.Size,
			})
			continue
		}
		localHash, err := hashFile(full)
		if err != nil {
			return plan, fmt.Errorf("校验本地文件 %s 失败: %w", rel, err)
		}
		if !strings.EqualFold(localHash, entry.SHA256) {
			plan.ChangedFiles++
			plan.ChangedBytes += entry.Size
			plan.appendChange(FileChange{
				Path: rel, State: "changed", Reason: "hash",
				LocalSize: localInfo.Size(), RemoteSize: entry.Size,
			})
			continue
		}
		plan.SameFiles++
	}

	full := containsDir(manifest.Dirs, DirAll)
	for _, dir := range manifest.Dirs {
		base := filepath.Join(localDir, dir)
		if full && dir == DirAll {
			base = localDir
		}
		if stat, err := os.Stat(base); err != nil || !stat.IsDir() {
			continue
		}
		_ = filepath.WalkDir(base, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if full && filePath != base && skipFullDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || skipSyncName(entry.Name()) {
				return nil
			}
			rel, err := filepath.Rel(localDir, filePath)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if _, err := safeRelativePath(rel, manifest.Dirs); err != nil {
				return nil
			}
			if !known[rel] {
				plan.ExtraFiles++
				if len(plan.Extras) < 100 {
					plan.Extras = append(plan.Extras, rel)
				}
			}
			return nil
		})
	}
	if plan.Changes == nil {
		plan.Changes = []FileChange{}
	}
	if plan.Extras == nil {
		plan.Extras = []string{}
	}
	return plan, nil
}

func (p *Plan) appendChange(change FileChange) {
	if len(p.Changes) < 200 {
		p.Changes = append(p.Changes, change)
	}
}

// ManifestDigest is the confirmation digest for a peer manifest.
func ManifestDigest(manifest Manifest) string {
	return manifestDigest(manifest)
}

func containsDir(dirs []string, target string) bool {
	for _, dir := range dirs {
		if dir == target {
			return true
		}
	}
	return false
}

// skipFullDir excludes generated/runtime-heavy directories from a complete
// version share. Saves, options, mods, configs and resource packs stay.
func skipFullDir(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "logs", "crash-reports", "natives", "cache", ".cache", "temp", "downloads":
		return true
	}
	return false
}

func manifestDigest(manifest Manifest) string {
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
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
		if dir == DirAll {
			return []string{DirAll}, nil
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
	for _, dir := range dirs {
		if dir == DirAll {
			return cleaned, nil
		}
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
	lower := strings.ToLower(name)
	switch lower {
	case ".ds_store", "thumbs.db", "desktop.ini", ".gitkeep":
		return true
	}
	return strings.HasSuffix(lower, ".tmp") ||
		strings.HasSuffix(lower, ".old") || strings.HasSuffix(lower, ".dis") ||
		strings.Contains(lower, ".old.") || strings.Contains(lower, ".dis.")
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
