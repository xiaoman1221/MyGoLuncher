package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/hitokoto"
)

// Paths contains every directory owned by the launcher.
type Paths struct {
	Base       string `json:"base"`
	Root       string `json:"root"` // configuration directory (MyGoLuncher)
	Data       string `json:"data"`
	Instances  string `json:"instances"`
	Versions   string `json:"versions"`
	Libraries  string `json:"libraries"`
	Assets     string `json:"assets"`
	Runtimes   string `json:"runtimes"`
	Meta       string `json:"meta"`
	Cache      string `json:"cache"`
	Settings   string `json:"-"`
	InstanceDB string `json:"-"`
	Accounts   string `json:"-"`
}

// DefaultPaths uses a portable layout next to the running executable:
//
//	<base>/.minecraft     game files
//	<base>/MyGoLuncher    settings, instance index, accounts and cache
//
// MYGOLUNCHER_HOME can override the base directory for development or unusual
// installations. On macOS the base is the folder containing the .app bundle.
func DefaultPaths() (Paths, error) {
	override := strings.TrimSpace(os.Getenv("MYGOLUNCHER_HOME"))
	if override == "" {
		override = strings.TrimSpace(os.Getenv("MGL_HOME"))
	}
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return Paths{}, err
		}
		return PathsFor(abs), nil
	}
	exe, err := os.Executable()
	if err != nil {
		return Paths{}, fmt.Errorf("无法定位程序所在目录: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(exe); resolveErr == nil {
		exe = resolved
	}
	base := filepath.Dir(exe)
	if runtime.GOOS == "darwin" {
		// For MyMinecraftGoLuncher.app/Contents/MacOS/binary, use the folder
		// containing the .app bundle so data is not placed inside the bundle.
		current := exe
		for i := 0; i < 6; i++ {
			parent := filepath.Dir(current)
			if strings.HasSuffix(parent, ".app") {
				base = filepath.Dir(parent)
				break
			}
			current = parent
		}
	}
	if runtime.GOOS == "linux" {
		if appImage := strings.TrimSpace(os.Getenv("APPIMAGE")); appImage != "" {
			base = filepath.Dir(appImage)
		}
	}
	migrateLegacyConfig(base)
	return PathsFor(base), nil
}

// migrateLegacyConfig quietly moves the pre-rename MGL directory to the new
// MyGoLuncher name so existing accounts and versions keep working.
func migrateLegacyConfig(base string) {
	current := filepath.Join(base, "MyGoLuncher")
	legacy := filepath.Join(base, "MGL")
	if _, err := os.Stat(current); err == nil {
		return
	}
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		_ = os.Rename(legacy, current)
	}
}

// PathsFor builds the portable directory set next to base.
func PathsFor(base string) Paths {
	config := filepath.Join(base, "MyGoLuncher")
	data := filepath.Join(base, ".minecraft")
	return Paths{
		Base:       base,
		Root:       config,
		Data:       data,
		Instances:  filepath.Join(data, "instances"),
		Versions:   filepath.Join(data, "versions"),
		Libraries:  filepath.Join(data, "libraries"),
		Assets:     filepath.Join(data, "assets"),
		Runtimes:   filepath.Join(data, "runtimes"),
		Meta:       filepath.Join(data, "meta"),
		Cache:      filepath.Join(config, "cache"),
		Settings:   filepath.Join(config, "settings.json"),
		InstanceDB: filepath.Join(config, "instances.json"),
		Accounts:   filepath.Join(config, "accounts.json"),
	}
}

// Ensure creates all necessary directories.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.Base, p.Root, p.Data, p.Instances, p.Versions, p.Libraries, p.Assets, p.Runtimes, p.Meta, p.Cache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// MirrorConfig is a user-provided download mirror.
type MirrorConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Kind string `json:"kind"` // bmclapi or proxy
}

// Settings are user preferences persisted between runs.
type Settings struct {
	PlayerName           string         `json:"playerName"`
	DefaultMemoryMB      int            `json:"defaultMemoryMB"`
	DefaultJavaPath      string         `json:"defaultJavaPath"`
	CurseForgeAPIKey     string         `json:"curseForgeAPIKey"`
	MicrosoftClientID    string         `json:"microsoftClientID"`
	ExtraJVMArgs         string         `json:"extraJvmArgs"`
	CustomMirrors        []MirrorConfig `json:"customMirrors"`
	DownloadWorkers      int            `json:"downloadWorkers"`
	ShowGameConsole      bool           `json:"showGameConsole"`
	ShowLauncherOnLaunch bool           `json:"showLauncherOnLaunch"`
	HitokotoAPIURL       string         `json:"hitokotoApiUrl"`
	SchemaVersion        int            `json:"schemaVersion"`
	ConcurrencyWarned    bool           `json:"-"`
}

// DefaultSettings builds first-run preferences.
func DefaultSettings() Settings {
	return Settings{
		PlayerName:      "Steve",
		DefaultMemoryMB: 0, // automatic
		CustomMirrors:   []MirrorConfig{},
		DownloadWorkers: 8,
		ShowGameConsole: true,
		HitokotoAPIURL:  hitokoto.DefaultAPIURL,
		SchemaVersion:   2,
	}
}

// Instance is a playable, isolated game directory.
type Instance struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	MCVersion       string    `json:"mcVersion"`
	Loader          string    `json:"loader"`
	LoaderVersion   string    `json:"loaderVersion"`
	LaunchVersionID string    `json:"launchVersionId"`
	MemoryMB        int       `json:"memoryMB"`
	JavaPath        string    `json:"javaPath"`
	CreatedAt       time.Time `json:"createdAt"`
	LastPlayed      time.Time `json:"lastPlayed"`
	PlayTimeMinutes int       `json:"playTimeMinutes"`
	Source          string    `json:"source"`
	Notes           string    `json:"notes"`
}

// LoaderLabel returns a human readable loader name.
func (i Instance) LoaderLabel() string {
	switch strings.ToLower(i.Loader) {
	case "fabric":
		return "Fabric"
	case "quilt":
		return "Quilt"
	case "forge":
		return "Forge"
	case "neoforge":
		return "NeoForge"
	default:
		return "原版"
	}
}

// IsModded reports whether a non-vanilla loader is used.
func (i Instance) IsModded() bool {
	l := strings.ToLower(i.Loader)
	return l != "" && l != "vanilla"
}

// File name in instances.json.
type instanceDB struct {
	Instances []Instance `json:"instances"`
}

// Store is the thread-safe launcher state.
type Store struct {
	mu        sync.RWMutex
	paths     Paths
	settings  Settings
	instances []Instance
	loadErr   error
}

// Open loads settings and instances from disk.
func Open(paths Paths) (*Store, error) {
	if err := paths.Ensure(); err != nil {
		return nil, err
	}
	s := &Store{paths: paths, settings: DefaultSettings()}
	// Reset only the schema marker so a legacy settings file can be detected.
	s.settings.SchemaVersion = 0
	if err := loadJSON(paths.Settings, &s.settings); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.loadErr = fmt.Errorf("读取设置失败: %w", err)
	}
	settingsDirty := false
	if s.settings.DownloadWorkers <= 0 || s.settings.DownloadWorkers > 24 {
		s.settings.DownloadWorkers = 8
		settingsDirty = true
	}
	if s.settings.SchemaVersion < 2 {
		// Older builds used 4096 MB as the implicit default. Migrate that
		// default to the new automatic mode without touching custom values.
		if s.settings.DefaultMemoryMB == 4096 {
			s.settings.DefaultMemoryMB = 0
		}
		s.settings.SchemaVersion = 2
		settingsDirty = true
	}
	if s.settings.DefaultMemoryMB != 0 && (s.settings.DefaultMemoryMB < 512 || s.settings.DefaultMemoryMB > 65536) {
		s.settings.DefaultMemoryMB = 0
		settingsDirty = true
	}
	if s.settings.CustomMirrors == nil {
		s.settings.CustomMirrors = []MirrorConfig{}
	}
	if strings.TrimSpace(s.settings.HitokotoAPIURL) == "" {
		s.settings.HitokotoAPIURL = hitokoto.DefaultAPIURL
		settingsDirty = true
	}
	var db instanceDB
	if err := loadJSON(paths.InstanceDB, &db); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.loadErr = fmt.Errorf("读取版本列表失败: %w", err)
	}
	s.instances = db.Instances
	instancesDirty := false
	for idx := range s.instances {
		if s.instances[idx].Loader == "" {
			s.instances[idx].Loader = "vanilla"
		}
		if s.instances[idx].LaunchVersionID == "" {
			s.instances[idx].LaunchVersionID = s.instances[idx].MCVersion
			instancesDirty = true
		}
		if s.instances[idx].ID == "" {
			s.instances[idx].ID = newID()
			instancesDirty = true
		}
		if strings.Contains(s.instances[idx].Name, "实例") {
			s.instances[idx].Name = strings.ReplaceAll(s.instances[idx].Name, "实例", "版本")
			instancesDirty = true
		}
		if strings.Contains(s.instances[idx].Notes, "实例") {
			s.instances[idx].Notes = strings.ReplaceAll(s.instances[idx].Notes, "实例", "版本")
			instancesDirty = true
		}
	}
	s.sortInstancesLocked()
	if settingsDirty {
		_ = saveJSON(paths.Settings, s.settings)
	}
	if instancesDirty {
		_ = saveJSON(paths.InstanceDB, instanceDB{Instances: s.instances})
	}
	if s.loadErr != nil {
		return s, s.loadErr
	}
	return s, nil
}

// ResetLoadError clears a non-fatal load warning once the user has seen it.
func (s *Store) ResetLoadError() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = nil
}

func loadJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return unmarshal(data, v)
}

func (s *Store) Paths() Paths {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paths
}

// DataDir returns the root of shared game data.
func (s *Store) DataDir() string { return s.Paths().Data }

// InstanceDir returns the isolated game directory for an instance.
func (s *Store) InstanceDir(id string) string {
	return filepath.Join(s.Paths().Instances, id)
}

// Settings returns a copy of current settings.
func (s *Store) Settings() Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// SaveSettings validates and persists settings.
func (s *Store) SaveSettings(next Settings) (Settings, error) {
	next.PlayerName = strings.TrimSpace(next.PlayerName)
	if next.PlayerName == "" {
		next.PlayerName = "Steve"
	}
	if len([]rune(next.PlayerName)) > 16 {
		next.PlayerName = string([]rune(next.PlayerName)[:16])
	}
	if next.DefaultMemoryMB != 0 && next.DefaultMemoryMB < 512 {
		return Settings{}, errors.New("默认内存必须为自动或至少 512 MB")
	}
	if next.DefaultMemoryMB > 65536 {
		return Settings{}, errors.New("默认内存不能超过 65536 MB")
	}
	if next.DownloadWorkers < 1 || next.DownloadWorkers > 24 {
		next.DownloadWorkers = 8
	}
	next.CurseForgeAPIKey = strings.TrimSpace(next.CurseForgeAPIKey)
	next.ExtraJVMArgs = strings.TrimSpace(next.ExtraJVMArgs)
	next.DefaultJavaPath = strings.TrimSpace(next.DefaultJavaPath)
	next.MicrosoftClientID = strings.TrimSpace(next.MicrosoftClientID)
	next.HitokotoAPIURL = strings.TrimSpace(next.HitokotoAPIURL)
	if next.HitokotoAPIURL == "" {
		next.HitokotoAPIURL = hitokoto.DefaultAPIURL
	}
	if next.CustomMirrors == nil {
		next.CustomMirrors = []MirrorConfig{}
	}
	next.SchemaVersion = 2
	s.mu.Lock()
	s.settings = next
	err := saveJSON(s.paths.Settings, next)
	s.mu.Unlock()
	return next, err
}

// Instances returns instances sorted by most recently played.
func (s *Store) Instances() []Instance {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Instance, len(s.instances))
	copy(out, s.instances)
	return out
}

// Instance returns one instance.
func (s *Store) Instance(id string) (Instance, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, inst := range s.instances {
		if inst.ID == id {
			return inst, true
		}
	}
	return Instance{}, false
}

// CreateInstance stores a new instance.
func (s *Store) CreateInstance(inst Instance) (Instance, error) {
	inst.ID = strings.TrimSpace(inst.ID)
	if inst.ID == "" {
		inst.ID = newID()
	}
	if _, ok := s.Instance(inst.ID); ok {
		return Instance{}, errors.New("版本 ID 冲突，请重试")
	}
	inst.Name = strings.TrimSpace(inst.Name)
	if inst.Name == "" {
		inst.Name = inst.MCVersion + " 版本"
	}
	if _, err := SafeName(inst.Name); err != nil {
		return Instance{}, err
	}
	if inst.Loader == "" {
		inst.Loader = "vanilla"
	}
	inst.Loader = strings.ToLower(inst.Loader)
	if inst.LaunchVersionID == "" && inst.Loader == "vanilla" {
		inst.LaunchVersionID = inst.MCVersion
	}
	if inst.MemoryMB < 0 || (inst.MemoryMB > 0 && inst.MemoryMB < 512) {
		return Instance{}, errors.New("版本内存必须为自动或至少 512 MB")
	}
	if inst.MemoryMB > 65536 {
		return Instance{}, errors.New("版本内存不能超过 65536 MB")
	}
	if inst.CreatedAt.IsZero() {
		inst.CreatedAt = time.Now()
	}
	if inst.Source == "" {
		inst.Source = "manual"
	}
	if err := os.MkdirAll(s.InstanceDir(inst.ID), 0o755); err != nil {
		return Instance{}, err
	}
	for _, sub := range []string{"mods", "shaderpacks", "resourcepacks", "saves", "logs"} {
		if err := os.MkdirAll(filepath.Join(s.InstanceDir(inst.ID), sub), 0o755); err != nil {
			return Instance{}, err
		}
	}
	s.mu.Lock()
	s.instances = append(s.instances, inst)
	s.sortInstancesLocked()
	err := saveJSON(s.paths.InstanceDB, instanceDB{Instances: s.instances})
	s.mu.Unlock()
	return inst, err
}

// UpdateInstance applies a mutator to an existing instance.
func (s *Store) UpdateInstance(id string, mutate func(*Instance) error) (Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for idx := range s.instances {
		if s.instances[idx].ID != id {
			continue
		}
		next := s.instances[idx]
		if err := mutate(&next); err != nil {
			return Instance{}, err
		}
		next.Name = strings.TrimSpace(next.Name)
		if next.Name == "" {
			return Instance{}, errors.New("版本名称不能为空")
		}
		if _, err := SafeName(next.Name); err != nil {
			return Instance{}, err
		}
		if next.MemoryMB < 0 || (next.MemoryMB > 0 && next.MemoryMB < 512) {
			return Instance{}, errors.New("版本内存必须为自动或至少 512 MB")
		}
		if next.MemoryMB > 65536 {
			return Instance{}, errors.New("版本内存不能超过 65536 MB")
		}
		s.instances[idx] = next
		s.sortInstancesLocked()
		err := saveJSON(s.paths.InstanceDB, instanceDB{Instances: s.instances})
		return next, err
	}
	return Instance{}, errors.New("版本不存在")
}

// MarkPlayed updates last played time and minutes.
func (s *Store) MarkPlayed(id string, minutes int) {
	_, _ = s.UpdateInstance(id, func(i *Instance) error {
		i.LastPlayed = time.Now()
		if minutes > 0 {
			i.PlayTimeMinutes += minutes
		}
		return nil
	})
}

// DeleteInstance removes an instance and, optionally, its files.
func (s *Store) DeleteInstance(id string, deleteFiles bool) error {
	s.mu.Lock()
	idx := -1
	for i := range s.instances {
		if s.instances[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return errors.New("版本不存在")
	}
	s.instances = append(s.instances[:idx], s.instances[idx+1:]...)
	err := saveJSON(s.paths.InstanceDB, instanceDB{Instances: s.instances})
	dir := filepath.Join(s.paths.Instances, id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if deleteFiles {
		return os.RemoveAll(dir)
	}
	return nil
}

func (s *Store) sortInstancesLocked() {
	sort.SliceStable(s.instances, func(a, b int) bool {
		left, right := s.instances[a], s.instances[b]
		if !left.LastPlayed.Equal(right.LastPlayed) {
			return left.LastPlayed.After(right.LastPlayed)
		}
		return left.CreatedAt.After(right.CreatedAt)
	})
}

func newID() string {
	var b [5]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// SafeName rejects path separators and reserved characters in user-facing names.
func SafeName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", errors.New("名称不能为空")
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) {
		return "", errors.New("名称不能包含 / \\ : * ? \" < > |")
	}
	if len([]rune(name)) > 60 {
		return "", errors.New("名称不能超过 60 个字符")
	}
	return name, nil
}
