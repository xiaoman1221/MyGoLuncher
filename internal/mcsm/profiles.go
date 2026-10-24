package mcsm

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/secretstore"
	"MyMinecraftGoLuncher/internal/util"
)

type profileDB struct {
	Profiles []Profile `json:"profiles"`
}

// Profiles manages the non-sensitive profile list and its encrypted API keys.
type Profiles struct {
	mu      sync.Mutex
	path    string
	secrets secretstore.Store
	items   []Profile
}

// OpenProfiles loads servers.json. The file is optional on first run.
func OpenProfiles(path string, secrets secretstore.Store) (*Profiles, error) {
	if secrets == nil {
		secrets = secretstore.NewMemory()
	}
	p := &Profiles{path: path, secrets: secrets}
	var db profileDB
	if err := util.ReadJSON(path, &db); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取服务器配置失败: %w", err)
	}
	p.items = db.Profiles
	return p, nil
}

// List returns all profiles without API keys.
func (p *Profiles) List() []ProfileView {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ProfileView, 0, len(p.items))
	for _, item := range p.items {
		out = append(out, p.viewLocked(item))
	}
	return out
}

// Get returns one profile without its API key.
func (p *Profiles) Get(id string) (Profile, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range p.items {
		if item.ID == id {
			return item, true
		}
	}
	return Profile{}, false
}

// APIKey returns the stored key for a profile, or an empty string.
func (p *Profiles) APIKey(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", nil
	}
	return p.secrets.Get(ProfileSecretName(id))
}

// Save creates or updates a profile. A blank APIKey preserves the existing key
// when the profile already exists.
func (p *Profiles) Save(req SaveProfileRequest) (ProfileView, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	idx := -1
	for i := range p.items {
		if p.items[i].ID == req.ID && req.ID != "" {
			idx = i
			break
		}
	}
	if req.ID != "" && idx < 0 {
		return ProfileView{}, errors.New("服务器配置不存在")
	}
	profile := Profile{}
	if idx >= 0 {
		profile = p.items[idx]
	} else {
		profile.ID = newProfileID()
		profile.CreatedAt = Now()
	}
	profile.Name = strings.TrimSpace(req.Name)
	profile.PanelURL = strings.TrimSpace(req.PanelURL)
	profile.DaemonID = strings.TrimSpace(req.DaemonID)
	profile.InstanceUUID = strings.TrimSpace(req.InstanceUUID)
	profile.LocalInstanceID = strings.TrimSpace(req.LocalInstanceID)
	profile.RemoteRoot = strings.TrimSpace(req.RemoteRoot)
	profile.SyncDirs = req.SyncDirs
	profile.DeleteExtras = req.DeleteExtras

	if err := normalizeProfile(&profile); err != nil {
		return ProfileView{}, err
	}
	if strings.TrimSpace(req.APIKey) != "" {
		if err := p.secrets.Set(ProfileSecretName(profile.ID), strings.TrimSpace(req.APIKey)); err != nil {
			return ProfileView{}, fmt.Errorf("保存 API Key 失败: %w", err)
		}
	} else if req.ClearAPIKey {
		if err := p.secrets.Delete(ProfileSecretName(profile.ID)); err != nil {
			return ProfileView{}, fmt.Errorf("删除 API Key 失败: %w", err)
		}
	}

	if idx >= 0 {
		p.items[idx] = profile
	} else {
		p.items = append(p.items, profile)
	}
	if err := p.saveLocked(); err != nil {
		return ProfileView{}, err
	}
	return p.viewLocked(profile), nil
}

// Delete removes a profile and its API key.
func (p *Profiles) Delete(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := -1
	for i := range p.items {
		if p.items[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("服务器配置不存在")
	}
	p.items = append(p.items[:idx], p.items[idx+1:]...)
	if err := p.secrets.Delete(ProfileSecretName(id)); err != nil {
		return fmt.Errorf("删除 API Key 失败: %w", err)
	}
	return p.saveLocked()
}

// AddBackup stores snapshot metadata for a profile and keeps only the newest
// max entries. It returns the backups that fell out of the window so the caller
// can delete their remote archives.
func (p *Profiles) AddBackup(id string, backup Backup, max int) ([]Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for idx := range p.items {
		if p.items[idx].ID != id {
			continue
		}
		if max <= 0 {
			max = 5
		}
		p.items[idx].Backups = append([]Backup{backup}, p.items[idx].Backups...)
		var removed []Backup
		if len(p.items[idx].Backups) > max {
			removed = append(removed, p.items[idx].Backups[max:]...)
			p.items[idx].Backups = p.items[idx].Backups[:max]
		}
		if err := p.saveLocked(); err != nil {
			return removed, err
		}
		return removed, nil
	}
	return nil, errors.New("服务器配置不存在")
}

// TrimBackups keeps only the newest max snapshots and returns the entries that
// were removed from the metadata.
func (p *Profiles) TrimBackups(id string, max int) ([]Backup, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if max <= 0 {
		max = 5
	}
	for idx := range p.items {
		if p.items[idx].ID != id {
			continue
		}
		if len(p.items[idx].Backups) <= max {
			return nil, nil
		}
		removed := append([]Backup(nil), p.items[idx].Backups[max:]...)
		p.items[idx].Backups = p.items[idx].Backups[:max]
		if err := p.saveLocked(); err != nil {
			return nil, err
		}
		return removed, nil
	}
	return nil, errors.New("服务器配置不存在")
}

// Backup returns one stored snapshot.
func (p *Profiles) Backup(id, backupID string) (Profile, Backup, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range p.items {
		if item.ID != id {
			continue
		}
		for _, backup := range item.Backups {
			if backup.ID == backupID {
				return item, backup, true
			}
		}
		return item, Backup{}, false
	}
	return Profile{}, Backup{}, false
}

// RemoveBackup drops one snapshot from a profile's history.
func (p *Profiles) RemoveBackup(id, backupID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for idx := range p.items {
		if p.items[idx].ID != id {
			continue
		}
		out := p.items[idx].Backups[:0]
		for _, backup := range p.items[idx].Backups {
			if backup.ID != backupID {
				out = append(out, backup)
			}
		}
		p.items[idx].Backups = out
		return p.saveLocked()
	}
	return errors.New("服务器配置不存在")
}

// MarkRolledBack records a successful rollback time.
func (p *Profiles) MarkRolledBack(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for idx := range p.items {
		if p.items[idx].ID == id {
			p.items[idx].LastRollback = Now()
			_ = p.saveLocked()
			return
		}
	}
}

// MarkSynced records a successful synchronization time.
func (p *Profiles) MarkSynced(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range p.items {
		if p.items[i].ID == id {
			p.items[i].LastSync = Now()
			_ = p.saveLocked()
			return
		}
	}
}

func (p *Profiles) viewLocked(profile Profile) ProfileView {
	key, err := p.secrets.Get(ProfileSecretName(profile.ID))
	return ProfileView{Profile: profile, APIKeySet: err == nil && strings.TrimSpace(key) != ""}
}

func (p *Profiles) saveLocked() error {
	if err := util.WriteJSON(p.path, profileDB{Profiles: p.items}); err != nil {
		return fmt.Errorf("保存服务器配置失败: %w", err)
	}
	return nil
}

func normalizeProfile(profile *Profile) error {
	panel, err := normalizePanelURL(profile.PanelURL)
	if err != nil {
		return err
	}
	profile.PanelURL = panel
	if profile.Name == "" {
		profile.Name = profile.DaemonID
	}
	if profile.Name == "" {
		profile.Name = "MCSM 服务器"
	}
	if len([]rune(profile.Name)) > 64 {
		profile.Name = string([]rune(profile.Name)[:64])
	}
	if profile.DaemonID == "" {
		return errors.New("请填写 MCSM 节点 ID")
	}
	if profile.InstanceUUID == "" {
		return errors.New("请填写 MCSM 实例 UUID")
	}
	if profile.LocalInstanceID == "" {
		return errors.New("请选择要同步的本地版本")
	}
	root, err := normalizeRemoteRoot(profile.RemoteRoot)
	if err != nil {
		return err
	}
	profile.RemoteRoot = root
	dirs, err := normalizeSyncDirs(profile.SyncDirs)
	if err != nil {
		return err
	}
	profile.SyncDirs = dirs
	return nil
}

func normalizePanelURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("请填写 MCSM 面板地址")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("面板地址无效: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("面板地址必须以 http:// 或 https:// 开头")
	}
	if u.Host == "" {
		return "", errors.New("面板地址缺少主机名")
	}
	if u.User != nil {
		return "", errors.New("面板地址不能包含用户名或密码")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("面板地址不能包含查询参数或片段")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeRemoteRoot(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" {
		return "/", nil
	}
	for _, segment := range strings.Split(raw, "/") {
		if segment == ".." {
			return "", errors.New("远端目录不能包含 ..")
		}
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	return path.Clean(raw), nil
}

func normalizeSyncDirs(dirs []string) ([]string, error) {
	if len(dirs) == 0 {
		return []string{DirMods, DirConfig, DirDefaultConfig, DirResourcepacks}, nil
	}
	allowed := make(map[string]bool, len(AllowedSyncDirs))
	for _, dir := range AllowedSyncDirs {
		allowed[dir] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")))
		dir = strings.Trim(dir, "/")
		if dir == "" {
			continue
		}
		if !allowed[dir] {
			return nil, fmt.Errorf("不支持的同步目录: %s", dir)
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	if len(out) == 0 {
		return nil, errors.New("请至少选择一个同步目录")
	}
	// Keep the canonical order for stable UI and progress output.
	ordered := make([]string, 0, len(out))
	for _, dir := range AllowedSyncDirs {
		if seen[dir] {
			ordered = append(ordered, dir)
		}
	}
	return ordered, nil
}

func newProfileID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
