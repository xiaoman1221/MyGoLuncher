package java

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/sysutil"
)

// Runtime is a discovered Java installation.
type Runtime struct {
	Path         string `json:"path"`
	Home         string `json:"home"`
	Version      string `json:"version"`
	Major        int    `json:"major"`
	Vendor       string `json:"vendor"`
	Architecture string `json:"architecture"`
	Managed      bool   `json:"managed"`
	Source       string `json:"source"`
}

// Label is a short UI string.
func (r Runtime) Label() string {
	return fmt.Sprintf("Java %d · %s", r.Major, r.Vendor)
}

// Manager discovers and installs Java runtimes.
type Manager struct {
	paths store.Paths
}

// NewManager creates a Java manager rooted at the launcher data directory.
func NewManager(paths store.Paths) *Manager {
	return &Manager{paths: paths}
}

// Probe inspects a Java executable or home directory.
func Probe(ctx context.Context, path string) (Runtime, error) {
	exe, home, err := resolveExecutable(path)
	if err != nil {
		return Runtime{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-XshowSettings:properties", "-version")
	sysutil.HideWindow(cmd)
	out, runErr := cmd.CombinedOutput()
	text := string(out)
	if ctx.Err() != nil {
		return Runtime{}, errors.New("检测 Java 超时")
	}
	rt := Runtime{Path: exe, Home: home, Source: "system"}
	props := parseProperties(text)
	rt.Version = props["java.version"]
	rt.Vendor = props["java.vendor"]
	rt.Architecture = props["os.arch"]
	if rt.Version == "" {
		rt.Version = parseVersionOutput(text)
	}
	if rt.Version == "" {
		if runErr != nil {
			return Runtime{}, fmt.Errorf("无法运行 Java: %w", runErr)
		}
		return Runtime{}, errors.New("无法读取 Java 版本")
	}
	rt.Major = MajorFromVersion(rt.Version)
	if rt.Major == 0 {
		return Runtime{}, fmt.Errorf("无法识别 Java 版本: %s", rt.Version)
	}
	if rt.Vendor == "" {
		rt.Vendor = "Java"
	}
	rt.Vendor = simplifyVendor(rt.Vendor)
	if props["java.home"] != "" {
		rt.Home = props["java.home"]
	}
	return rt, nil
}

func resolveExecutable(path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", errors.New("Java 路径为空")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("Java 不存在: %w", err)
	}
	if info.IsDir() {
		exe := filepath.Join(path, "bin", javaExeName())
		if _, err := os.Stat(exe); err != nil {
			// macOS bundles point at Contents/Home.
			exe = filepath.Join(path, "Contents", "Home", "bin", javaExeName())
			if _, err := os.Stat(exe); err != nil {
				return "", "", errors.New("目录中没有找到 Java 可执行文件")
			}
			return exe, filepath.Dir(filepath.Dir(exe)), nil
		}
		return exe, path, nil
	}
	home := filepath.Dir(filepath.Dir(path))
	return path, home, nil
}

func javaExeName() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}

func parseProperties(out string) map[string]string {
	props := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Property settings:") {
			continue
		}
		if idx := strings.Index(line, " = "); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+3:])
			if strings.HasPrefix(key, "java.") || key == "os.arch" {
				props[key] = value
			}
		}
	}
	return props
}

func parseVersionOutput(out string) string {
	// Example: openjdk version "21.0.2" 2024-01-16
	if idx := strings.Index(out, "version \""); idx >= 0 {
		rest := out[idx+len("version \""):]
		if end := strings.Index(rest, "\""); end > 0 {
			return rest[:end]
		}
	}
	return ""
}

// MajorFromVersion converts 1.8.0_511 and 21.0.2 to their feature version.
func MajorFromVersion(version string) int {
	version = strings.TrimSpace(strings.TrimPrefix(version, "v"))
	version = strings.TrimPrefix(version, "1.")
	parts := strings.FieldsFunc(version, func(r rune) bool {
		return r == '.' || r == '_' || r == '-' || r == '+'
	})
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}
	return n
}

func simplifyVendor(vendor string) string {
	vendor = strings.TrimSpace(vendor)
	switch {
	case strings.Contains(vendor, "Temurin"), strings.Contains(vendor, "Adoptium"), strings.Contains(vendor, "Eclipse"):
		return "Temurin"
	case strings.Contains(vendor, "Oracle"):
		return "Oracle"
	case strings.Contains(vendor, "Microsoft"):
		return "Microsoft"
	case strings.Contains(vendor, "Azul"):
		return "Azul"
	case strings.Contains(vendor, "Amazon"):
		return "Corretto"
	case strings.Contains(vendor, "BellSoft"), strings.Contains(vendor, "Liberica"):
		return "Liberica"
	case strings.Contains(vendor, "OpenJDK"):
		return "OpenJDK"
	case vendor == "":
		return "Java"
	default:
		if len(vendor) > 24 {
			return vendor[:24]
		}
		return vendor
	}
}

// Scan finds managed runtimes, JAVA_HOME, PATH entries and common installs.
func (m *Manager) Scan(ctx context.Context, extraPaths []string) []Runtime {
	candidates := []string{}
	for _, p := range extraPaths {
		if strings.TrimSpace(p) != "" {
			candidates = append(candidates, p)
		}
	}
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		candidates = append(candidates, jh)
	}
	candidates = append(candidates, m.managedJavaExecutables()...)
	if p, err := exec.LookPath("java"); err == nil {
		candidates = append(candidates, p)
	}
	candidates = append(candidates, commonJavaPaths()...)

	seen := map[string]bool{}
	var runtimes []Runtime
	for _, candidate := range candidates {
		key := canonicalPath(candidate)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		rt, err := Probe(ctx, candidate)
		if err != nil {
			continue
		}
		rt.Managed = isWithin(m.paths.Runtimes, rt.Path)
		if rt.Managed {
			rt.Source = "managed"
		} else if containsPath(extraPaths, candidate) {
			rt.Source = "custom"
		}
		runtimes = append(runtimes, rt)
	}
	sort.SliceStable(runtimes, func(i, j int) bool {
		if runtimes[i].Major != runtimes[j].Major {
			return runtimes[i].Major > runtimes[j].Major
		}
		return runtimes[i].Path < runtimes[j].Path
	})
	return runtimes
}

// Managed returns only launcher-installed runtimes.
func (m *Manager) Managed(ctx context.Context) []Runtime {
	var out []Runtime
	for _, exe := range m.managedJavaExecutables() {
		if rt, err := Probe(ctx, exe); err == nil {
			rt.Managed = true
			rt.Source = "managed"
			out = append(out, rt)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Major > out[j].Major })
	return out
}

func (m *Manager) managedJavaExecutables() []string {
	var out []string
	_ = filepath.WalkDir(m.paths.Runtimes, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel(m.paths.Runtimes, path)
			if rel != "." && strings.Count(rel, string(filepath.Separator)) > 3 {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if name == "java" || name == "java.exe" {
			out = append(out, path)
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// FindSuitable chooses an installed runtime. An instance-level preferred path
// is treated as an explicit override and must be compatible. Otherwise an
// exact match wins, with a newer runtime allowed where Minecraft supports it.
func (m *Manager) FindSuitable(ctx context.Context, major int, preferred string, extraPaths []string) (Runtime, error) {
	if strings.TrimSpace(preferred) != "" {
		rt, err := Probe(ctx, preferred)
		if err != nil {
			return Runtime{}, fmt.Errorf("自定义 Java 不可用: %w", err)
		}
		if !Compatible(rt.Major, major) {
			return Runtime{}, fmt.Errorf("指定的 Java %d 不能运行需要 Java %d 的版本", rt.Major, major)
		}
		return rt, nil
	}
	runtimes := m.Scan(ctx, extraPaths)
	for _, rt := range runtimes {
		if rt.Major == major {
			return rt, nil
		}
	}
	for _, rt := range runtimes {
		if Compatible(rt.Major, major) {
			return rt, nil
		}
	}
	if len(runtimes) == 0 {
		return Runtime{}, fmt.Errorf("未找到 Java，请在 Java 环境页安装 Java %d", major)
	}
	return Runtime{}, fmt.Errorf("没有兼容 Java %d 的运行时，请在 Java 环境页安装", major)
}

// Compatible reports whether a Java feature version can run a game that
// declares the required major. Java 8 is strict; newer releases usually work
// with a newer runtime, which is how the vanilla launcher behaves as well.
func Compatible(actual, required int) bool {
	if actual == required {
		return true
	}
	return required >= 16 && actual > required
}

func commonJavaPaths() []string {
	var globs []string
	exe := "java"
	if runtime.GOOS == "windows" {
		exe = "java.exe"
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			base := os.Getenv(env)
			if base == "" {
				continue
			}
			globs = append(globs,
				filepath.Join(base, "Java", "*", "bin", exe),
				filepath.Join(base, "Eclipse Adoptium", "*", "bin", exe),
				filepath.Join(base, "Microsoft", "jdk-*", "bin", exe),
				filepath.Join(base, "Zulu", "*", "bin", exe),
				filepath.Join(base, "Amazon Corretto", "*", "bin", exe),
				filepath.Join(base, "BellSoft", "LibericaJDK-*", "bin", exe),
				filepath.Join(base, "Programs", "Eclipse Adoptium", "*", "bin", exe),
			)
		}
	} else if runtime.GOOS == "darwin" {
		globs = append(globs,
			"/Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java",
			filepath.Join(homeDir(), "Library/Java/JavaVirtualMachines/*/Contents/Home/bin/java"),
			"/opt/homebrew/opt/openjdk*/bin/java",
			"/usr/local/opt/openjdk*/bin/java",
		)
	} else {
		globs = append(globs, "/usr/lib/jvm/*/bin/java", "/usr/java/*/bin/java")
	}
	var out []string
	for _, pattern := range globs {
		matches, _ := filepath.Glob(pattern)
		out = append(out, matches...)
	}
	return out
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func canonicalPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	return strings.ToLower(filepath.Clean(abs))
}

func isWithin(base, path string) bool {
	baseAbs, err1 := filepath.Abs(base)
	pathAbs, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(baseAbs, pathAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func containsPath(paths []string, target string) bool {
	key := canonicalPath(target)
	for _, p := range paths {
		if canonicalPath(p) == key {
			return true
		}
	}
	return false
}
