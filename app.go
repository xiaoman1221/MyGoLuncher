package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"MyMinecraftGoLuncher/internal/auth"
	"MyMinecraftGoLuncher/internal/external"
	"MyMinecraftGoLuncher/internal/gameoptions"
	"MyMinecraftGoLuncher/internal/hitokoto"
	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/java"
	"MyMinecraftGoLuncher/internal/lan"
	"MyMinecraftGoLuncher/internal/lantools"
	"MyMinecraftGoLuncher/internal/launcher"
	"MyMinecraftGoLuncher/internal/launcherprofiles"
	licensepkg "MyMinecraftGoLuncher/internal/license"
	"MyMinecraftGoLuncher/internal/loader"
	"MyMinecraftGoLuncher/internal/mcsm"
	"MyMinecraftGoLuncher/internal/mirror"
	"MyMinecraftGoLuncher/internal/modpack"
	"MyMinecraftGoLuncher/internal/modrinth"
	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/secretstore"
	securitypkg "MyMinecraftGoLuncher/internal/security"
	"MyMinecraftGoLuncher/internal/serverpack"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/sysinfo"
	"MyMinecraftGoLuncher/internal/sysutil"
	"MyMinecraftGoLuncher/internal/tray"
)

const appVersion = "1.0.2"

// App binds backend services to the Wails frontend.
type App struct {
	ctx context.Context

	paths     store.Paths
	store     *store.Store
	accounts  *auth.Store
	servers   *mcsm.Profiles
	serverErr string
	lanSvc    *lan.Service
	mojang    *mojang.Client
	javaMgr   *java.Manager
	loaders   *loader.Installer
	launcher  *launcher.Launcher
	license   *licensepkg.Manager
	initError string
	elevated  bool

	elevationMu        sync.Mutex
	elevationConfirmed bool

	syncMu sync.Mutex

	lanShare  lanShareState
	lanSyncMu sync.Mutex

	lanToolMu sync.Mutex
	lanTool   *lantools.Server

	orcaMu      sync.Mutex
	orcaCancel  context.CancelFunc
	orcaLoginID int

	installMu sync.Mutex
	taskMu    sync.Mutex
	taskLogs  map[string]*taskLog
	authMu    sync.Mutex
	msCancel  context.CancelFunc
	yssCancel context.CancelFunc

	totalMemoryMB int
	autoMemoryMB  int
	autoHidden    bool

	procMu     sync.Mutex
	procs      map[string]*launcher.Process
	logs       map[string]*logRing
	gameJob    *sysutil.Job
	trayIcon   *tray.Tray
	allowClose atomic.Bool
	uiReady    atomic.Bool
}

type logRing struct {
	mu    sync.Mutex
	lines []string
}

func (r *logRing) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) >= 600 {
		copy(r.lines, r.lines[len(r.lines)-400:])
		r.lines = r.lines[:400]
	}
	r.lines = append(r.lines, line)
}

func (r *logRing) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// NewApp constructs the application and loads persisted state.
func NewApp() *App {
	a := &App{
		procs: map[string]*launcher.Process{}, logs: map[string]*logRing{},
		elevated: securitypkg.IsElevated(),
		gameJob:  sysutil.NewKillOnCloseJob(),
	}
	a.totalMemoryMB = sysinfo.TotalMemoryMB()
	a.autoMemoryMB = sysinfo.RecommendedMemoryMB(a.totalMemoryMB)
	paths, err := store.DefaultPaths()
	if err != nil {
		a.initError = err.Error()
		return a
	}
	a.paths = paths
	secrets, err := secretstore.New()
	if err != nil {
		a.initError = "初始化安全存储失败: " + err.Error()
		return a
	}
	st, err := store.OpenWithSecrets(paths, secrets)
	if err != nil && st == nil {
		a.initError = err.Error()
		return a
	}
	a.store = st
	if st == nil {
		return a
	}
	licenseMgr, licenseErr := licensepkg.NewManager(secrets, appVersion)
	if licenseMgr == nil {
		a.initError = "初始化授权模块失败: " + licenseErr.Error()
		return a
	}
	a.license = licenseMgr
	accountStore, err := auth.OpenWithSecrets(secrets, paths.Accounts, st.Settings().PlayerName)
	if err != nil {
		a.initError = err.Error()
		return a
	}
	a.accounts = accountStore
	if profiles, profileErr := mcsm.OpenProfiles(paths.Servers, secrets); profileErr != nil {
		a.serverErr = profileErr.Error()
	} else {
		a.servers = profiles
	}
	a.lanSvc = lan.New()
	a.mojang = mojang.New(paths)
	a.javaMgr = java.NewManager(paths)
	a.loaders = loader.New(a.mojang, paths)
	a.launcher = launcher.New(a.mojang, paths)
	a.applyMirrorSources()
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.configureUpdates()
	if err := securitypkg.Guard(); err != nil {
		a.initError = err.Error()
		return
	}
	go a.validateLicenseOnStartup()
	a.syncLauncherProfiles()
	// LAN discovery is a paid feature. Start/stop it as entitlement changes.
	a.syncLanEntitlement()
	if icon, err := tray.New(a.showMainWindow, a.QuitApp); err == nil {
		a.trayIcon = icon
	}
}

// syncLauncherProfiles keeps .minecraft/launcher_profiles.json in sync with
// the instance list. The file makes the portable .minecraft folder directly
// usable by the official launcher and launchers that understand its profile
// format, without requiring MyGoLuncher to be running.
func (a *App) syncLauncherProfiles() {
	if a.store == nil {
		return
	}
	a.ensurePortableVersions()
	settings := a.store.Settings()
	_ = launcherprofiles.Sync(a.paths.Data, a.store.Instances(), settings.VersionIsolation)
}

// SyncLauncherProfiles regenerates the portable launcher profile file.
func (a *App) SyncLauncherProfiles() error {
	if err := a.ready(); err != nil {
		return err
	}
	a.ensurePortableVersions()
	settings := a.store.Settings()
	return launcherprofiles.Sync(a.paths.Data, a.store.Instances(), settings.VersionIsolation)
}

// RequestClose handles the custom title-bar close button according to the
// saved close behavior. "ask" emits app:close-request so the frontend can show
// the confirmation dialog.
func (a *App) RequestClose() {
	switch a.closeBehavior() {
	case "exit":
		a.QuitApp()
	case "minimize":
		a.MinimizeWindow()
	default:
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "app:close-request")
		}
	}
}

// MinimizeWindow hides the launcher in the notification area without touching
// running games. If the tray icon could not be created, fall back to a normal
// taskbar minimize.
func (a *App) MinimizeWindow() {
	if a.ctx == nil {
		return
	}
	if a.trayIcon != nil {
		runtime.WindowHide(a.ctx)
		return
	}
	runtime.WindowMinimise(a.ctx)
}

func (a *App) showMainWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// QuitApp bypasses the close confirmation and exits the launcher. Shutdown
// then terminates every game process owned by the launcher.
func (a *App) QuitApp() {
	a.allowClose.Store(true)
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

func (a *App) closeBehavior() string {
	if a.store == nil {
		return "ask"
	}
	behavior := strings.ToLower(strings.TrimSpace(a.store.Settings().CloseBehavior))
	switch behavior {
	case "exit", "minimize":
		return behavior
	default:
		return "ask"
	}
}

// beforeClose intercepts the native window close button / Alt+F4. Returning
// true keeps the window open while the frontend asks what to do.
func (a *App) beforeClose(ctx context.Context) bool {
	if a.allowClose.Load() || !a.uiReady.Load() {
		return false
	}
	switch a.closeBehavior() {
	case "exit":
		return false
	case "minimize":
		a.MinimizeWindow()
		return true
	default:
		runtime.EventsEmit(ctx, "app:close-request")
		return true
	}
}

func (a *App) shutdown(ctx context.Context) {
	// Stop games first: closing the launcher must not leave a Minecraft JVM
	// running if later shutdown steps block or take too long.
	a.stopAllGames()
	if a.trayIcon != nil {
		a.trayIcon.Close()
		a.trayIcon = nil
	}
	a.lanShare.mu.Lock()
	if a.lanShare.server != nil {
		a.lanShare.server.Stop()
		a.lanShare.server = nil
		a.lanShare.instanceID = ""
		a.lanShare.dirs = nil
	}
	a.lanShare.mu.Unlock()
	a.stopLanTools()
	if a.lanSvc != nil {
		a.lanSvc.Stop()
	}
	a.orcaMu.Lock()
	if a.orcaCancel != nil {
		a.orcaCancel()
		a.orcaCancel = nil
	}
	a.orcaMu.Unlock()
	a.authMu.Lock()
	if a.msCancel != nil {
		a.msCancel()
		a.msCancel = nil
	}
	if a.yssCancel != nil {
		a.yssCancel()
		a.yssCancel = nil
	}
	a.authMu.Unlock()
}

// stopAllGames explicitly terminates every game process and then closes the
// Windows job object as a final safety net.
func (a *App) stopAllGames() {
	a.procMu.Lock()
	procs := make([]*launcher.Process, 0, len(a.procs))
	for _, p := range a.procs {
		procs = append(procs, p)
	}
	a.procMu.Unlock()
	for _, p := range procs {
		_ = p.Stop()
	}
	if a.gameJob != nil {
		_ = a.gameJob.Close()
	}
}

func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) baseReady() error {
	if a.initError != "" {
		return errors.New(a.initError)
	}
	if a.store == nil || a.accounts == nil || a.mojang == nil || a.javaMgr == nil || a.loaders == nil || a.launcher == nil || a.license == nil {
		return errors.New("启动器尚未完成初始化")
	}
	return nil
}

// ready performs the common initialization and elevated-run confirmation gate.
// Free features must remain usable without a license; paid APIs call
// requireFeature separately.
func (a *App) ready() error {
	if err := a.baseReady(); err != nil {
		return err
	}
	a.elevationMu.Lock()
	needsElevationConfirm := a.elevated && !a.elevationConfirmed
	a.elevationMu.Unlock()
	if needsElevationConfirm {
		return errors.New("请先确认管理员权限运行提示")
	}
	return nil
}

func (a *App) emitInstancesChanged() {
	a.syncLauncherProfiles()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "instances:changed", nil)
	}
}

// PathsView is a serializable view of the launcher directories.
type PathsView struct {
	Base      string `json:"base"`
	Root      string `json:"root"`
	Data      string `json:"data"`
	Instances string `json:"instances"`
	Versions  string `json:"versions"`
	Libraries string `json:"libraries"`
	Assets    string `json:"assets"`
	Runtimes  string `json:"runtimes"`
	Cache     string `json:"cache"`
}

// InstanceView adds live state to a stored instance. It deliberately uses
// strings for timestamps so the generated Wails bindings stay fully typed.
type InstanceView struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	MCVersion         string `json:"mcVersion"`
	Loader            string `json:"loader"`
	LoaderVersion     string `json:"loaderVersion"`
	LaunchVersionID   string `json:"launchVersionId"`
	MemoryMB          int    `json:"memoryMB"`
	JavaPath          string `json:"javaPath"`
	CreatedAt         string `json:"createdAt"`
	LastPlayed        string `json:"lastPlayed"`
	PlayTimeMinutes   int    `json:"playTimeMinutes"`
	Source            string `json:"source"`
	Notes             string `json:"notes"`
	GameDir           string `json:"gameDir"`
	RequiredJava      int    `json:"requiredJava"`
	VersionInstalled  bool   `json:"versionInstalled"`
	Running           bool   `json:"running"`
	ModCount          int    `json:"modCount"`
	ShaderCount       int    `json:"shaderCount"`
	ResourcePackCount int    `json:"resourcePackCount"`
	HasJavaOverride   bool   `json:"hasJavaOverride"`
	AutoMemory        bool   `json:"autoMemory"`
	EffectiveMemoryMB int    `json:"effectiveMemoryMB"`
}

// Bootstrap is loaded once by the frontend.
type Bootstrap struct {
	Settings           store.Settings    `json:"settings"`
	Instances          []InstanceView    `json:"instances"`
	Paths              PathsView         `json:"paths"`
	Version            string            `json:"version"`
	InitError          string            `json:"initError"`
	Running            []string          `json:"running"`
	Accounts           []auth.View       `json:"accounts"`
	SelectedAccountID  string            `json:"selectedAccountId"`
	TotalMemoryMB      int               `json:"totalMemoryMB"`
	AutoMemoryMB       int               `json:"autoMemoryMB"`
	License            licensepkg.Status `json:"license"`
	Elevated           bool              `json:"elevated"`
	ElevationConfirmed bool              `json:"elevationConfirmed"`
}

// Bootstrap returns all initial state.
func (a *App) Bootstrap() Bootstrap {
	b := Bootstrap{
		Version: appVersion, Running: []string{},
		TotalMemoryMB: a.totalMemoryMB, AutoMemoryMB: a.autoMemoryMB,
	}
	if a.license != nil {
		b.License = a.license.Status()
	}
	a.elevationMu.Lock()
	b.Elevated = a.elevated
	b.ElevationConfirmed = a.elevationConfirmed
	a.elevationMu.Unlock()
	if a.paths.Root != "" {
		b.Paths = PathsView{
			Base: a.paths.Base, Root: a.paths.Root, Data: a.paths.Data, Instances: a.paths.Instances,
			Versions: a.paths.Versions, Libraries: a.paths.Libraries,
			Assets: a.paths.Assets, Runtimes: a.paths.Runtimes, Cache: a.paths.Cache,
		}
	}
	if err := a.baseReady(); err != nil {
		b.InitError = err.Error()
		return b
	}
	b.Settings = a.store.Settings()
	b.Accounts = a.accounts.List()
	b.SelectedAccountID = a.accounts.SelectedID()
	for _, inst := range a.store.Instances() {
		v := a.instanceView(inst)
		b.Instances = append(b.Instances, v)
		if v.Running {
			b.Running = append(b.Running, inst.ID)
		}
	}
	if b.Instances == nil {
		b.Instances = []InstanceView{}
	}
	a.uiReady.Store(true)
	return b
}

// instanceDir returns the effective game directory for an instance. When
// version isolation is disabled every instance shares the .minecraft folder.
func (a *App) instanceDir(inst store.Instance) string {
	if a.store.Settings().VersionIsolation {
		return a.store.InstanceDir(inst.ID)
	}
	return a.paths.Data
}

func (a *App) instanceView(inst store.Instance) InstanceView {
	gameDir := a.instanceDir(inst)
	required := a.requiredJava(inst)
	launchID := inst.LaunchVersionID
	if launchID == "" {
		launchID = inst.MCVersion
	}
	installed := a.mojang.IsInstalled(launchID) || a.mojang.IsInstalled(inst.MCVersion)
	a.procMu.Lock()
	_, running := a.procs[inst.ID]
	a.procMu.Unlock()
	lastPlayed := ""
	if !inst.LastPlayed.IsZero() {
		lastPlayed = inst.LastPlayed.Format(time.RFC3339)
	}
	createdAt := ""
	if !inst.CreatedAt.IsZero() {
		createdAt = inst.CreatedAt.Format(time.RFC3339)
	}
	return InstanceView{
		ID:                inst.ID,
		Name:              inst.Name,
		MCVersion:         inst.MCVersion,
		Loader:            inst.Loader,
		LoaderVersion:     inst.LoaderVersion,
		LaunchVersionID:   inst.LaunchVersionID,
		MemoryMB:          inst.MemoryMB,
		JavaPath:          inst.JavaPath,
		CreatedAt:         createdAt,
		LastPlayed:        lastPlayed,
		PlayTimeMinutes:   inst.PlayTimeMinutes,
		Source:            inst.Source,
		Notes:             inst.Notes,
		GameDir:           gameDir,
		RequiredJava:      required,
		VersionInstalled:  installed,
		Running:           running,
		ModCount:          countFiles(filepath.Join(gameDir, "mods"), ".jar"),
		ShaderCount:       countEntries(filepath.Join(gameDir, "shaderpacks")),
		ResourcePackCount: countEntries(filepath.Join(gameDir, "resourcepacks")),
		HasJavaOverride:   strings.TrimSpace(inst.JavaPath) != "",
		AutoMemory:        inst.MemoryMB == 0,
		EffectiveMemoryMB: a.effectiveMemoryMB(inst),
	}
}

func (a *App) effectiveMemoryMB(inst store.Instance) int {
	if inst.MemoryMB > 0 {
		return inst.MemoryMB
	}
	if a.autoMemoryMB > 0 {
		return a.autoMemoryMB
	}
	return sysinfo.RecommendedMemoryMB(a.totalMemoryMB)
}

func (a *App) requiredJava(inst store.Instance) int {
	versionID := inst.LaunchVersionID
	if versionID == "" {
		versionID = inst.MCVersion
	}
	if v, err := a.mojang.Merged(versionID); err == nil {
		return v.JavaMajor()
	}
	if v, err := a.mojang.Merged(inst.MCVersion); err == nil {
		return v.JavaMajor()
	}
	return mojang.FallbackJavaMajor(inst.MCVersion)
}

func countFiles(dir, suffix string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), suffix) {
			n++
		}
	}
	return n
}

func countEntries(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if name != "" && !strings.HasPrefix(name, ".") && !strings.HasSuffix(strings.ToLower(name), ".disabled") {
			n++
		}
	}
	return n
}

// GetInstances returns fresh instance views.
func (a *App) GetInstances() ([]InstanceView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	out := []InstanceView{}
	for _, inst := range a.store.Instances() {
		out = append(out, a.instanceView(inst))
	}
	return out, nil
}

func customsForMirrors(items []store.MirrorConfig) []mirror.CustomConfig {
	out := make([]mirror.CustomConfig, 0, len(items))
	for _, item := range items {
		out = append(out, mirror.CustomConfig{ID: item.ID, Name: item.Name, URL: item.URL, Kind: item.Kind})
	}
	return out
}

func (a *App) applyMirrorSources() {
	settings := a.store.Settings()
	customs := customsForMirrors(settings.CustomMirrors)
	sources := mirror.Build(customs)
	a.mojang.SetSources(sources)
	if a.loaders != nil {
		a.loaders.SetSources(sources)
	}
	if a.javaMgr != nil {
		a.javaMgr.SetSources(mirror.JavaSources(customs))
	}
}

func normalizeCustomMirrors(items []store.MirrorConfig) ([]store.MirrorConfig, error) {
	out := make([]store.MirrorConfig, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.URL) == "" && strings.TrimSpace(item.Name) == "" {
			continue
		}
		source, err := mirror.NormalizeCustom(mirror.CustomConfig{ID: item.ID, Name: item.Name, URL: item.URL, Kind: item.Kind})
		if err != nil {
			return nil, fmt.Errorf("下载镜像「%s」无效: %w", firstNonEmpty(item.Name, item.URL), err)
		}
		if seen[source.BaseURL] {
			continue
		}
		seen[source.BaseURL] = true
		kind := "bmclapi"
		if source.Kind == mirror.KindCustomProxy {
			kind = "proxy"
		}
		out = append(out, store.MirrorConfig{ID: source.ID, Name: source.Name, URL: source.BaseURL, Kind: kind})
	}
	return out, nil
}

func normalizeResourceMirrors(items []store.MirrorConfig) ([]store.MirrorConfig, error) {
	out := make([]store.MirrorConfig, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.URL) == "" && strings.TrimSpace(item.Name) == "" {
			continue
		}
		source, err := mirror.NormalizeResourceCustom(mirror.CustomConfig{
			ID: item.ID, Name: item.Name, URL: item.URL, Kind: item.Kind,
		})
		if err != nil {
			return nil, fmt.Errorf("资源镜像「%s」无效: %w", firstNonEmpty(item.Name, item.URL), err)
		}
		if seen[source.BaseURL] {
			continue
		}
		seen[source.BaseURL] = true
		out = append(out, store.MirrorConfig{ID: source.ID, Name: source.Name, URL: source.BaseURL, Kind: source.Kind})
	}
	return out, nil
}

func (a *App) resourceSource(id string) mirror.ResourceSource {
	if a.store == nil {
		return mirror.FindResourceSource("official", nil)
	}
	settings := a.store.Settings()
	customs := customsForMirrors(settings.CustomResourceMirrors)
	if strings.TrimSpace(id) == "" {
		id = settings.ResourceMirror
	}
	return mirror.FindResourceSource(id, customs)
}

// ListResourceMirrors returns the built-in and configured Modrinth/CurseForge
// download mirrors. It contains no secrets.
func (a *App) ListResourceMirrors() []mirror.ResourceSource {
	if a.store == nil {
		return mirror.ResourceSources(nil)
	}
	return mirror.ResourceSources(customsForMirrors(a.store.Settings().CustomResourceMirrors))
}

// TestDownloadMirrors measures configured sources using their version manifest.
func (a *App) TestDownloadMirrors(customs []store.MirrorConfig) ([]mirror.SpeedResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	normalized, err := normalizeCustomMirrors(customs)
	if err != nil {
		return nil, err
	}
	configs := make([]mirror.CustomConfig, 0, len(normalized))
	for _, item := range normalized {
		configs = append(configs, mirror.CustomConfig{ID: item.ID, Name: item.Name, URL: item.URL, Kind: item.Kind})
	}
	sources := mirror.Build(configs)
	return mirror.Test(a.context(), sources, func(source mirror.Source) (string, bool) {
		return source.URL(mirror.ResourceManifest, mirror.OfficialManifest, "")
	}, 10*time.Second), nil
}

// SaveSettings persists launcher preferences.
func (a *App) SaveSettings(next store.Settings) (store.Settings, error) {
	if err := a.ready(); err != nil {
		return store.Settings{}, err
	}
	next.PlayerName = strings.TrimSpace(next.PlayerName)
	normalizedMirrors, err := normalizeCustomMirrors(next.CustomMirrors)
	if err != nil {
		return store.Settings{}, err
	}
	next.CustomMirrors = normalizedMirrors
	normalizedResourceMirrors, err := normalizeResourceMirrors(next.CustomResourceMirrors)
	if err != nil {
		return store.Settings{}, err
	}
	next.CustomResourceMirrors = normalizedResourceMirrors
	next.ResourceMirror = strings.TrimSpace(next.ResourceMirror)
	if next.ResourceMirror == "" {
		next.ResourceMirror = "official"
	}
	validResourceMirror := false
	for _, source := range mirror.ResourceSources(customsForMirrors(next.CustomResourceMirrors)) {
		if source.ID == next.ResourceMirror {
			validResourceMirror = true
			break
		}
	}
	if !validResourceMirror {
		next.ResourceMirror = "official"
	}
	saved, err := a.store.SaveSettings(next)
	if err != nil {
		return store.Settings{}, err
	}
	a.applyMirrorSources()
	a.syncLauncherProfiles()
	return saved, nil
}

// AccountState is the serializable account list.
type AccountState struct {
	Accounts   []auth.View `json:"accounts"`
	SelectedID string      `json:"selectedId"`
}

func (a *App) accountState() AccountState {
	return AccountState{Accounts: a.accounts.List(), SelectedID: a.accounts.SelectedID()}
}

func (a *App) emitAccountsChanged() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "accounts:changed", a.accountState())
}

// GetAccounts returns all configured accounts.
func (a *App) GetAccounts() (AccountState, error) {
	if err := a.ready(); err != nil {
		return AccountState{}, err
	}
	return a.accountState(), nil
}

// SelectAccount switches the active launch identity.
func (a *App) SelectAccount(id string) (AccountState, error) {
	if err := a.ready(); err != nil {
		return AccountState{}, err
	}
	if err := a.accounts.Select(id); err != nil {
		return AccountState{}, err
	}
	a.syncOfflineSetting()
	a.emitAccountsChanged()
	return a.accountState(), nil
}

// AddOfflineAccount creates a new offline identity or selects an existing
// offline account with the same name.
func (a *App) AddOfflineAccount(name string) (AccountState, error) {
	if err := a.ready(); err != nil {
		return AccountState{}, err
	}
	account, err := a.accounts.AddOfflineAccount(name)
	if err != nil {
		return AccountState{}, err
	}
	settings := a.store.Settings()
	settings.PlayerName = account.Name
	_, _ = a.store.SaveSettings(settings)
	a.emitAccountsChanged()
	return a.accountState(), nil
}

// RenameOfflineAccount changes an offline identity and keeps it selected.
func (a *App) RenameOfflineAccount(id, name string) (AccountState, error) {
	if err := a.ready(); err != nil {
		return AccountState{}, err
	}
	account, err := a.accounts.RenameOfflineAccount(id, name)
	if err != nil {
		return AccountState{}, err
	}
	settings := a.store.Settings()
	settings.PlayerName = account.Name
	_, _ = a.store.SaveSettings(settings)
	a.emitAccountsChanged()
	return a.accountState(), nil
}

// RemoveAccount deletes a premium or third-party account.
func (a *App) RemoveAccount(id string) (AccountState, error) {
	if err := a.ready(); err != nil {
		return AccountState{}, err
	}
	if err := a.accounts.Remove(id); err != nil {
		return AccountState{}, err
	}
	a.syncOfflineSetting()
	a.emitAccountsChanged()
	return a.accountState(), nil
}

// GetHitokoto returns one random sentence for the home page.
func (a *App) GetHitokoto() (hitokoto.View, error) {
	if err := a.ready(); err != nil {
		return hitokoto.View{}, err
	}
	return hitokoto.Fetch(a.context(), a.store.Settings().HitokotoAPIURL)
}

// AuthlibServerView is a built-in third-party authentication provider.
type AuthlibServerView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// AuthlibServers returns the built-in third-party login providers.
func (a *App) AuthlibServers() []AuthlibServerView {
	return []AuthlibServerView{
		{ID: "littleskin", Name: "LittleSkin", URL: "https://littleskin.cn/api/yggdrasil"},
		{ID: "yss", Name: "YggdrasilSkinServer", URL: "https://yss.1v.fit"},
	}
}

// LoginAuthlib signs in to an Authlib-Injector compatible server.
func (a *App) LoginAuthlib(serverURL, username, password string) (auth.View, error) {
	if err := a.ready(); err != nil {
		return auth.View{}, err
	}
	account, err := auth.LoginAuthlib(a.context(), serverURL, username, password)
	if err != nil {
		return auth.View{}, err
	}
	saved, err := a.accounts.Add(account)
	if err != nil {
		return auth.View{}, err
	}
	a.emitAccountsChanged()
	return saved.View(), nil
}

// YSSDeviceView is returned when a YSS one-click authorization starts.
type YSSDeviceView struct {
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
	Message                 string `json:"message"`
}

// BeginYSSLogin starts YSS device-code authorization and opens no UI itself.
// The browser is opened by the frontend with the returned verification URL.
func (a *App) BeginYSSLogin(serverURL string) (YSSDeviceView, error) {
	if err := a.ready(); err != nil {
		return YSSDeviceView{}, err
	}
	serverURL = strings.TrimSpace(serverURL)
	if serverURL == "" {
		serverURL = auth.DefaultYSSBaseURL
	}
	a.authMu.Lock()
	if a.msCancel != nil || a.yssCancel != nil {
		a.authMu.Unlock()
		return YSSDeviceView{}, errors.New("已有登录正在进行")
	}
	a.authMu.Unlock()

	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	meta, device, err := auth.BeginYSSDevice(ctx, serverURL, auth.DefaultYSSClientID)
	cancel()
	if err != nil {
		return YSSDeviceView{}, err
	}

	pollCtx, pollCancel := context.WithCancel(a.context())
	a.authMu.Lock()
	a.yssCancel = pollCancel
	a.authMu.Unlock()

	go func() {
		defer func() {
			pollCancel()
			a.authMu.Lock()
			a.yssCancel = nil
			a.authMu.Unlock()
		}()
		account, err := auth.PollYSSDevice(pollCtx, meta, device, auth.DefaultYSSClientID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, auth.ErrLoginCancelled) {
				return
			}
			a.emitYSSLogin(AccountLoginEvent{Success: false, Message: err.Error()})
			return
		}
		saved, err := a.accounts.Add(account)
		if err != nil {
			a.emitYSSLogin(AccountLoginEvent{Success: false, Message: err.Error()})
			return
		}
		view := saved.View()
		a.emitAccountsChanged()
		a.emitYSSLogin(AccountLoginEvent{Success: true, Message: "YSS 授权成功", Account: &view})
	}()

	return YSSDeviceView{
		UserCode:                device.UserCode,
		VerificationURI:         device.VerificationURI,
		VerificationURIComplete: device.VerificationURIComplete,
		ExpiresIn:               device.ExpiresIn,
		Interval:                device.Interval,
		Message:                 "在浏览器中登录 YSS 并确认授权。",
	}, nil
}

// CancelYSSLogin aborts a pending YSS device-code poll.
func (a *App) CancelYSSLogin() {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.yssCancel != nil {
		a.yssCancel()
		a.yssCancel = nil
	}
}

func (a *App) emitYSSLogin(event AccountLoginEvent) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "auth:yss", event)
	}
}

// DeviceCodeView is safe to expose to the frontend.
type DeviceCodeView struct {
	UserCode        string `json:"userCode"`
	VerificationURI string `json:"verificationUri"`
	ExpiresIn       int    `json:"expiresIn"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

// AccountLoginEvent is emitted after a background Microsoft login finishes.
type AccountLoginEvent struct {
	Success bool       `json:"success"`
	Message string     `json:"message"`
	Account *auth.View `json:"account,omitempty"`
}

// BeginMicrosoftLogin starts device-code authentication and returns at once.
func (a *App) BeginMicrosoftLogin() (DeviceCodeView, error) {
	if err := a.ready(); err != nil {
		return DeviceCodeView{}, err
	}
	a.authMu.Lock()
	if a.msCancel != nil || a.yssCancel != nil {
		a.authMu.Unlock()
		return DeviceCodeView{}, errors.New("已有登录正在进行")
	}
	a.authMu.Unlock()

	clientID := a.microsoftClientID()
	ctx, cancel := context.WithTimeout(a.context(), 30*time.Second)
	info, err := auth.StartDeviceCode(ctx, clientID)
	cancel()
	if err != nil {
		return DeviceCodeView{}, err
	}
	pollCtx, pollCancel := context.WithCancel(a.context())
	a.authMu.Lock()
	a.msCancel = pollCancel
	a.authMu.Unlock()

	go func() {
		defer func() {
			pollCancel()
			a.authMu.Lock()
			a.msCancel = nil
			a.authMu.Unlock()
		}()
		token, err := auth.PollDeviceCode(pollCtx, clientID, info.DeviceCode, info.Interval, info.ExpiresIn)
		if err != nil {
			if errors.Is(err, auth.ErrLoginCancelled) {
				return
			}
			a.emitAccountLogin(AccountLoginEvent{Success: false, Message: err.Error()})
			return
		}
		account, err := auth.AuthenticateMinecraft(pollCtx, clientID, token.AccessToken, token.RefreshToken)
		if err != nil {
			a.emitAccountLogin(AccountLoginEvent{Success: false, Message: err.Error()})
			return
		}
		saved, err := a.accounts.Add(account)
		if err != nil {
			a.emitAccountLogin(AccountLoginEvent{Success: false, Message: err.Error()})
			return
		}
		view := saved.View()
		a.emitAccountsChanged()
		a.emitAccountLogin(AccountLoginEvent{Success: true, Message: "Microsoft 登录成功", Account: &view})
	}()

	return DeviceCodeView{
		UserCode: info.UserCode, VerificationURI: info.VerificationURI,
		ExpiresIn: info.ExpiresIn, Interval: info.Interval, Message: info.Message,
	}, nil
}

// CancelMicrosoftLogin aborts a pending device-code poll.
func (a *App) CancelMicrosoftLogin() {
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.msCancel != nil {
		a.msCancel()
		a.msCancel = nil
	}
}

func (a *App) emitAccountLogin(event AccountLoginEvent) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "auth:login", event)
	}
}

func (a *App) microsoftClientID() string {
	if value := strings.TrimSpace(a.store.Settings().MicrosoftClientID); value != "" {
		return value
	}
	return auth.DefaultMicrosoftClientID
}

type launchAuth struct {
	Name          string
	UUID          string
	AccessToken   string
	UserType      string
	XUID          string
	AuthlibJar    string
	AuthlibServer string
}

// resolveLaunchAuth prepares the selected account right before launch. Token
// refreshes for Microsoft and external accounts happen transparently.
func (a *App) resolveLaunchAuth(ctx context.Context) (launchAuth, error) {
	account, ok := a.accounts.Selected()
	if !ok {
		name := strings.TrimSpace(a.store.Settings().PlayerName)
		if name == "" {
			name = "Steve"
		}
		return launchAuth{Name: name, UUID: auth.OfflineUUID(name), AccessToken: "0", UserType: "legacy"}, nil
	}
	switch account.Kind {
	case auth.KindOffline:
		return launchAuth{
			Name: account.Name, UUID: firstNonEmpty(account.UUID, auth.OfflineUUID(account.Name)),
			AccessToken: "0", UserType: "legacy",
		}, nil
	case auth.KindMicrosoft:
		if account.AccessToken == "" || time.Until(account.ExpiresAt) < 2*time.Minute {
			t := a.newTask("刷新 Microsoft 登录")
			var refreshErr error
			token, err := auth.RefreshMSA(ctx, a.microsoftClientID(), account.RefreshToken)
			if err == nil {
				renewed, authErr := auth.AuthenticateMinecraft(ctx, a.microsoftClientID(), token.AccessToken, token.RefreshToken)
				if authErr != nil {
					refreshErr = authErr
				} else {
					renewed.ID = account.ID
					renewed.AddedAt = account.AddedAt
					if updateErr := a.accounts.Update(renewed); updateErr != nil {
						refreshErr = updateErr
					} else {
						account = renewed
						a.emitAccountsChanged()
					}
				}
			} else {
				refreshErr = err
			}
			t.finish(refreshErr)
			if refreshErr != nil {
				return launchAuth{}, fmt.Errorf("Microsoft 登录已失效，请重新登录: %w", refreshErr)
			}
		}
		return launchAuth{
			Name: account.Name, UUID: strings.ReplaceAll(account.UUID, "-", ""),
			AccessToken: account.AccessToken, UserType: "msa", XUID: account.XUID,
		}, nil
	case auth.KindAuthlib:
		t := a.newTask("准备外置登录")
		var authErr error
		valid, err := auth.ValidateAuthlib(ctx, account)
		if err != nil {
			authErr = err
		} else if !valid {
			var renewed auth.Account
			var refreshErr error
			if account.OAuth2 {
				renewed, refreshErr = auth.RefreshYSSAccount(ctx, account)
			} else {
				renewed, refreshErr = auth.RefreshAuthlib(ctx, account)
			}
			if refreshErr != nil {
				authErr = refreshErr
			} else {
				renewed.ID = account.ID
				renewed.AddedAt = account.AddedAt
				if updateErr := a.accounts.Update(renewed); updateErr != nil {
					authErr = updateErr
				} else {
					account = renewed
					a.emitAccountsChanged()
				}
			}
		}
		jar := auth.FindInjector(a.paths)
		if authErr == nil && jar == "" {
			jar, authErr = auth.EnsureInjector(ctx, a.paths)
		}
		t.finish(authErr)
		if authErr != nil {
			return launchAuth{}, fmt.Errorf("外置登录不可用，请重新登录: %w", authErr)
		}
		return launchAuth{
			Name: account.Name, UUID: strings.ReplaceAll(account.UUID, "-", ""),
			AccessToken: account.AccessToken, UserType: "mojang",
			AuthlibJar: jar, AuthlibServer: account.ServerURL,
		}, nil
	default:
		return launchAuth{}, errors.New("未知的账户类型")
	}
}

func (a *App) syncOfflineSetting() {
	account, ok := a.accounts.Selected()
	if !ok || account.Kind != auth.KindOffline {
		return
	}
	settings := a.store.Settings()
	if settings.PlayerName != account.Name {
		settings.PlayerName = account.Name
		_, _ = a.store.SaveSettings(settings)
	}
}

// VersionView is one downloadable game version.
type VersionView struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	ReleaseTime string `json:"releaseTime"`
	JavaMajor   int    `json:"javaMajor"`
	Installed   bool   `json:"installed"`
}

// VersionList is the version catalog plus latest aliases.
type VersionList struct {
	Versions       []VersionView `json:"versions"`
	LatestRelease  string        `json:"latestRelease"`
	LatestSnapshot string        `json:"latestSnapshot"`
}

// ListVersions returns Minecraft versions from Mojang's manifest.
func (a *App) ListVersions(includeSnapshots, includeOld bool) (VersionList, error) {
	if err := a.ready(); err != nil {
		return VersionList{}, err
	}
	m, err := a.mojang.Manifest(a.context())
	if err != nil {
		return VersionList{}, err
	}
	out := VersionList{LatestRelease: m.Latest.Release, LatestSnapshot: m.Latest.Snapshot, Versions: []VersionView{}}
	for _, entry := range m.Versions {
		switch entry.Type {
		case "release":
		case "snapshot":
			if !includeSnapshots {
				continue
			}
		case "old_beta", "old_alpha":
			if !includeOld {
				continue
			}
		default:
			continue
		}
		out.Versions = append(out.Versions, VersionView{
			ID: entry.ID, Type: entry.Type, ReleaseTime: entry.ReleaseTime.Format(time.RFC3339),
			JavaMajor: mojang.FallbackJavaMajor(entry.ID),
			Installed: a.mojang.IsInstalled(entry.ID),
		})
	}
	return out, nil
}

// InstallVersion downloads a full vanilla game version.
func (a *App) InstallVersion(id string) (err error) {
	if err := a.ready(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("版本 ID 不能为空")
	}
	t := a.newTask("安装 Minecraft " + id)
	defer func() { t.finish(err) }()
	if _, err := a.mojang.ManifestEntry(t.context(), id); err != nil {
		return err
	}
	a.installMu.Lock()
	defer a.installMu.Unlock()
	return a.mojang.Install(t.context(), id, a.store.Settings().DownloadWorkers, t.mojangProgress(""))
}

// RepairVersion verifies the game files of an installed instance and fills in
// missing or corrupted client, library, asset and loader files.
func (a *App) RepairVersion(instanceID string) (err error) {
	if err := a.ready(); err != nil {
		return err
	}
	inst, ok := a.store.Instance(strings.TrimSpace(instanceID))
	if !ok {
		return errors.New("版本不存在")
	}
	a.procMu.Lock()
	_, running := a.procs[inst.ID]
	a.procMu.Unlock()
	if running {
		return errors.New("请先关闭游戏再修复版本")
	}
	workers := a.store.Settings().DownloadWorkers
	t := a.newTask("修复版本 · " + inst.Name)
	defer func() { t.finish(err) }()

	a.installMu.Lock()
	defer a.installMu.Unlock()
	t.update("game", "正在校验游戏本体、运行库与资源…", 0, 0)
	if err := a.mojang.Repair(t.context(), inst.MCVersion, workers, t.mojangProgress("game")); err != nil {
		return fmt.Errorf("修复游戏版本失败: %w", err)
	}
	loaderType := loader.Normalize(inst.Loader)
	profileID := strings.TrimSpace(inst.LaunchVersionID)
	if loaderType != "" && loaderType != "vanilla" {
		if profileID != "" && profileID != inst.MCVersion && a.mojang.IsProfileInstalled(profileID) {
			t.update("loader", "正在校验加载器运行库…", 0, 0)
			if err := a.mojang.RepairProfile(t.context(), profileID, workers, t.mojangProgress("loader")); err != nil {
				return fmt.Errorf("修复加载器文件失败: %w", err)
			}
		} else {
			javaPath := inst.JavaPath
			if loaderType == "forge" || loaderType == "neoforge" {
				rt, javaErr := a.ensureJavaForLoader(t.context(), &inst, t)
				if javaErr != nil {
					return javaErr
				}
				javaPath = rt.Path
			}
			repairedID, installErr := a.loaders.Ensure(t.context(), inst.MCVersion, loaderType, inst.LoaderVersion, javaPath, workers, t.mojangProgress("loader"))
			if installErr != nil {
				return fmt.Errorf("补充加载器文件失败: %w", installErr)
			}
			profileID = repairedID
			updated, updateErr := a.store.UpdateInstance(inst.ID, func(item *store.Instance) error {
				item.LaunchVersionID = repairedID
				return nil
			})
			if updateErr != nil {
				return updateErr
			}
			inst = updated
			// Ensure may return an already-marked profile without checking every
			// library. Repair explicitly verifies the loader's own libraries so a
			// stale profile cannot survive a "repair version" click.
			t.update("loader", "正在校验加载器运行库…", 0, 0)
			if err := a.mojang.RepairProfile(t.context(), repairedID, workers, t.mojangProgress("loader")); err != nil {
				return fmt.Errorf("修复加载器文件失败: %w", err)
			}
		}
	}
	t.update("done", "版本文件校验完成", 1, 1)
	a.emitInstancesChanged()
	return nil
}

// CreateInstanceRequest creates a new isolated instance.
type CreateInstanceRequest struct {
	Name          string `json:"name"`
	MCVersion     string `json:"mcVersion"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loaderVersion"`
	MemoryMB      int    `json:"memoryMB"`
}

// CreateInstance creates an instance and installs its runtime.
func (a *App) CreateInstance(req CreateInstanceRequest) (view InstanceView, err error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	req.MCVersion = strings.TrimSpace(req.MCVersion)
	if req.MCVersion == "" {
		return InstanceView{}, errors.New("请选择 Minecraft 版本")
	}
	if _, err := a.mojang.ManifestEntry(a.context(), req.MCVersion); err != nil {
		return InstanceView{}, fmt.Errorf("无效的 Minecraft 版本: %w", err)
	}
	if !a.mojang.IsInstalled(req.MCVersion) {
	}
	loaderType := loader.Normalize(req.Loader)
	if loaderType == "" {
		loaderType = "vanilla"
	}
	loaderVersion := strings.TrimSpace(req.LoaderVersion)
	t := a.newTask("创建版本 " + firstNonEmpty(req.Name, req.MCVersion))
	defer func() { t.finish(err) }()

	if loaderType != "vanilla" && loaderVersion == "" {
		t.update("mirror", "正在为模组加载器测速并选择下载源…", 0, 0)
		t.update("metadata", "正在选择 "+loaderType+" 版本…", 0, 0)
		loaderVersion, err = a.loaders.LatestVersion(t.context(), loaderType, req.MCVersion)
		if err != nil {
			return InstanceView{}, err
		}
	}
	inst, err := a.store.CreateInstance(store.Instance{
		Name:            req.Name,
		MCVersion:       req.MCVersion,
		Loader:          loaderType,
		LoaderVersion:   loaderVersion,
		LaunchVersionID: req.MCVersion,
		MemoryMB:        req.MemoryMB,
		Source:          "manual",
	})
	if err != nil {
		return InstanceView{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = a.store.DeleteInstance(inst.ID, true)
		}
	}()
	if err = a.installInstanceRuntime(t.context(), &inst, t); err != nil {
		return InstanceView{}, err
	}
	success = true
	a.emitInstancesChanged()
	return a.instanceView(inst), nil
}

// installInstanceRuntime ensures the vanilla version and loader are installed.
func (a *App) installInstanceRuntime(ctx context.Context, inst *store.Instance, t *progressTask) error {
	a.installMu.Lock()
	defer a.installMu.Unlock()
	workers := a.store.Settings().DownloadWorkers
	t.update("game", "正在准备游戏文件…", 0, 0)
	if err := a.mojang.Install(ctx, inst.MCVersion, workers, t.mojangProgress("game")); err != nil {
		return fmt.Errorf("安装游戏版本失败: %w", err)
	}
	loaderType := loader.Normalize(inst.Loader)
	if loaderType == "vanilla" || loaderType == "" {
		updated, err := a.store.UpdateInstance(inst.ID, func(i *store.Instance) error {
			i.LaunchVersionID = inst.MCVersion
			return nil
		})
		if err != nil {
			return err
		}
		*inst = updated
		return nil
	}
	loaderVersion := inst.LoaderVersion
	if loaderVersion == "" {
		t.update("metadata", "正在选择加载器版本…", 0, 0)
		var err error
		loaderVersion, err = a.loaders.LatestVersion(ctx, loaderType, inst.MCVersion)
		if err != nil {
			return err
		}
	}
	javaPath := inst.JavaPath
	if loaderType == "forge" || loaderType == "neoforge" {
		rt, err := a.ensureJavaForLoader(ctx, inst, t)
		if err != nil {
			return err
		}
		javaPath = rt.Path
	}
	t.update("mirror", "正在为模组加载器测速并选择镜像…", 0, 0)
	profileID, err := a.loaders.Ensure(ctx, inst.MCVersion, loaderType, loaderVersion, javaPath, workers, t.mojangProgress("loader"))
	if err != nil {
		return err
	}
	updated, err := a.store.UpdateInstance(inst.ID, func(i *store.Instance) error {
		i.LoaderVersion = loaderVersion
		i.LaunchVersionID = profileID
		return nil
	})
	if err != nil {
		return err
	}
	*inst = updated
	return nil
}

func (a *App) ensureJavaForLoader(ctx context.Context, inst *store.Instance, t *progressTask) (java.Runtime, error) {
	required := a.requiredJava(*inst)
	extra := []string{a.store.Settings().DefaultJavaPath}
	if inst.JavaPath != "" {
		extra = append(extra, inst.JavaPath)
	}
	t.update("java", fmt.Sprintf("正在查找 Java %d…", required), 0, 0)
	rt, err := a.javaMgr.FindSuitable(ctx, required, inst.JavaPath, extra)
	if err == nil {
		return rt, nil
	}
	t.update("java", fmt.Sprintf("未找到 Java %d，正在自动下载 Temurin…", required), 0, 0)
	rt, err = a.javaMgr.Install(ctx, required, t.javaBytesProgress("java"))
	if err != nil {
		return java.Runtime{}, fmt.Errorf("需要 Java %d，但自动安装失败: %w", required, err)
	}
	settings := a.store.Settings()
	if strings.TrimSpace(settings.DefaultJavaPath) == "" {
		settings.DefaultJavaPath = rt.Path
		if _, saveErr := a.store.SaveSettings(settings); saveErr != nil {
			return java.Runtime{}, saveErr
		}
	}
	return rt, nil
}

// UpdateInstanceRequest changes user-editable instance settings.
type UpdateInstanceRequest struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MemoryMB int    `json:"memoryMB"`
	JavaPath string `json:"javaPath"`
}

// UpdateInstance edits name, memory or the per-instance Java override.
func (a *App) UpdateInstance(req UpdateInstanceRequest) (InstanceView, error) {
	if err := a.ready(); err != nil {
		return InstanceView{}, err
	}
	javaPath := strings.TrimSpace(req.JavaPath)
	if javaPath != "" {
		rt, err := java.Probe(a.context(), javaPath)
		if err != nil {
			return InstanceView{}, fmt.Errorf("Java 路径不可用: %w", err)
		}
		javaPath = rt.Path
	}
	updated, err := a.store.UpdateInstance(req.ID, func(i *store.Instance) error {
		i.Name = req.Name
		i.MemoryMB = req.MemoryMB
		i.JavaPath = javaPath
		return nil
	})
	if err != nil {
		return InstanceView{}, err
	}
	a.emitInstancesChanged()
	return a.instanceView(updated), nil
}

// DeleteInstance removes an instance, stopping it first if necessary.
func (a *App) DeleteInstance(id string, deleteFiles bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	a.procMu.Lock()
	proc := a.procs[id]
	a.procMu.Unlock()
	if proc != nil {
		_ = proc.Stop()
		select {
		case <-proc.Done():
		case <-time.After(6 * time.Second):
		}
	}
	if err := a.store.DeleteInstance(id, deleteFiles && a.store.Settings().VersionIsolation); err != nil {
		return err
	}
	a.procMu.Lock()
	delete(a.procs, id)
	delete(a.logs, id)
	a.procMu.Unlock()
	a.emitInstancesChanged()
	return nil
}

// LaunchState is emitted when a game starts or exits.
type LaunchState struct {
	InstanceID      string `json:"instanceId"`
	Running         bool   `json:"running"`
	ExitCode        int    `json:"exitCode"`
	DurationSeconds int64  `json:"durationSeconds"`
	StoppedByUser   bool   `json:"stoppedByUser"`
}

// LaunchInstance starts Minecraft for an instance. language is the launcher's
// current UI language and is written to the game's options.txt.
func (a *App) LaunchInstance(id, language string) error {
	return a.launchInstance(id, language, nil)
}

// LaunchInstanceToServer starts an instance and joins a Minecraft server
// directly. 1.20+ uses quick play; older versions use --server/--port.
func (a *App) LaunchInstanceToServer(id, language, address string) error {
	server, err := launcher.ParseServerAddress(address)
	if err != nil {
		return fmt.Errorf("服务器地址无效: %w", err)
	}
	return a.launchInstance(id, language, server)
}

// resolveLaunchVersion prefers the installed loader profile, but falls back to
// a self-contained merged version (as created by PCL/HMCL imports) when the
// separate loader profile was removed or never existed.
func (a *App) resolveLaunchVersion(inst store.Instance) store.Instance {
	launchID := strings.TrimSpace(inst.LaunchVersionID)
	if launchID != "" && a.mojang.IsInstalled(launchID) {
		return inst
	}
	versionID := strings.TrimSpace(inst.VersionID)
	if versionID == "" {
		versionID = strings.TrimSpace(inst.ID)
	}
	if versionID == "" || versionID == launchID || !a.mojang.IsInstalled(versionID) {
		return inst
	}
	version, err := a.mojang.LocalVersion(versionID)
	if err != nil || !hasLoaderMainClass(version) {
		return inst
	}
	updated, err := a.store.UpdateInstance(inst.ID, func(item *store.Instance) error {
		item.LaunchVersionID = versionID
		return nil
	})
	if err == nil {
		return updated
	}
	inst.LaunchVersionID = versionID
	return inst
}

func hasLoaderMainClass(version mojang.Version) bool {
	mainClass := strings.ToLower(strings.TrimSpace(version.MainClass))
	return strings.Contains(mainClass, "bootstraplauncher") ||
		strings.Contains(mainClass, "modlauncher") ||
		strings.Contains(mainClass, "launchwrapper") ||
		strings.Contains(mainClass, "fabric") ||
		strings.Contains(mainClass, "quilt")
}

func (a *App) ensureLoaderReady(inst store.Instance) error {
	loaderType := loader.Normalize(inst.Loader)
	if loaderType == "" || loaderType == "vanilla" {
		return nil
	}
	launchID := strings.TrimSpace(inst.LaunchVersionID)
	if launchID == "" || launchID == strings.TrimSpace(inst.MCVersion) || !a.mojang.IsInstalled(launchID) {
		return errors.New("模组加载器尚未安装完成，请先点击「修复版本」")
	}
	version, err := a.mojang.LocalVersion(launchID)
	if err != nil || strings.TrimSpace(version.MainClass) == "" {
		return errors.New("模组加载器档案不完整，请先点击「修复版本」")
	}
	return nil
}

func (a *App) launchInstance(id, language string, server *launcher.ServerAddress) (err error) {
	if err := a.ready(); err != nil {
		return err
	}
	inst, ok := a.store.Instance(id)
	if !ok {
		return errors.New("版本不存在")
	}
	a.procMu.Lock()
	if _, exists := a.procs[id]; exists {
		a.procMu.Unlock()
		return errors.New("该版本正在运行")
	}
	a.procMu.Unlock()

	inst = a.resolveLaunchVersion(inst)
	if err := a.ensureLoaderReady(inst); err != nil {
		return err
	}
	launchAccount, err := a.resolveLaunchAuth(a.context())
	if err != nil {
		return err
	}
	launchInst := inst
	launchInst.MemoryMB = a.effectiveMemoryMB(inst)
	required := a.requiredJava(inst)
	preferred := strings.TrimSpace(inst.JavaPath)
	rt, findErr := a.javaMgr.FindSuitable(a.context(), required, preferred, []string{a.store.Settings().DefaultJavaPath, inst.JavaPath})
	if findErr != nil {
		if preferred != "" {
			return fmt.Errorf("需要 Java %d：%w", required, findErr)
		}
		// No compatible Java is available: install the matching Temurin
		// runtime on demand so the first launch can proceed directly.
		t := a.newTask(fmt.Sprintf("准备 Java %d", required))
		var installErr error
		rt, installErr = a.javaMgr.Install(t.context(), required, t.javaBytesProgress("java"))
		t.finish(installErr)
		if installErr != nil {
			return fmt.Errorf("需要 Java %d，自动安装失败: %w", required, installErr)
		}
		settings := a.store.Settings()
		if strings.TrimSpace(settings.DefaultJavaPath) == "" {
			settings.DefaultJavaPath = rt.Path
			if _, saveErr := a.store.SaveSettings(settings); saveErr != nil {
				return fmt.Errorf("保存 Java 设置失败: %w", saveErr)
			}
		}
	}
	ring := &logRing{}
	ring.add("[启动器] 正在启动 " + inst.Name + " · Java " + fmt.Sprint(rt.Major))
	if server != nil {
		ring.add("[启动器] 直连服务器 " + server.String())
	}
	uiLanguage := strings.TrimSpace(language)
	if uiLanguage == "" {
		uiLanguage = a.store.Settings().Language
	}
	if code, langErr := gameoptions.SetLanguage(a.instanceDir(inst), uiLanguage); langErr != nil {
		ring.add("[启动器] 无法更新游戏语言: " + langErr.Error())
	} else if code != "" {
		ring.add("[启动器] 游戏语言: " + code)
	}
	proc, err := a.launcher.Start(a.context(), launcher.Options{
		Instance:      launchInst,
		GameDir:       a.instanceDir(inst),
		Settings:      a.store.Settings(),
		JavaPath:      rt.Path,
		PlayerName:    launchAccount.Name,
		PlayerUUID:    launchAccount.UUID,
		AccessToken:   launchAccount.AccessToken,
		UserType:      launchAccount.UserType,
		XUID:          launchAccount.XUID,
		AuthlibJar:    launchAccount.AuthlibJar,
		AuthlibServer: launchAccount.AuthlibServer,
		Server:        server,
		OnLine: func(line string) {
			ring.add(line)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "launch:log", map[string]string{
					"instanceId": id,
					"line":       line,
				})
			}
		},
	})
	if err != nil {
		return err
	}
	if err := a.gameJob.Assign(proc.PID()); err != nil {
		ring.add("[启动器] 无法启用退出自动清理: " + err.Error())
	}
	a.procMu.Lock()
	// Re-check under the lock in case two clicks raced during Java detection.
	if _, exists := a.procs[id]; exists {
		a.procMu.Unlock()
		_ = proc.Stop()
		return errors.New("该版本正在运行")
	}
	a.procs[id] = proc
	a.logs[id] = ring
	a.procMu.Unlock()

	_, _ = a.store.UpdateInstance(id, func(i *store.Instance) error {
		i.LastPlayed = time.Now()
		return nil
	})
	a.emitLaunchState(LaunchState{InstanceID: id, Running: true})
	a.emitInstancesChanged()
	hideWhenReady := a.ctx != nil && !a.store.Settings().ShowLauncherOnLaunch
	watchWindow := a.ctx != nil

	go func() {
		windowCtx, stopWatch := context.WithCancel(a.context())
		watchDone := make(chan struct{})
		if watchWindow {
			go func() {
				defer close(watchDone)
				if !sysutil.WaitForGameWindow(windowCtx, proc.PID()) {
					return
				}
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "launch:window", map[string]string{"instanceId": id})
				}
				if !hideWhenReady {
					return
				}
				a.procMu.Lock()
				if _, running := a.procs[id]; running {
					a.autoHidden = true
					if a.ctx != nil {
						runtime.WindowHide(a.ctx)
					}
				}
				a.procMu.Unlock()
			}()
		} else {
			close(watchDone)
		}

		<-proc.Done()
		stopWatch()
		duration := proc.Duration()
		minutes := int(duration.Minutes())
		a.store.MarkPlayed(id, minutes)
		a.procMu.Lock()
		delete(a.procs, id)
		shouldShow := a.autoHidden && len(a.procs) == 0
		if shouldShow {
			a.autoHidden = false
		}
		a.procMu.Unlock()
		// Let an in-flight hide finish before restoring the launcher.
		<-watchDone
		if shouldShow && a.ctx != nil {
			runtime.WindowShow(a.ctx)
		}
		a.emitLaunchState(LaunchState{
			InstanceID: id, Running: false, ExitCode: proc.ExitCode(),
			DurationSeconds: int64(duration.Seconds()), StoppedByUser: proc.StoppedByUser(),
		})
		a.emitInstancesChanged()
	}()
	return nil
}

func (a *App) emitLaunchState(state LaunchState) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "launch:state", state)
	}
}

// StopInstance terminates a running game.
func (a *App) StopInstance(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	a.procMu.Lock()
	proc := a.procs[id]
	a.procMu.Unlock()
	if proc == nil {
		return errors.New("版本没有在运行")
	}
	return proc.Stop()
}

// GetInstanceLog returns the buffered launcher/game output.
func (a *App) GetInstanceLog(id string) []string {
	a.procMu.Lock()
	ring := a.logs[id]
	a.procMu.Unlock()
	if ring == nil {
		return []string{}
	}
	return ring.snapshot()
}

// OpenInstanceFolder opens an instance's game directory.
func (a *App) OpenInstanceFolder(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	inst, ok := a.store.Instance(id)
	if !ok {
		return errors.New("版本不存在")
	}
	return openPath(a.instanceDir(inst))
}

// OpenInstanceSubfolder opens one of the known content directories for an
// instance. The whitelist keeps this API from becoming an arbitrary file browser.
func (a *App) OpenInstanceSubfolder(id, folder string) error {
	if err := a.ready(); err != nil {
		return err
	}
	inst, ok := a.store.Instance(id)
	if !ok {
		return errors.New("版本不存在")
	}
	folder = strings.ToLower(strings.TrimSpace(folder))
	allowed := map[string]bool{
		"mods": true, "plugins": true, "config": true, "defaultconfigs": true,
		"resourcepacks": true, "shaderpacks": true, "datapacks": true, "saves": true,
	}
	if !allowed[folder] || filepath.Base(folder) != folder {
		return errors.New("资源目录无效")
	}
	dir := filepath.Join(a.instanceDir(inst), folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return openPath(dir)
}

// OpenDataFolder opens the portable game directory.
func (a *App) OpenDataFolder() error {
	if a.paths.Data == "" {
		return errors.New("游戏目录不可用")
	}
	return openPath(a.paths.Data)
}

// OpenConfigFolder opens the portable MGL configuration directory.
func (a *App) OpenConfigFolder() error {
	if a.paths.Root == "" {
		return errors.New("配置目录不可用")
	}
	return openPath(a.paths.Root)
}

// OpenURL opens a trusted external link in the browser.
func (a *App) OpenURL(raw string) error {
	if !strings.HasPrefix(raw, "https://") && !strings.HasPrefix(raw, "http://") {
		return errors.New("只允许打开 HTTP 链接")
	}
	runtime.BrowserOpenURL(a.context(), raw)
	return nil
}

// SearchProjects queries the Modrinth catalog.
func (a *App) SearchProjects(req modrinth.SearchRequest) (modrinth.SearchResult, error) {
	if err := a.ready(); err != nil {
		return modrinth.SearchResult{}, err
	}
	req.ProjectType = modrinth.NormalizeProjectType(req.ProjectType)
	return modrinth.Search(a.context(), req)
}

func projectLoader(inst store.Instance, projectType string) string {
	if projectType == modrinth.TypeMod && inst.Loader != "" && inst.Loader != "vanilla" {
		return inst.Loader
	}
	return ""
}

// ListProjectVersions returns only files compatible with an instance's game
// version and loader.
func (a *App) ListProjectVersions(instanceID, projectID string) ([]modrinth.Version, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return nil, errors.New("请先选择一个版本")
	}
	project, err := modrinth.ProjectByID(a.context(), projectID)
	if err != nil {
		return nil, err
	}
	loaderName := projectLoader(inst, project.ProjectType)
	if project.ProjectType == modrinth.TypeMod && loaderName == "" {
		return nil, errors.New("原版游戏无法安装模组，请创建 Fabric / Quilt / Forge / NeoForge 版本")
	}
	versions, err := modrinth.ListVersions(a.context(), projectID, inst.MCVersion, loaderName)
	if err != nil {
		return nil, err
	}
	versions = modrinth.FilterCompatible(versions, inst.MCVersion, loaderName)
	if versions == nil {
		versions = []modrinth.Version{}
	}
	return versions, nil
}

// InstallProjectResult describes a completed content install.
type InstallProjectResult struct {
	FileName     string   `json:"fileName"`
	Path         string   `json:"path"`
	VersionID    string   `json:"versionId"`
	VersionName  string   `json:"versionName"`
	GameVersion  string   `json:"gameVersion"`
	Loader       string   `json:"loader"`
	Dependencies []string `json:"dependencies"`
	RollbackPath string   `json:"rollbackPath,omitempty"`
}

func resourceRewriter(source mirror.ResourceSource) func(string) []string {
	return func(original string) []string {
		return mirror.ResourceURLs(source, original)
	}
}

// InstallProject installs a mod, shader or resource pack into an instance.
func (a *App) InstallProject(instanceID, projectID, versionID string) (result InstallProjectResult, err error) {
	return a.installProject(instanceID, projectID, versionID, "", false)
}

func (a *App) installProject(instanceID, projectID, versionID, mirrorID string, force bool) (result InstallProjectResult, err error) {
	if err := a.ready(); err != nil {
		return InstallProjectResult{}, err
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return InstallProjectResult{}, errors.New("请先选择一个版本")
	}
	project, err := modrinth.ProjectByID(a.context(), projectID)
	if err != nil {
		return InstallProjectResult{}, err
	}
	if project.ProjectType == modrinth.TypeModpack {
		return InstallProjectResult{}, errors.New("整合包请使用整合包安装功能")
	}
	loaderName := projectLoader(inst, project.ProjectType)
	if project.ProjectType == modrinth.TypeMod && loaderName == "" {
		return InstallProjectResult{}, errors.New("原版游戏无法安装模组，请创建 Fabric / Quilt / Forge / NeoForge 版本")
	}
	var version modrinth.Version
	if versionID != "" {
		version, err = modrinth.VersionByID(a.context(), versionID)
		if err != nil {
			return InstallProjectResult{}, err
		}
		if !version.Compatible(inst.MCVersion, loaderName) {
			return InstallProjectResult{}, fmt.Errorf("所选文件不兼容 %s / %s，请使用自动匹配的版本", inst.MCVersion, firstNonEmpty(loaderName, "原版"))
		}
	} else {
		version, err = modrinth.BestVersion(a.context(), projectID, inst.MCVersion, loaderName)
		if err != nil {
			return InstallProjectResult{}, err
		}
	}
	subdir, err := modrinth.InstallPath(project.ProjectType)
	if err != nil {
		return InstallProjectResult{}, err
	}
	t := a.newTask("安装 " + project.Title)
	defer func() { t.finish(err) }()
	t.update("download", "正在下载 "+project.Title+"…", 0, 0)
	dir := filepath.Join(a.instanceDir(inst), subdir)
	rewriter := resourceRewriter(a.resourceSource(mirrorID))
	var path string
	if project.ProjectType == modrinth.TypeMod {
		cacheDir := filepath.Join(a.paths.Cache, "resources")
		tempPath, downloadErr := modrinth.DownloadFileAny(t.context(), version, cacheDir, rewriter)
		if downloadErr != nil {
			return InstallProjectResult{}, downloadErr
		}
		incompatibleFiles, incompatibleIDs := modrinthIncompatibilities(version)
		conflicts := detectModConflicts(a.instanceDir(inst), tempPath, incompatibleFiles, incompatibleIDs)
		if len(conflicts) > 0 && !force {
			_ = os.Remove(tempPath)
			return InstallProjectResult{}, &ResourceConflictError{Conflicts: conflicts}
		}
		path, err = installDownloadedFile(tempPath, dir)
	} else {
		path, err = modrinth.DownloadFileAny(t.context(), version, dir, rewriter)
	}
	if err != nil {
		return InstallProjectResult{}, err
	}
	result = InstallProjectResult{
		FileName: filepath.Base(path), Path: path, VersionID: version.ID,
		VersionName: version.Name, GameVersion: inst.MCVersion, Loader: loaderName,
		Dependencies: []string{},
	}
	if project.ProjectType == modrinth.TypeMod {
		t.update("dependencies", "正在检查前置模组…", 0, 0)
		deps, depErr := modrinth.InstallDependencies(
			t.context(), version, inst.MCVersion, loaderName, dir, 6,
			func(message string) { t.update("dependencies", message, 0, 0) },
		)
		if deps == nil {
			deps = []string{}
		}
		result.Dependencies = deps
		if depErr != nil {
			return result, depErr
		}
	}
	t.update("done", "安装完成", 1, 1)
	a.emitInstancesChanged()
	return result, nil
}

// ModpackImportResponse is returned by all modpack import entry points.
type ModpackImportResponse struct {
	Instance     InstanceView `json:"instance"`
	Format       string       `json:"format"`
	FormatLabel  string       `json:"formatLabel"`
	MissingFiles int          `json:"missingFiles"`
	Warnings     []string     `json:"warnings"`
}

// ImportModpackDialog lets the user pick and import a modpack. mirrorID
// selects the Modrinth/CurseForge download mirror; empty uses the configured
// default.
func (a *App) ImportModpackDialog(mirrorID string) (resp ModpackImportResponse, err error) {
	if err := a.ready(); err != nil {
		return ModpackImportResponse{}, err
	}
	path, err := runtime.OpenFileDialog(a.context(), runtime.OpenDialogOptions{
		Title: "导入整合包",
		Filters: []runtime.FileFilter{
			{DisplayName: "Minecraft 整合包 (*.mrpack;*.zip)", Pattern: "*.mrpack;*.zip"},
		},
	})
	if err != nil {
		return ModpackImportResponse{}, err
	}
	if strings.TrimSpace(path) == "" {
		return ModpackImportResponse{}, nil
	}
	t := a.newTask("导入整合包")
	defer func() { t.finish(err) }()
	return a.importModpack(t.context(), path, t, a.resourceSource(mirrorID))
}

func (a *App) importModpack(ctx context.Context, path string, t *progressTask, source mirror.ResourceSource) (ModpackImportResponse, error) {
	ins, err := modpack.Inspect(path)
	if err != nil {
		return ModpackImportResponse{}, err
	}
	opts := modpack.ImportOptions{
		CurseForgeAPIKey: a.store.Settings().CurseForgeAPIKey,
		URLRewriter:      resourceRewriter(source),
	}
	if !a.store.Settings().VersionIsolation {
		opts.GameDir = a.paths.Data
	}
	if ins.MCVersion == "" {
		latest, merr := a.mojang.LatestRelease(ctx)
		if merr != nil {
			return ModpackImportResponse{}, errors.New("该整合包没有声明游戏版本，且无法获取最新版本")
		}
		opts.FallbackMCVersion = latest
	}
	t.update("import", "正在导入 "+modpack.FormatLabel(ins.Format)+"…", 0, 0)
	imported, err := modpack.Import(ctx, a.store, path, opts, func(stage, message string, current, total int) {
		t.update(stage, message, int64(current), int64(total))
	})
	if err != nil {
		return ModpackImportResponse{}, err
	}
	inst := imported.Instance
	var warnings = imported.Warnings
	t.update("game", "正在准备游戏与加载器…", 0, 0)
	if err := a.installInstanceRuntime(ctx, &inst, t); err != nil {
		warnings = append(warnings, "文件已导入，但游戏环境安装失败："+err.Error())
	}
	updated, updateErr := a.store.UpdateInstance(inst.ID, func(i *store.Instance) error {
		i.Name = inst.Name
		i.LoaderVersion = inst.LoaderVersion
		i.LaunchVersionID = inst.LaunchVersionID
		i.Notes = inst.Notes
		return nil
	})
	if updateErr == nil {
		inst = updated
	}
	a.emitInstancesChanged()
	return ModpackImportResponse{
		Instance: a.instanceView(inst), Format: string(imported.Format),
		FormatLabel: imported.FormatLabel, MissingFiles: imported.MissingFiles, Warnings: warnings,
	}, nil
}

// InstallModpackProject downloads and imports a Modrinth modpack version.
func (a *App) InstallModpackProject(projectID, versionID, mirrorID string) (resp ModpackImportResponse, err error) {
	if err := a.ready(); err != nil {
		return ModpackImportResponse{}, err
	}
	source := a.resourceSource(mirrorID)
	t := a.newTask("安装整合包")
	defer func() { t.finish(err) }()
	version, err := modrinth.VersionByID(t.context(), versionID)
	if err != nil {
		return ModpackImportResponse{}, err
	}
	file, err := version.Primary()
	if err != nil {
		return ModpackImportResponse{}, err
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".mrpack") {
		return ModpackImportResponse{}, errors.New("该文件不是 Modrinth 整合包")
	}
	dir := filepath.Join(a.paths.Cache, "modpacks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ModpackImportResponse{}, err
	}
	t.update("download", "正在下载整合包…", 0, 0)
	dest := filepath.Join(dir, filepath.Base(file.Filename))
	if err := httpx.DownloadAny(t.context(), mirror.ResourceURLs(source, file.URL), httpx.File{
		Dest: dest, SHA512: file.Hashes["sha512"], SHA1: file.Hashes["sha1"],
		Size: file.Size, Verify: true,
		Progress: func(done, total int64) {
			t.update("download", "正在下载整合包…", done, total)
		},
	}); err != nil {
		return ModpackImportResponse{}, err
	}
	return a.importModpack(t.context(), dest, t, source)
}

// PickExternalLauncher lets the user select another launcher's data or
// instance folder and previews the discovered versions.
func (a *App) PickExternalLauncher() (external.ScanResult, error) {
	return a.PickVersionDirectory()
}

// PickVersionDirectory lets the user choose any game directory, versions
// directory or single version folder and previews the versions inside.
func (a *App) PickVersionDirectory() (external.ScanResult, error) {
	if err := a.ready(); err != nil {
		return external.ScanResult{}, err
	}
	path, err := runtime.OpenDirectoryDialog(a.context(), runtime.OpenDialogOptions{
		Title: "选择游戏目录、versions 目录或版本文件夹",
	})
	if err != nil {
		return external.ScanResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return external.ScanResult{}, nil
	}
	return external.Scan(path)
}

// ScanVersionDirectory inspects a known path without opening a dialog.
func (a *App) ScanVersionDirectory(path string) (external.ScanResult, error) {
	if err := a.ready(); err != nil {
		return external.ScanResult{}, err
	}
	return external.Scan(path)
}

// SearchMinecraftFolders searches a folder tree for .minecraft/versions
// directories and returns every importable version it recognises.
func (a *App) SearchMinecraftFolders(root string) ([]external.ScanResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("请选择要搜索的目录")
	}
	task := a.newTask("搜索 .minecraft 文件夹")
	ctx, cancel := context.WithTimeout(task.context(), 45*time.Second)
	defer cancel()
	results, err := external.SearchMinecraftFolders(ctx, root, func(directory string, found int) {
		task.update("scan", fmt.Sprintf("正在搜索 %s · 已找到 %d 个目录", directory, found), 0, 0)
	})
	task.finish(err)
	if err != nil && len(results) == 0 {
		return nil, err
	}
	return results, nil
}

// DetectLocalLaunchers scans the well-known official launcher, HMCL, PCL2,
// Prism/MultiMC and MultiMC data directories on this machine.
func (a *App) DetectLocalLaunchers() ([]external.ScanResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	results := []external.ScanResult{}
	seen := map[string]bool{}
	for _, root := range external.KnownRoots() {
		if !root.Exists {
			continue
		}
		key := strings.ToLower(root.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		scan, err := external.Scan(root.Path)
		if err != nil || len(scan.Candidates) == 0 {
			continue
		}
		if scan.Source == "其他启动器" {
			scan.Source = root.Label
		}
		results = append(results, scan)
	}
	if len(results) == 0 {
		return results, errors.New("没有在本机找到官方启动器或其他启动器的版本，请手动选择目录")
	}
	return results, nil
}

// CustomImportRequest describes a manually configured version import.
type CustomImportRequest struct {
	Name          string `json:"name"`
	MCVersion     string `json:"mcVersion"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loaderVersion"`
	SourceDir     string `json:"sourceDir"`
}

// PickCustomSourceDirectory lets the user choose an optional game/instance
// directory whose mods, shaders, resource packs, saves and configs are copied.
func (a *App) PickCustomSourceDirectory() (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	path, err := runtime.OpenDirectoryDialog(a.context(), runtime.OpenDialogOptions{
		Title: "选择游戏目录、实例目录或要搜索的父目录（例如 D:/Games）",
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(path), nil
}

// ImportCustomInstance creates an instance with manually supplied metadata and
// optional content copied from a game or instance folder.
func (a *App) ImportCustomInstance(req CustomImportRequest) (response ExternalImportResponse, err error) {
	if err := a.ready(); err != nil {
		return ExternalImportResponse{}, err
	}
	mcVersion := strings.TrimSpace(req.MCVersion)
	if mcVersion == "" {
		return ExternalImportResponse{}, errors.New("请填写 Minecraft 版本")
	}
	loaderType := loader.Normalize(req.Loader)
	switch loaderType {
	case "vanilla", "fabric", "quilt", "forge", "neoforge":
	default:
		return ExternalImportResponse{}, errors.New("不支持的模组加载器")
	}
	if _, err := a.mojang.ManifestEntry(a.context(), mcVersion); err != nil {
		return ExternalImportResponse{}, fmt.Errorf("Minecraft %s 不在官方版本清单中: %w", mcVersion, err)
	}
	sourceDir := strings.TrimSpace(req.SourceDir)
	if sourceDir != "" {
		if stat, statErr := os.Stat(sourceDir); statErr != nil || !stat.IsDir() {
			return ExternalImportResponse{}, errors.New("来源目录不存在或不是文件夹")
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = mcVersion + " 自定义版本"
	}
	if _, nameErr := store.SafeName(name); nameErr != nil {
		name = mcVersion + " 自定义版本"
	}
	loaderVersion := strings.TrimSpace(req.LoaderVersion)
	if loaderType == "vanilla" {
		loaderVersion = ""
	}

	t := a.newTask("导入自定义版本 " + name)
	defer func() { t.finish(err) }()
	inst, err := a.store.CreateInstance(store.Instance{
		Name:            name,
		MCVersion:       mcVersion,
		Loader:          loaderType,
		LoaderVersion:   loaderVersion,
		LaunchVersionID: mcVersion,
		MemoryMB:        a.store.Settings().DefaultMemoryMB,
		Source:          "custom",
		Notes:           "自定义版本导入",
	})
	if err != nil {
		return ExternalImportResponse{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = a.store.DeleteInstance(inst.ID, true)
		}
	}()
	stats := external.CopyStats{}
	if sourceDir != "" {
		t.update("copy", "正在复制版本内容…", 0, 1)
		stats, err = external.CopyContentFrom(sourceDir, a.instanceDir(inst))
		if err != nil {
			return ExternalImportResponse{}, err
		}
		t.update("copy", "版本内容复制完成", 1, 1)
	}
	warnings := []string{}
	if err := a.installInstanceRuntime(t.context(), &inst, t); err != nil {
		warnings = append(warnings, "版本内容已导入，但游戏/加载器安装失败："+err.Error())
	}
	cleanup = false
	a.emitInstancesChanged()
	return ExternalImportResponse{
		Instance: a.instanceView(inst), Source: "自定义导入",
		FilesCopied: stats.Files, ModsCopied: stats.Mods,
		ResourcePacksCopied: stats.ResourcePacks, ShaderPacksCopied: stats.ShaderPacks,
		SavesCopied: stats.Saves, Warnings: warnings,
	}, nil
}

// ExternalImportResponse describes an imported third-party launcher instance.
type ExternalImportResponse struct {
	Instance            InstanceView `json:"instance"`
	Source              string       `json:"source"`
	FilesCopied         int          `json:"filesCopied"`
	ModsCopied          int          `json:"modsCopied"`
	ResourcePacksCopied int          `json:"resourcePacksCopied"`
	ShaderPacksCopied   int          `json:"shaderPacksCopied"`
	SavesCopied         int          `json:"savesCopied"`
	Warnings            []string     `json:"warnings"`
}

// resolveExternalMCVersion validates the detected game version and, when a
// launcher used a custom profile name, finds the official version ID inside the
// candidate name. This keeps imports working even when the version JSON was
// renamed by another launcher.
func (a *App) resolveExternalMCVersion(ctx context.Context, candidate external.Candidate) (string, error) {
	if version := strings.TrimSpace(candidate.MCVersion); version != "" {
		if _, err := a.mojang.ManifestEntry(ctx, version); err == nil {
			return version, nil
		}
	}
	manifest, err := a.mojang.Manifest(ctx)
	if err != nil {
		return "", fmt.Errorf("无法读取 Minecraft 版本清单: %w", err)
	}
	texts := []string{candidate.MCVersion, candidate.Name}
	for _, entry := range manifest.Versions {
		for _, text := range texts {
			if containsVersionToken(text, entry.ID) {
				return entry.ID, nil
			}
		}
	}
	if version := strings.TrimSpace(candidate.MCVersion); version != "" {
		return "", fmt.Errorf("Minecraft %s 不在官方版本清单中，请使用自定义导入选择正确的版本", version)
	}
	return "", errors.New("无法识别该版本的 Minecraft 版本，请使用自定义导入填写")
}

func containsVersionToken(text, version string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	version = strings.ToLower(strings.TrimSpace(version))
	if text == "" || version == "" {
		return false
	}
	for offset := 0; offset <= len(text)-len(version); {
		index := strings.Index(text[offset:], version)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(version)
		beforeOK := index == 0 || !isVersionTokenChar(text[index-1])
		afterOK := end == len(text) || !isVersionTokenChar(text[end])
		if beforeOK && afterOK {
			return true
		}
		offset = index + 1
	}
	return false
}

func isVersionTokenChar(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_'
}

// ImportExternalInstance copies user content and rebuilds the game runtime for
// a version found in another launcher.
func (a *App) ImportExternalInstance(root, candidateID, nameOverride string) (response ExternalImportResponse, err error) {
	if err := a.ready(); err != nil {
		return ExternalImportResponse{}, err
	}
	candidate, err := external.FindCandidate(root, candidateID)
	if err != nil {
		return ExternalImportResponse{}, err
	}
	mcVersion, err := a.resolveExternalMCVersion(a.context(), candidate)
	if err != nil {
		return ExternalImportResponse{}, err
	}
	candidate.MCVersion = mcVersion
	name := strings.TrimSpace(nameOverride)
	if name == "" {
		name = candidate.Name
	}
	if _, nameErr := store.SafeName(name); nameErr != nil {
		name = candidate.MCVersion + " 版本"
	}
	t := a.newTask("导入 " + name)
	defer func() { t.finish(err) }()
	inst, err := a.store.CreateInstance(store.Instance{
		Name:            name,
		MCVersion:       candidate.MCVersion,
		Loader:          candidate.Loader,
		LoaderVersion:   candidate.LoaderVersion,
		LaunchVersionID: candidate.MCVersion,
		MemoryMB:        a.store.Settings().DefaultMemoryMB,
		Source:          "external:" + candidate.Source,
		Notes:           "从 " + candidate.Source + " 版本 " + candidate.Name + " 导入",
	})
	if err != nil {
		return ExternalImportResponse{}, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = a.store.DeleteInstance(inst.ID, true)
		}
	}()
	t.update("copy", "正在复制版本内容…", 0, 1)
	stats, err := external.CopyContent(candidate, a.instanceDir(inst))
	if err != nil {
		return ExternalImportResponse{}, err
	}
	t.update("copy", "版本内容复制完成", 1, 1)
	warnings := append([]string{}, candidate.Warnings...)
	if candidate.Loader != "" && candidate.Loader != "vanilla" && strings.TrimSpace(candidate.LoaderVersion) == "" {
		warnings = append(warnings, "未识别到加载器版本，已按最新版本准备运行环境；如整合包需要固定加载器版本，请使用自定义导入填写")
	}
	if candidate.Loader == "vanilla" && candidate.ModCount > 0 {
		warnings = append(warnings, "该版本包含模组但未声明模组加载器，导入后可能无法加载这些模组")
	}
	if err := a.installInstanceRuntime(t.context(), &inst, t); err != nil {
		warnings = append(warnings, "版本内容已导入，但游戏/加载器安装失败："+err.Error())
	}
	cleanup = false
	a.emitInstancesChanged()
	return ExternalImportResponse{
		Instance: a.instanceView(inst), Source: candidate.Source,
		FilesCopied: stats.Files, ModsCopied: stats.Mods,
		ResourcePacksCopied: stats.ResourcePacks, ShaderPacksCopied: stats.ShaderPacks,
		SavesCopied: stats.Saves, Warnings: warnings,
	}, nil
}

// ExportModpackDialog exports the instance as a portable .mrpack.
func (a *App) ExportModpackDialog(instanceID string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return "", errors.New("版本不存在")
	}
	dest, err := runtime.SaveFileDialog(a.context(), runtime.SaveDialogOptions{
		Title:           "导出 Modrinth 整合包",
		DefaultFilename: safeFilename(inst.Name) + ".mrpack",
		Filters:         []runtime.FileFilter{{DisplayName: "Modrinth 整合包 (*.mrpack)", Pattern: "*.mrpack"}},
	})
	if err != nil || strings.TrimSpace(dest) == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".mrpack") {
		dest += ".mrpack"
	}
	if err := modpack.ExportMRPack(a.context(), inst, a.instanceDir(inst), dest); err != nil {
		return "", err
	}
	return dest, nil
}

// ServerPackExportResult describes an exported server pack.
type ServerPackExportResult struct {
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

// ExportServerPackDialog exports an instance as a ready-to-configure server
// pack, including mods, configs, loader runtime, scripts, and an EULA template.
func (a *App) ExportServerPackDialog(instanceID string) (response ServerPackExportResult, err error) {
	if err := a.ready(); err != nil {
		return ServerPackExportResult{}, err
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return ServerPackExportResult{}, errors.New("版本不存在")
	}
	dest, err := runtime.SaveFileDialog(a.context(), runtime.SaveDialogOptions{
		Title:           "导出服务器包",
		DefaultFilename: safeFilename(inst.Name) + "-server.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "服务器包 (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || strings.TrimSpace(dest) == "" {
		return ServerPackExportResult{}, err
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".zip") {
		dest += ".zip"
	}
	t := a.newTask("导出服务器包")
	defer func() { t.finish(err) }()
	pack, packErr := serverpack.Export(t.context(), serverpack.Options{
		Client:    a.mojang,
		Instance:  inst,
		GameDir:   a.instanceDir(inst),
		Dest:      dest,
		JavaMajor: a.requiredJava(inst),
		Sources:   mirror.Build(customsForMirrors(a.store.Settings().CustomMirrors)),
		Progress: func(stage, message string, current, total int) {
			t.update(stage, message, int64(current), int64(total))
		},
	})
	response = ServerPackExportResult{Path: pack.Path, Warnings: pack.Warnings}
	return response, packErr
}

// ExportInstanceBackupDialog exports a Prism/MultiMC-compatible zip.
func (a *App) ExportInstanceBackupDialog(instanceID string) (string, error) {
	if err := a.ready(); err != nil {
		return "", err
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return "", errors.New("版本不存在")
	}
	dest, err := runtime.SaveFileDialog(a.context(), runtime.SaveDialogOptions{
		Title:           "导出版本备份",
		DefaultFilename: safeFilename(inst.Name) + "-backup.zip",
		Filters:         []runtime.FileFilter{{DisplayName: "版本备份 (*.zip)", Pattern: "*.zip"}},
	})
	if err != nil || strings.TrimSpace(dest) == "" {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(dest), ".zip") {
		dest += ".zip"
	}
	if err := modpack.ExportZip(a.context(), inst, a.instanceDir(inst), dest); err != nil {
		return "", err
	}
	return dest, nil
}

// GetJavaRuntimes scans installed Java runtimes.
func (a *App) GetJavaRuntimes() ([]java.Runtime, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	extras := []string{a.store.Settings().DefaultJavaPath}
	for _, inst := range a.store.Instances() {
		if inst.JavaPath != "" {
			extras = append(extras, inst.JavaPath)
		}
	}
	runtimes := a.javaMgr.Scan(a.context(), extras)
	if runtimes == nil {
		runtimes = []java.Runtime{}
	}
	return runtimes, nil
}

// GetJavaReleases returns Adoptium's available Java feature versions.
func (a *App) GetJavaReleases() (java.ReleaseInfo, error) {
	if err := a.ready(); err != nil {
		return java.ReleaseInfo{}, err
	}
	return a.javaMgr.Releases(a.context())
}

// GetJavaPackage resolves a downloadable Temurin JRE.
func (a *App) GetJavaPackage(feature int) (java.Archive, error) {
	if err := a.ready(); err != nil {
		return java.Archive{}, err
	}
	return a.javaMgr.PackageInfo(a.context(), feature)
}

// InstallJava downloads and installs a managed Temurin JRE.
func (a *App) InstallJava(feature int) (rt java.Runtime, err error) {
	if err := a.ready(); err != nil {
		return java.Runtime{}, err
	}
	t := a.newTask(fmt.Sprintf("安装 Java %d", feature))
	defer func() { t.finish(err) }()
	rt, err = a.javaMgr.Install(t.context(), feature, t.javaBytesProgress("java"))
	if err != nil {
		return java.Runtime{}, err
	}
	settings := a.store.Settings()
	if strings.TrimSpace(settings.DefaultJavaPath) == "" {
		settings.DefaultJavaPath = rt.Path
		if _, saveErr := a.store.SaveSettings(settings); saveErr != nil {
			return java.Runtime{}, saveErr
		}
	}
	return rt, nil
}

func openPath(target string) error {
	path := strings.TrimSpace(target)
	if path == "" {
		return errors.New("目录路径为空")
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	if stat, err := os.Stat(path); err != nil || !stat.IsDir() {
		// The launcher owns these folders; recreate a missing one so the
		// "open folder" action still works after manual cleanup.
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("目录不存在或无法创建: %w", err)
		}
	}
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "windows":
		cmd = exec.Command("explorer.exe", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	// Do not hide the child window: explorer.exe / Finder / xdg-open are GUI
	// programs and SW_HIDE makes the folder window invisible or prevents it
	// from opening at all.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法打开目录: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func safeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "instance"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-", "<", "-", ">", "-", "|", "-")
	name = replacer.Replace(name)
	if len(name) > 80 {
		name = name[:80]
	}
	return name
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
