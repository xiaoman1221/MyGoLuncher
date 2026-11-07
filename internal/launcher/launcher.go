package launcher

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/sysutil"
	"MyMinecraftGoLuncher/internal/util"
)

// Launcher builds and starts Minecraft processes.
type Launcher struct {
	Mojang *mojang.Client
	Paths  store.Paths
}

// New creates a launcher.
func New(client *mojang.Client, paths store.Paths) *Launcher {
	return &Launcher{Mojang: client, Paths: paths}
}

// Options controls one launch.
type Options struct {
	Instance store.Instance
	Settings store.Settings
	JavaPath string
	GameDir  string

	PlayerName    string
	PlayerUUID    string
	AccessToken   string
	UserType      string
	XUID          string
	AuthlibJar    string
	AuthlibServer string

	// Server optionally connects directly to a Minecraft server after launch.
	Server *ServerAddress

	OnLine func(line string)
}

// Process is a running game.
type Process struct {
	InstanceID string
	StartedAt  time.Time
	LogPath    string

	cmd  *exec.Cmd
	done chan struct{}

	mu       sync.Mutex
	exitCode int
	err      error
	stopOnce sync.Once
	stopped  atomic.Bool
}

// Done closes when the process has exited and its output is drained.
func (p *Process) Done() <-chan struct{} { return p.done }

// ExitCode returns the process exit code.
func (p *Process) ExitCode() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitCode
}

// Err returns the wait error, if any.
func (p *Process) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// Duration returns elapsed run time.
func (p *Process) Duration() time.Duration { return time.Since(p.StartedAt) }

// PID returns the operating-system process ID of the Java game.
func (p *Process) PID() int {
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// Stop terminates the process tree.
func (p *Process) Stop() error {
	var err error
	p.stopOnce.Do(func() {
		p.stopped.Store(true)
		err = sysutil.KillProcessTree(p.cmd)
	})
	return err
}

// StoppedByUser reports whether the launcher deliberately terminated the game.
func (p *Process) StoppedByUser() bool { return p.stopped.Load() }

// Start validates an installed version, prepares natives, and launches Java.
func (l *Launcher) Start(ctx context.Context, opts Options) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	inst := opts.Instance
	if inst.ID == "" {
		return nil, errors.New("版本无效")
	}
	launchVersion := strings.TrimSpace(inst.LaunchVersionID)
	if launchVersion == "" {
		launchVersion = inst.MCVersion
	}
	chain, err := l.Mojang.Chain(launchVersion)
	if err != nil {
		return nil, fmt.Errorf("启动版本不完整: %w", err)
	}
	merged := mojang.Merge(chain)
	if merged.MainClass == "" {
		return nil, errors.New("版本档案缺少主类")
	}
	if !l.Mojang.IsInstalled(launchVersion) && !l.Mojang.IsInstalled(inst.MCVersion) {
		return nil, errors.New("游戏文件尚未安装完成，请先下载版本")
	}
	if opts.JavaPath == "" {
		return nil, errors.New("没有可用的 Java 运行时")
	}
	if _, err := os.Stat(opts.JavaPath); err != nil {
		return nil, fmt.Errorf("Java 不存在: %w", err)
	}
	gameDir := strings.TrimSpace(opts.GameDir)
	if gameDir == "" {
		versionID := strings.TrimSpace(inst.VersionID)
		if versionID == "" {
			versionID = inst.ID
		}
		gameDir = filepath.Join(l.Paths.Versions, versionID)
	}
	if !filepath.IsAbs(gameDir) {
		gameDir, _ = filepath.Abs(gameDir)
	}
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		return nil, err
	}

	nativesBase, nativesPath, err := l.prepareNatives(merged, gameDir, launchVersion)
	if err != nil {
		return nil, err
	}
	skipVanillaClient := isModernModLoader(merged, inst.Loader)
	classpath, err := l.buildClasspath(merged, chain, skipVanillaClient)
	if err != nil {
		return nil, err
	}
	gameAssets, err := l.prepareLegacyAssets(merged, gameDir)
	if err != nil {
		return nil, err
	}
	jvmArgs, gameArgs, err := l.buildArguments(merged, opts, gameDir, nativesBase, nativesPath, classpath, gameAssets, launchVersion)
	if err != nil {
		return nil, err
	}
	allArgs := append(jvmArgs, merged.MainClass)
	allArgs = append(allArgs, gameArgs...)

	cmd := exec.CommandContext(ctx, opts.JavaPath, allArgs...)
	cmd.Dir = gameDir
	sysutil.ConfigureProcess(cmd)
	cmd.Env = os.Environ()

	logDir := filepath.Join(gameDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, "launcher-"+time.Now().Format("20060102-150405")+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(logFile, "[MyGoLuncher] Java: %s\n", opts.JavaPath)
	fmt.Fprintf(logFile, "[MyGoLuncher] Version: %s\n", launchVersion)
	fmt.Fprintf(logFile, "[MyGoLuncher] Command: %s %s\n\n", opts.JavaPath, strings.Join(redactCommand(allArgs), " "))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logFile.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("无法启动 Java: %w", err)
	}

	proc := &Process{
		InstanceID: inst.ID,
		StartedAt:  time.Now(),
		LogPath:    logPath,
		cmd:        cmd,
		done:       make(chan struct{}),
	}
	var writeMu sync.Mutex
	writeLine := func(line string) {
		writeMu.Lock()
		_, _ = io.WriteString(logFile, line+"\n")
		writeMu.Unlock()
		if opts.OnLine != nil {
			opts.OnLine(line)
		}
	}
	var readers sync.WaitGroup
	readers.Add(2)
	stream := func(r io.Reader) {
		defer readers.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64<<10), 2<<20)
		parser := &logParser{}
		for scanner.Scan() {
			for _, line := range parser.Feed(scanner.Text()) {
				writeLine(line)
			}
		}
		for _, line := range parser.Flush() {
			writeLine(line)
		}
	}
	go stream(stdout)
	go stream(stderr)
	go func() {
		// Drain stdout/stderr before Wait: Wait closes the pipes, which would
		// truncate the final lines of a crash log.
		readers.Wait()
		waitErr := cmd.Wait()
		code := 0
		if cmd.ProcessState != nil {
			code = cmd.ProcessState.ExitCode()
		}
		proc.mu.Lock()
		proc.exitCode = code
		proc.err = waitErr
		proc.mu.Unlock()
		_ = logFile.Close()
		writeMu.Lock()
		writeMu.Unlock()
		close(proc.done)
	}()
	return proc, nil
}

func (l *Launcher) prepareNatives(merged mojang.Version, gameDir, versionID string) (string, string, error) {
	safeVersion := sanitizeComponent(versionID)
	base := filepath.Join(gameDir, "natives", safeVersion)
	if filepath.Base(filepath.Dir(gameDir)) == "versions" {
		// Match PCL/HMCL isolated version folders: the native directory sits
		// next to the version JSON/JAR instead of inside a nested natives tree.
		base = filepath.Join(gameDir, filepath.Base(gameDir)+"-natives")
	}
	suffix := mojang.NativeDirectorySuffix(merged.Arguments.JVM)
	effective := base
	if suffix != "" {
		effective = filepath.Join(base, suffix)
	}
	if err := os.RemoveAll(base); err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(effective, 0o755); err != nil {
		return "", "", err
	}
	extracted := 0
	for _, lib := range merged.Libraries {
		if !mojang.RulesAllow(lib.Rules, nil) {
			continue
		}
		dl := mojang.NativeArtifact(lib)
		if dl == nil {
			continue
		}
		path := dl.Path
		if path == "" {
			var err error
			path, err = mojang.ArtifactPath(lib.Name)
			if err != nil {
				continue
			}
		}
		jar := filepath.Join(l.Paths.Libraries, filepath.FromSlash(path))
		if _, err := os.Stat(jar); err != nil {
			continue
		}
		excludes := []string{}
		if lib.Extract != nil {
			excludes = lib.Extract.Exclude
		}
		if err := mojang.ExtractNativeJar(jar, effective, excludes); err != nil {
			return "", "", fmt.Errorf("释放本地库 %s 失败: %w", filepath.Base(jar), err)
		}
		extracted++
	}
	if extracted == 0 && suffix != "" {
		// A small number of new snapshots expect the subdirectory even when no
		// classic native library is declared.
		_ = os.MkdirAll(effective, 0o755)
	}
	return base, effective, nil
}

func (l *Launcher) buildClasspath(merged mojang.Version, chain []mojang.Version, skipVanillaClient bool) (string, error) {
	var entries []string
	seen := map[string]bool{}
	for _, lib := range merged.Libraries {
		if !mojang.RulesAllow(lib.Rules, nil) {
			continue
		}
		// Native-only legacy libraries have no classes artifact. Everything
		// else must be on the classpath, including LWJGL artifacts that also
		// happen to declare native classifiers.
		if lib.Downloads.Artifact == nil && len(lib.Natives) > 0 {
			continue
		}
		dl, err := mojang.LibraryDownload(lib)
		if err != nil {
			return "", fmt.Errorf("解析运行库 %s 失败: %w", lib.Name, err)
		}
		path := dl.Path
		if path == "" {
			path, err = mojang.ArtifactPath(lib.Name)
			if err != nil {
				return "", err
			}
		}
		full := filepath.Join(l.Paths.Libraries, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			return "", fmt.Errorf("运行库缺失：%s，请重新安装版本", filepath.Base(full))
		}
		key := strings.ToLower(full)
		if !seen[key] {
			seen[key] = true
			entries = append(entries, full)
		}
	}
	if skipVanillaClient {
		// Modern Forge/NeoForge use the patched client jar from their installer
		// and supply it through FML's module locator. Adding the vanilla client
		// jar as well makes both jars export the same packages and ModLauncher
		// fails with a module ResolutionException.
		if len(entries) == 0 {
			return "", errors.New("版本档案没有可用的运行库")
		}
		return strings.Join(entries, string(os.PathListSeparator)), nil
	}
	clientJar := ""
	for _, v := range chain {
		if _, ok := v.Downloads["client"]; ok {
			candidate := l.Mojang.ClientJar(v.ID)
			if _, err := os.Stat(candidate); err == nil {
				clientJar = candidate
				break
			}
		}
	}
	if clientJar == "" {
		return "", errors.New("客户端主文件缺失，请重新安装版本")
	}
	entries = append(entries, clientJar)
	return strings.Join(entries, string(os.PathListSeparator)), nil
}

func isModernModLoader(version mojang.Version, loaderType string) bool {
	switch strings.ToLower(strings.TrimSpace(loaderType)) {
	case "forge", "neoforge", "neo-forge":
	default:
		return false
	}
	mainClass := strings.ToLower(version.MainClass)
	return strings.Contains(mainClass, "bootstraplauncher") || strings.Contains(mainClass, "modlauncher")
}

func (l *Launcher) prepareLegacyAssets(merged mojang.Version, gameDir string) (string, error) {
	indexID := merged.AssetIndex.ID
	if indexID == "" {
		indexID = merged.Assets
	}
	if indexID == "" {
		return l.Paths.Assets, nil
	}
	indexPath := filepath.Join(l.Paths.Assets, "indexes", indexID+".json")
	var catalog mojang.AssetCatalog
	if err := util.ReadJSON(indexPath, &catalog); err != nil {
		return l.Paths.Assets, nil
	}
	objects := filepath.Join(l.Paths.Assets, "objects")
	switch {
	case catalog.Virtual:
		dest := filepath.Join(l.Paths.Assets, "virtual", indexID)
		if err := mojang.MaterializeAssets(catalog, objects, dest); err != nil {
			return "", fmt.Errorf("准备旧版资源失败: %w", err)
		}
		return dest, nil
	case catalog.MapsToResources():
		dest := filepath.Join(gameDir, "resources")
		if err := mojang.MaterializeAssets(catalog, objects, dest); err != nil {
			return "", fmt.Errorf("准备旧版资源失败: %w", err)
		}
		return dest, nil
	default:
		return l.Paths.Assets, nil
	}
}

func (l *Launcher) buildArguments(merged mojang.Version, opts Options, gameDir, nativesBase, nativesPath, classpath, gameAssets, launchVersion string) ([]string, []string, error) {
	playerName := strings.TrimSpace(opts.PlayerName)
	if playerName == "" {
		playerName = strings.TrimSpace(opts.Settings.PlayerName)
	}
	if playerName == "" {
		playerName = "Steve"
	}
	memory := opts.Instance.MemoryMB
	if memory < 512 {
		memory = opts.Settings.DefaultMemoryMB
	}
	if memory < 512 {
		memory = 4096
	}
	uuid := strings.ReplaceAll(strings.TrimSpace(opts.PlayerUUID), "-", "")
	if uuid == "" {
		uuid = offlineUUID(playerName)
	}
	accessToken := strings.TrimSpace(opts.AccessToken)
	if accessToken == "" {
		accessToken = "0"
	}
	userType := strings.TrimSpace(opts.UserType)
	if userType == "" {
		userType = "legacy"
	}
	assetsIndex := merged.AssetIndex.ID
	if assetsIndex == "" {
		assetsIndex = merged.Assets
	}
	versionType := merged.Type
	if versionType == "" {
		versionType = "release"
	}
	nativesSuffix := mojang.NativeDirectorySuffix(merged.Arguments.JVM)
	nativesBaseDir := nativesBase
	if nativesSuffix == "" {
		nativesBaseDir = nativesPath
	}
	values := map[string]string{
		"auth_player_name":      playerName,
		"version_name":          launchVersion,
		"game_directory":        gameDir,
		"assets_root":           l.Paths.Assets,
		"game_assets":           gameAssets,
		"assets_index_name":     assetsIndex,
		"auth_uuid":             uuid,
		"auth_access_token":     accessToken,
		"auth_session":          accessToken,
		"clientid":              "minecraft-go-launcher",
		"auth_xuid":             opts.XUID,
		"user_type":             userType,
		"version_type":          versionType,
		"natives_directory":     nativesBaseDir,
		"launcher_name":         "MyGoLuncher",
		"launcher_version":      "1.0.4",
		"classpath":             classpath,
		"classpath_separator":   string(os.PathListSeparator),
		"library_directory":     l.Paths.Libraries,
		"resolution_width":      "1280",
		"resolution_height":     "720",
		"quickPlayPath":         "",
		"quickPlaySingleplayer": "",
		"quickPlayMultiplayer":  "",
	}
	quickPlay := opts.Server != nil && supportsQuickPlay(merged)
	if quickPlay {
		values["quickPlayMultiplayer"] = opts.Server.String()
	}
	features := map[string]bool{
		"is_demo_user":               false,
		"has_custom_resolution":      false,
		"has_quick_plays_support":    quickPlay,
		"is_quick_play_singleplayer": false,
		"is_quick_play_multiplayer":  quickPlay,
		"is_quick_play_realms":       false,
	}

	replace := func(s string) string {
		for key, value := range values {
			s = strings.ReplaceAll(s, "${"+key+"}", value)
		}
		return unknownPlaceholder.ReplaceAllString(s, "")
	}

	minMemory := memory / 2
	if minMemory > 1024 {
		minMemory = 1024
	}
	if minMemory < 256 {
		minMemory = 256
	}
	jvmArgs := []string{
		fmt.Sprintf("-Xms%dM", minMemory),
		fmt.Sprintf("-Xmx%dM", memory),
		"-Djava.library.path=" + nativesPath,
		"-Dminecraft.launcher.brand=MyGoLuncher",
		"-Dminecraft.launcher.version=1.0.4",
		"-Dlog4j2.formatMsgNoLookups=true",
	}
	if merged.Logging.Client != nil {
		cfg := filepath.Join(l.Paths.Assets, "log_configs", merged.Logging.Client.File.ID)
		if _, err := os.Stat(cfg); err == nil {
			arg := strings.ReplaceAll(merged.Logging.Client.Argument, "${path}", cfg)
			jvmArgs = append(jvmArgs, arg)
		}
	}
	if opts.AuthlibJar != "" {
		agent := "-javaagent:" + opts.AuthlibJar
		if opts.AuthlibServer != "" {
			agent += "=" + opts.AuthlibServer
		}
		jvmArgs = append(jvmArgs, agent)
	}
	if opts.Settings.ExtraJVMArgs != "" {
		jvmArgs = append(jvmArgs, strings.Fields(opts.Settings.ExtraJVMArgs)...)
	}
	hasClasspathFlag := false
	for _, arg := range merged.Arguments.JVM {
		if !mojang.RulesAllow(arg.Rules, features) {
			continue
		}
		for _, value := range arg.Value {
			value = replace(value)
			if value == "" {
				continue
			}
			if value == "-cp" || value == "-classpath" {
				hasClasspathFlag = true
			}
			jvmArgs = append(jvmArgs, value)
		}
	}
	if !hasClasspathFlag {
		jvmArgs = append(jvmArgs, "-cp", classpath)
	}

	var gameArgs []string
	if len(merged.Arguments.Game) > 0 {
		for _, arg := range merged.Arguments.Game {
			if !mojang.RulesAllow(arg.Rules, features) {
				continue
			}
			for _, value := range arg.Value {
				value = replace(value)
				if value != "" {
					gameArgs = append(gameArgs, value)
				}
			}
		}
	} else if strings.TrimSpace(merged.MinecraftArguments) != "" {
		for _, field := range strings.Fields(merged.MinecraftArguments) {
			gameArgs = append(gameArgs, replace(field))
		}
	}
	if len(gameArgs) == 0 {
		return nil, nil, errors.New("版本档案缺少游戏启动参数")
	}
	if opts.Server != nil && !quickPlay {
		// Minecraft removed --server/--port in 1.20 in favour of quick play.
		// Older versions still accept these direct-connect arguments.
		gameArgs = append(gameArgs, "--server", opts.Server.HostArg(), "--port", fmt.Sprintf("%d", opts.Server.Port))
	}
	return jvmArgs, gameArgs, nil
}

// supportsQuickPlay reports whether the version metadata includes the 1.20+
// quick play argument. This is more reliable than parsing snapshot names.
func supportsQuickPlay(version mojang.Version) bool {
	for _, argument := range version.Arguments.Game {
		for _, value := range argument.Value {
			if strings.Contains(value, "quickPlayMultiplayer") {
				return true
			}
		}
	}
	return false
}

var unknownPlaceholder = regexp.MustCompile(`\$\{[^}]+\}`)

// redactCommand hides credentials before the launch command is written to the
// log file. Minecraft accepts the access token both as a separate argument and
// in --flag=value form depending on the version.
func redactCommand(args []string) []string {
	out := append([]string(nil), args...)
	for idx := range out {
		lower := strings.ToLower(out[idx])
		switch lower {
		case "--accesstoken", "--authsession", "--session", "--token":
			if idx+1 < len(out) {
				out[idx+1] = "<redacted>"
			}
			continue
		}
		for _, flag := range []string{"--accesstoken=", "--authsession=", "--session=", "--token="} {
			if strings.HasPrefix(lower, flag) {
				out[idx] = out[idx][:len(flag)] + "<redacted>"
				break
			}
		}
	}
	return out
}

func offlineUUID(name string) string {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30
	sum[8] = (sum[8] & 0x3f) | 0x80
	return hex.EncodeToString(sum[:])
}

func sanitizeComponent(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, "..", "_")
	if s == "" {
		return "version"
	}
	return s
}
