package external

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"MyMinecraftGoLuncher/internal/util"
)

// Candidate is a game instance discovered inside another launcher.
type Candidate struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	MCVersion     string   `json:"mcVersion"`
	Loader        string   `json:"loader"`
	LoaderVersion string   `json:"loaderVersion"`
	Source        string   `json:"source"`
	ModCount      int      `json:"modCount"`
	HasSaves      bool     `json:"hasSaves"`
	Warnings      []string `json:"warnings"`
	ContentDir    string   `json:"contentDir,omitempty"`

	versionDir string
	contentDir string
	gameRoot   string
}

// ScanResult describes one scan of a launcher directory.
type ScanResult struct {
	Root       string      `json:"root"`
	Source     string      `json:"source"`
	Candidates []Candidate `json:"candidates"`
}

// KnownRoot is a well-known launcher data directory on this machine.
type KnownRoot struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

// KnownRoots lists common launcher directories. Directories that do not exist
// are still returned with Exists=false so the UI can explain what was checked.
func KnownRoots() []KnownRoot {
	home, _ := os.UserHomeDir()
	appData := strings.TrimSpace(os.Getenv("APPDATA"))
	configDir, _ := os.UserConfigDir()

	var raw []KnownRoot
	add := func(id, label, path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		raw = append(raw, KnownRoot{ID: id, Label: label, Path: filepath.Clean(path)})
	}

	switch runtime.GOOS {
	case "windows":
		add("official", "官方启动器", filepath.Join(appData, ".minecraft"))
		add("hmcl", "HMCL", filepath.Join(appData, ".hmcl"))
		add("hmcl-data", "HMCL", filepath.Join(home, ".hmcl", ".minecraft"))
		add("pcl2", "PCL2", filepath.Join(appData, "PCL"))
		add("prism", "Prism Launcher", filepath.Join(appData, "PrismLauncher"))
		add("multimc", "MultiMC", filepath.Join(appData, "MultiMC"))
		add("modrinth", "Modrinth App", filepath.Join(appData, "com.modrinth.theseus", "profiles"))
		add("curseforge", "CurseForge", filepath.Join(appData, "curseforge", "Instances"))
		add("curseforge-minecraft", "CurseForge", filepath.Join(appData, "curseforge", "minecraft", "Instances"))
		add("atlauncher", "ATLauncher", filepath.Join(appData, "ATLauncher", "instances"))
		add("gdlauncher", "GDLauncher", filepath.Join(appData, "gdlauncher_next", "instances"))
		add("gdlauncher-legacy", "GDLauncher", filepath.Join(appData, "gdlauncher", "instances"))
		add("ftb", "FTB App", filepath.Join(appData, "ftbapp", "instances"))
		add("ftb-legacy", "FTB App", filepath.Join(appData, "ftb", "instances"))
	case "darwin":
		add("official", "官方启动器", filepath.Join(home, "Library", "Application Support", "minecraft"))
		add("hmcl", "HMCL", filepath.Join(home, ".hmcl", ".minecraft"))
		add("prism", "Prism Launcher", filepath.Join(home, "Library", "Application Support", "PrismLauncher"))
		add("multimc", "MultiMC", filepath.Join(home, "Library", "Application Support", "MultiMC"))
		add("modrinth", "Modrinth App", filepath.Join(home, "Library", "Application Support", "com.modrinth.theseus", "profiles"))
		add("curseforge", "CurseForge", filepath.Join(home, "Library", "Application Support", "curseforge", "Instances"))
		add("curseforge-minecraft", "CurseForge", filepath.Join(home, "Library", "Application Support", "curseforge", "minecraft", "Instances"))
		add("atlauncher", "ATLauncher", filepath.Join(home, "Library", "Application Support", "ATLauncher", "instances"))
		add("gdlauncher", "GDLauncher", filepath.Join(home, "Library", "Application Support", "gdlauncher_next", "instances"))
		add("ftb", "FTB App", filepath.Join(home, "Library", "Application Support", "ftbapp", "instances"))
	default:
		add("official", "官方启动器", filepath.Join(home, ".minecraft"))
		add("hmcl", "HMCL", filepath.Join(home, ".hmcl", ".minecraft"))
		add("prism", "Prism Launcher", filepath.Join(home, ".local", "share", "PrismLauncher"))
		add("prism-flatpak", "Prism Launcher", filepath.Join(home, ".var", "app", "org.prismlauncher.PrismLauncher", "data", "PrismLauncher"))
		add("multimc", "MultiMC", filepath.Join(home, ".local", "share", "multimc"))
		add("modrinth", "Modrinth App", filepath.Join(home, ".local", "share", "com.modrinth.theseus", "profiles"))
		add("modrinth-config", "Modrinth App", filepath.Join(configDir, "com.modrinth.theseus", "profiles"))
		add("curseforge", "CurseForge", filepath.Join(home, ".local", "share", "curseforge", "Instances"))
		add("curseforge-minecraft", "CurseForge", filepath.Join(home, ".local", "share", "curseforge", "minecraft", "Instances"))
		add("curseforge-config", "CurseForge", filepath.Join(configDir, "curseforge", "Instances"))
		add("atlauncher", "ATLauncher", filepath.Join(home, ".local", "share", "ATLauncher", "instances"))
		add("gdlauncher", "GDLauncher", filepath.Join(home, ".local", "share", "gdlauncher_next", "instances"))
		add("ftb", "FTB App", filepath.Join(home, ".local", "share", "ftbapp", "instances"))
	}
	if configDir != "" {
		// Some launchers keep data next to the per-user configuration folder.
		add("prism-config", "Prism Launcher", filepath.Join(configDir, "PrismLauncher"))
		add("multimc-config", "MultiMC", filepath.Join(configDir, "MultiMC"))
	}

	seen := map[string]bool{}
	out := make([]KnownRoot, 0, len(raw))
	for _, root := range raw {
		key := strings.ToLower(root.Path)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		if info, err := os.Stat(root.Path); err == nil && info.IsDir() {
			root.Exists = true
		}
		out = append(out, root)
	}
	return out
}

// OfficialMinecraftDir reports the official launcher's default data directory.
func OfficialMinecraftDir() string {
	for _, root := range KnownRoots() {
		if root.ID == "official" {
			return root.Path
		}
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func childDirFold(parent, name string) string {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return ""
	}
	fallback := ""
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() == name {
			return filepath.Join(parent, entry.Name())
		}
		if fallback == "" && strings.EqualFold(entry.Name(), name) {
			fallback = filepath.Join(parent, entry.Name())
		}
	}
	return fallback
}

// samePath compares cleaned paths case-insensitively on Windows.
func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

type versionLibrary struct {
	Name string `json:"name"`
}

type versionMetadata struct {
	ID           string           `json:"id"`
	InheritsFrom string           `json:"inheritsFrom"`
	MainClass    string           `json:"mainClass"`
	Libraries    []versionLibrary `json:"libraries"`
}

type mmcPack struct {
	Components []struct {
		UID     string `json:"uid"`
		Version string `json:"version"`
	} `json:"components"`
}

// Scan inspects .minecraft/versions folders, launcher instance directories and
// the common third-party launcher data layouts. It deliberately accepts several
// metadata formats so moving a version between launchers does not require a
// manual conversion step.
func Scan(rawPath string) (ScanResult, error) {
	path, err := filepath.Abs(strings.TrimSpace(rawPath))
	if err != nil {
		return ScanResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ScanResult{}, errors.New("请选择游戏目录、versions 目录、实例目录或版本文件夹")
	}
	result := ScanResult{Root: path, Source: launcherSource(path)}

	// The selected folder itself is a launcher instance.
	if candidate, ok := instanceCandidate(path, path, filepath.Base(path)); ok {
		if candidate.Source == "其他启动器" && result.Source != "其他启动器" {
			candidate.Source = result.Source
		}
		candidate.ID = "instance:" + filepath.Base(path)
		candidate.ContentDir = candidate.contentDir
		result.Source = candidate.Source
		result.Candidates = []Candidate{candidate}
		return result, nil
	}

	// The user selected the instances/profiles container itself. Treat its
	// direct children as instances instead of looking for a nested container.
	if base := strings.ToLower(filepath.Base(path)); base == "instances" || base == "profiles" {
		gameRoot := path
		if base == "instances" {
			gameRoot = filepath.Dir(path)
		}
		if entries, readErr := os.ReadDir(path); readErr == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				instanceDir := filepath.Join(path, entry.Name())
				candidate, ok := instanceCandidate(instanceDir, gameRoot, entry.Name())
				if !ok {
					continue
				}
				if candidate.Source == "其他启动器" && result.Source != "其他启动器" {
					candidate.Source = result.Source
				}
				candidate.ID = "instance:" + filepath.ToSlash(entry.Name())
				candidate.ContentDir = candidate.contentDir
				result.Candidates = append(result.Candidates, candidate)
			}
			if len(result.Candidates) > 0 {
				if result.Source == "其他启动器" {
					result.Source = result.Candidates[0].Source
				}
				sortCandidates(result.Candidates)
				return result, nil
			}
		}
	}

	// A folder containing Prism/MultiMC, Modrinth, CurseForge, ATLauncher,
	// GDLauncher or FTB instances.
	for _, container := range []string{"instances", "profiles"} {
		instancesDir := childDirFold(path, container)
		if instancesDir == "" {
			continue
		}
		entries, readErr := os.ReadDir(instancesDir)
		if readErr != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			instanceDir := filepath.Join(instancesDir, entry.Name())
			candidate, ok := instanceCandidate(instanceDir, path, entry.Name())
			if !ok {
				continue
			}
			if candidate.Source == "其他启动器" && result.Source != "其他启动器" {
				candidate.Source = result.Source
			}
			rel, relErr := filepath.Rel(result.Root, instanceDir)
			if relErr != nil {
				rel = filepath.Base(instanceDir)
			}
			candidate.ID = "instance:" + filepath.ToSlash(rel)
			candidate.ContentDir = candidate.contentDir
			result.Candidates = append(result.Candidates, candidate)
		}
		if len(result.Candidates) > 0 {
			if result.Source == "其他启动器" {
				result.Source = result.Candidates[0].Source
			}
			sortCandidates(result.Candidates)
			return result, nil
		}
	}

	// A version folder selected directly, e.g. .minecraft/versions/1.21.1.
	if candidate, ok := directVersionCandidate(path); ok {
		result.Source = candidate.Source
		candidate.ContentDir = candidate.contentDir
		result.Candidates = []Candidate{candidate}
		return result, nil
	}

	// A versions directory selected directly.
	if strings.EqualFold(filepath.Base(path), "versions") && dirExists(path) {
		result.Source = launcherSource(filepath.Dir(path))
		return scanVersionDirectory(path, filepath.Dir(path), result)
	}

	mcRoot := ""
	if _, err := os.Stat(filepath.Join(path, "versions")); err == nil {
		mcRoot = path
	} else if _, err := os.Stat(filepath.Join(path, ".minecraft", "versions")); err == nil {
		mcRoot = filepath.Join(path, ".minecraft")
	}
	if mcRoot == "" {
		return ScanResult{}, errors.New("没有找到可导入的版本或实例。请选择包含 versions/ 的 .minecraft 目录、launcher instances 目录、单个版本文件夹或整合包实例")
	}

	result.Source = launcherSource(mcRoot)
	return scanVersionDirectory(filepath.Join(mcRoot, "versions"), mcRoot, result)
}

func directVersionCandidate(versionDir string) (Candidate, bool) {
	base := filepath.Base(versionDir)
	// A version folder normally lives in <game>/.minecraft/versions/<id>.
	// When it was copied somewhere else, only that folder is used as content.
	gameRoot := versionDir
	if strings.EqualFold(filepath.Base(filepath.Dir(versionDir)), "versions") {
		gameRoot = filepath.Dir(filepath.Dir(versionDir))
	}
	source := launcherSource(gameRoot)
	candidate, ok := loadVersionCandidate(versionDir, gameRoot, source, "version:"+base)
	if !ok {
		return Candidate{}, false
	}
	return candidate, true
}

func loadVersionCandidate(versionDir, gameRoot, source, id string) (Candidate, bool) {
	if jsonPath, ok := findVersionJSON(versionDir); ok {
		candidate, err := versionCandidate(versionDir, jsonPath, gameRoot, source)
		if err == nil {
			candidate.ID = id
			return candidate, true
		}
	}
	candidate, ok := jarOnlyCandidate(versionDir, gameRoot, source)
	if !ok {
		return Candidate{}, false
	}
	candidate.ID = id
	return candidate, true
}

func findVersionJSON(versionDir string) (string, bool) {
	base := filepath.Base(versionDir)
	preferred := filepath.Join(versionDir, base+".json")
	if util.Exists(preferred) {
		return preferred, true
	}
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		candidate := filepath.Join(versionDir, entry.Name())
		var meta versionMetadata
		if util.ReadJSON(candidate, &meta) == nil && (meta.MainClass != "" || meta.InheritsFrom != "" || meta.ID != "") {
			return candidate, true
		}
	}
	return "", false
}

func findVersionJar(versionDir string) string {
	base := filepath.Base(versionDir)
	preferred := filepath.Join(versionDir, base+".jar")
	if util.Exists(preferred) {
		return preferred
	}
	entries, err := os.ReadDir(versionDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			return filepath.Join(versionDir, entry.Name())
		}
	}
	return ""
}

func jarOnlyCandidate(versionDir, gameRoot, source string) (Candidate, bool) {
	if findVersionJar(versionDir) == "" {
		return Candidate{}, false
	}
	name := filepath.Base(versionDir)
	mcVersion := extractMCVersion(name)
	loader, loaderVersion := parseLoaderText(name)
	warnings := []string{"该版本缺少版本 JSON，已根据文件夹名称推断版本信息；如不正确请使用自定义导入"}
	contentDir := versionDir
	if !hasContent(versionDir) {
		contentDir = gameRoot
	}
	return Candidate{
		Name: name, MCVersion: mcVersion, Loader: loader, LoaderVersion: loaderVersion,
		Source: source, ModCount: countMods(contentDir),
		HasSaves: dirExists(filepath.Join(contentDir, "saves")), Warnings: warnings,
		versionDir: versionDir, contentDir: contentDir, gameRoot: gameRoot,
	}, true
}

var minecraftVersionPattern = regexp.MustCompile(`(?i)(1\.[0-9]+(?:\.[0-9]+)?|[0-9]{2}w[0-9]{2}[a-z]|[ab][0-9]+\.[0-9]+(?:\.[0-9]+)?|2[6-9]\.[0-9]+(?:\.[0-9]+)?)`)

func extractMCVersion(value string) string {
	for _, indexes := range minecraftVersionPattern.FindAllStringIndex(value, -1) {
		start, end := indexes[0], indexes[1]
		if start > 0 && isVersionBoundaryChar(value[start-1]) {
			continue
		}
		if end < len(value) && isVersionBoundaryChar(value[end]) {
			continue
		}
		return value[start:end]
	}
	return ""
}

func isVersionBoundaryChar(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') || char == '.'
}

func normalizeLoaderName(value string) string {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch {
	case strings.Contains(lower, "neoforge"):
		return "neoforge"
	case strings.Contains(lower, "quilt"):
		return "quilt"
	case strings.Contains(lower, "fabric"):
		return "fabric"
	case strings.Contains(lower, "forge"):
		return "forge"
	case lower == "":
		return ""
	default:
		return "vanilla"
	}
}

func loaderVersionFromText(value, loader string) string {
	lower := strings.ToLower(value)
	if loader == "" {
		return ""
	}
	index := strings.Index(lower, loader)
	if index < 0 {
		return ""
	}
	tail := value[index+len(loader):]
	re := regexp.MustCompile(`[0-9]+\.[0-9]+(?:\.[0-9]+)?`)
	matches := re.FindAllString(tail, -1)
	if len(matches) == 0 {
		return ""
	}
	if loader == "forge" || loader == "neoforge" {
		return matches[len(matches)-1]
	}
	return matches[0]
}

// instanceCandidate detects the common instance metadata formats used by
// Prism/MultiMC, Modrinth App, CurseForge, ATLauncher, GDLauncher and FTB.
func instanceCandidate(instanceDir, gameRoot, fallbackName string) (Candidate, bool) {
	if util.Exists(filepath.Join(instanceDir, "mmc-pack.json")) {
		candidate, err := prismCandidate(instanceDir, instanceDir, gameRoot, fallbackName)
		if err == nil {
			candidate.Source = "Prism / MultiMC"
			return candidate, true
		}
	}
	if util.Exists(filepath.Join(instanceDir, "profile.json")) {
		if candidate, ok := modrinthCandidate(instanceDir, gameRoot, fallbackName); ok {
			return candidate, true
		}
	}
	if util.Exists(filepath.Join(instanceDir, "minecraftinstance.json")) {
		if candidate, ok := curseForgeCandidate(instanceDir, gameRoot, fallbackName); ok {
			return candidate, true
		}
	}
	if util.Exists(filepath.Join(instanceDir, "instance.json")) {
		if candidate, ok := genericInstanceCandidate(instanceDir, gameRoot, fallbackName); ok {
			return candidate, true
		}
	}
	return Candidate{}, false
}

func modrinthCandidate(instanceDir, gameRoot, fallbackName string) (Candidate, bool) {
	var meta map[string]any
	if err := util.ReadJSON(filepath.Join(instanceDir, "profile.json"), &meta); err != nil {
		return Candidate{}, false
	}
	name := firstMapString(meta, "name", "profile_name", "displayName")
	if strings.TrimSpace(name) == "" {
		name = fallbackName
	}
	mcVersion := firstMapString(meta, "game_version", "minecraft_version", "mcVersion", "gameVersion")
	loader := normalizeLoaderName(firstMapString(meta, "loader", "mod_loader", "modLoader"))
	loaderVersion := firstMapString(meta, "loader_version", "loaderVersion", "modLoaderVersion")
	if loader == "" {
		loader, loaderVersion = parseLoaderText(firstMapString(meta, "loader", "mod_loader") + " " + name)
	}
	if mcVersion == "" {
		mcVersion = extractMCVersion(name)
	}
	warnings := []string{}
	if mcVersion == "" {
		warnings = append(warnings, "Modrinth 实例未声明 Minecraft 版本，请使用自定义导入补充")
	}
	return newInstanceCandidate(instanceDir, gameRoot, name, mcVersion, loader, loaderVersion, "Modrinth App", warnings), true
}

func curseForgeCandidate(instanceDir, gameRoot, fallbackName string) (Candidate, bool) {
	var meta map[string]any
	if err := util.ReadJSON(filepath.Join(instanceDir, "minecraftinstance.json"), &meta); err != nil {
		return Candidate{}, false
	}
	name := firstMapString(meta, "name", "instanceName", "displayName")
	if strings.TrimSpace(name) == "" {
		name = fallbackName
	}
	mcVersion := firstMapString(meta, "gameVersion", "minecraftVersion", "mcVersion")
	base, _ := meta["baseModLoader"].(map[string]any)
	loader := ""
	loaderVersion := ""
	if base != nil {
		loaderText := firstMapString(base, "name", "loader", "type")
		loader, loaderVersion = parseLoaderText(loaderText)
		if loaderVersion == "" {
			loaderVersion = firstMapString(base, "forgeVersion", "fabricVersion", "quiltVersion", "neoForgeVersion")
		}
	}
	if loader == "" {
		loader, loaderVersion = parseLoaderText(name)
	}
	warnings := []string{}
	if mcVersion == "" {
		warnings = append(warnings, "CurseForge 实例未声明 Minecraft 版本，请使用自定义导入补充")
	}
	return newInstanceCandidate(instanceDir, gameRoot, name, mcVersion, loader, loaderVersion, "CurseForge", warnings), true
}

func genericInstanceCandidate(instanceDir, gameRoot, fallbackName string) (Candidate, bool) {
	var meta map[string]any
	if err := util.ReadJSON(filepath.Join(instanceDir, "instance.json"), &meta); err != nil {
		return Candidate{}, false
	}
	name := firstMapString(meta, "name", "instanceName", "displayName", "title")
	if strings.TrimSpace(name) == "" {
		name = fallbackName
	}
	mcVersion := firstMapString(meta, "minecraftVersion", "gameVersion", "mcVersion", "minecraft", "version")
	if nested, ok := meta["minecraft"].(map[string]any); ok {
		mcVersion = firstNonEmptyString(mcVersion, firstMapString(nested, "version", "minecraftVersion"))
	}
	loaderText := firstMapString(meta, "loader", "modLoader", "modLoaderType")
	loaderVersion := firstMapString(meta, "loaderVersion", "modLoaderVersion", "forgeVersion", "fabricVersion", "quiltVersion", "neoForgeVersion")
	if nested, ok := meta["loader"].(map[string]any); ok {
		loaderText = firstMapString(nested, "type", "name", "loader")
		if loaderVersion == "" {
			loaderVersion = firstMapString(nested, "version")
		}
	}
	if nested, ok := meta["modLoader"].(map[string]any); ok {
		loaderText = firstMapString(nested, "type", "name", "loader")
		if loaderVersion == "" {
			loaderVersion = firstMapString(nested, "version")
		}
	}
	loader, parsedVersion := parseLoaderText(loaderText)
	if loaderVersion == "" {
		loaderVersion = parsedVersion
	}
	if loader == "" {
		loader, loaderVersion = parseLoaderText(name)
	}
	if loaderVersion == "" {
		loaderVersion = firstMapString(meta, "forgeVersion", "fabricVersion", "quiltVersion", "neoForgeVersion")
	}
	if mcVersion == "" {
		mcVersion = extractMCVersion(name)
	}
	warnings := []string{}
	if mcVersion == "" {
		warnings = append(warnings, "实例未声明 Minecraft 版本，请使用自定义导入补充")
	}
	return newInstanceCandidate(instanceDir, gameRoot, name, mcVersion, loader, loaderVersion, "其他启动器", warnings), true
}

func newInstanceCandidate(instanceDir, gameRoot, name, mcVersion, loader, loaderVersion, source string, warnings []string) Candidate {
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(instanceDir)
	}
	if loader == "" {
		loader = "vanilla"
	}
	if loader != "vanilla" && loaderVersion == "" {
		warnings = append(warnings, "未识别到加载器版本，导入时会自动选择最新版本")
	}
	contentDir := instanceContentDir(instanceDir)
	return Candidate{
		Name: name, MCVersion: strings.TrimSpace(mcVersion),
		Loader: loader, LoaderVersion: strings.TrimSpace(loaderVersion),
		Source: source, ModCount: countMods(contentDir),
		HasSaves: dirExists(filepath.Join(contentDir, "saves")), Warnings: warnings,
		versionDir: instanceDir, contentDir: contentDir, gameRoot: gameRoot,
	}
}

func instanceContentDir(instanceDir string) string {
	for _, name := range []string{"minecraft", ".minecraft"} {
		nested := filepath.Join(instanceDir, name)
		if dirExists(nested) && hasContent(nested) {
			return nested
		}
	}
	if hasContent(instanceDir) {
		return instanceDir
	}
	return instanceDir
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstMapString(value map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := value[key]; ok {
			if text, ok := raw.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func parseLoaderText(value string) (string, string) {
	lower := strings.ToLower(value)
	loader := ""
	switch {
	case strings.Contains(lower, "neoforge"):
		loader = "neoforge"
	case strings.Contains(lower, "quilt"):
		loader = "quilt"
	case strings.Contains(lower, "fabric"):
		loader = "fabric"
	case strings.Contains(lower, "forge"):
		loader = "forge"
	}
	if loader == "" {
		return "", ""
	}
	return loader, loaderVersionFromText(value, loader)
}

func scanVersionDirectory(versionsDir, mcRoot string, result ScanResult) (ScanResult, error) {
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return ScanResult{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		versionDir := filepath.Join(versionsDir, entry.Name())
		rel, relErr := filepath.Rel(result.Root, versionDir)
		if relErr != nil {
			rel = filepath.Base(versionDir)
		}
		candidate, ok := loadVersionCandidate(versionDir, mcRoot, result.Source, filepath.ToSlash(rel))
		if !ok {
			continue
		}
		candidate.ContentDir = candidate.contentDir
		result.Candidates = append(result.Candidates, candidate)
	}
	if len(result.Candidates) == 0 {
		return ScanResult{}, errors.New("没有在该目录中发现可导入的 Minecraft 版本")
	}
	sortCandidates(result.Candidates)
	return result, nil
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].MCVersion != candidates[j].MCVersion {
			return candidates[i].MCVersion > candidates[j].MCVersion
		}
		return candidates[i].Name < candidates[j].Name
	})
}

func prismCandidate(instanceDir, versionDir, gameRoot, name string) (Candidate, error) {
	var pack mmcPack
	if err := util.ReadJSON(filepath.Join(instanceDir, "mmc-pack.json"), &pack); err != nil {
		return Candidate{}, err
	}
	mcVersion := ""
	loader := "vanilla"
	loaderVersion := ""
	for _, comp := range pack.Components {
		switch comp.UID {
		case "net.minecraft":
			mcVersion = comp.Version
		case "net.fabricmc.fabric-loader":
			loader, loaderVersion = "fabric", comp.Version
		case "org.quiltmc.quilt-loader":
			loader, loaderVersion = "quilt", comp.Version
		case "net.minecraftforge":
			loader, loaderVersion = "forge", comp.Version
		case "net.neoforged":
			loader, loaderVersion = "neoforge", comp.Version
		}
	}
	contentDir := filepath.Join(instanceDir, "minecraft")
	if _, err := os.Stat(contentDir); err != nil {
		contentDir = filepath.Join(instanceDir, ".minecraft")
	}
	if _, err := os.Stat(contentDir); err != nil {
		contentDir = instanceDir
	}
	if iniName := readInstanceName(filepath.Join(instanceDir, "instance.cfg")); iniName != "" {
		name = iniName
	}
	modCount := countMods(contentDir)
	hasSaves := dirExists(filepath.Join(contentDir, "saves"))
	if mcVersion == "" {
		return Candidate{}, errors.New("版本缺少 Minecraft 版本")
	}
	return Candidate{
		ID: filepath.ToSlash(filepath.Join(filepath.Base(instanceDir), name)), Name: name,
		MCVersion: mcVersion, Loader: loader, LoaderVersion: loaderVersion,
		Source: "Prism / MultiMC", ModCount: modCount, HasSaves: hasSaves,
		versionDir: versionDir, contentDir: contentDir, gameRoot: gameRoot,
	}, nil
}

func versionCandidate(versionDir, jsonPath, gameRoot, source string) (Candidate, error) {
	var meta versionMetadata
	if err := util.ReadJSON(jsonPath, &meta); err != nil {
		return Candidate{}, err
	}
	folderName := filepath.Base(versionDir)
	id := folderName
	if strings.TrimSpace(meta.ID) != "" {
		id = strings.TrimSpace(meta.ID)
	}
	loader, loaderVersion := detectLoader(id, meta.Libraries, meta.MainClass)
	if loader == "vanilla" {
		if fallbackLoader, fallbackVersion := parseLoaderText(folderName); fallbackLoader != "" {
			loader, loaderVersion = fallbackLoader, fallbackVersion
		}
	}
	mcVersion := strings.TrimSpace(meta.InheritsFrom)
	if mcVersion == "" {
		mcVersion = inferMCVersion(id, loader, loaderVersion)
	}
	if mcVersion == "" {
		mcVersion = inferMCVersion(folderName, loader, loaderVersion)
	}
	if mcVersion == "" {
		mcVersion = extractMCVersion(id)
	}
	if mcVersion == "" {
		mcVersion = extractMCVersion(folderName)
	}
	if loaderVersion == "" && loader != "vanilla" {
		if _, parsed := parseLoaderText(id + " " + folderName); parsed != "" {
			loaderVersion = parsed
		}
	}
	warnings := []string{}
	if mcVersion == "" {
		warnings = append(warnings, "无法从版本档案推断 Minecraft 版本，请使用自定义导入补充")
	}
	if loader != "vanilla" && loaderVersion == "" {
		warnings = append(warnings, "未识别到加载器版本，导入时会自动选择最新版本")
	}
	if loader == "vanilla" && countMods(versionDir) > 0 {
		warnings = append(warnings, "版本档案未声明模组加载器，但版本目录中存在模组")
	}
	if loader == "vanilla" && meta.MainClass != "" && !strings.Contains(strings.ToLower(meta.MainClass), "net.minecraft.client.main.main") {
		warnings = append(warnings, "该版本使用自定义启动主类（可能包含 OptiFine 等组件），导入后可能需要手动补充运行库")
	}
	contentDir := versionDir
	if !hasContent(versionDir) {
		contentDir = gameRoot
	}
	return Candidate{
		Name: id, MCVersion: mcVersion, Loader: loader, LoaderVersion: loaderVersion,
		Source: source, ModCount: countMods(contentDir),
		HasSaves: dirExists(filepath.Join(contentDir, "saves")), Warnings: warnings,
		versionDir: versionDir, contentDir: contentDir, gameRoot: gameRoot,
	}, nil
}

func detectLoader(id string, libraries []versionLibrary, mainClass string) (string, string) {
	lowerID := strings.ToLower(id)
	lowerMain := strings.ToLower(mainClass)
	loader := "vanilla"
	version := ""
	for _, lib := range libraries {
		lower := strings.ToLower(lib.Name)
		switch {
		case strings.Contains(lower, "net.fabricmc:fabric-loader:"):
			loader = "fabric"
			version = mavenVersion(lib.Name)
		case strings.Contains(lower, "org.quiltmc:quilt-loader:"):
			loader = "quilt"
			version = mavenVersion(lib.Name)
		case strings.Contains(lower, "net.neoforged:neoforge:"):
			loader = "neoforge"
			version = mavenVersion(lib.Name)
		case strings.Contains(lower, "net.neoforged:forge:"):
			loader = "neoforge"
			version = mavenVersion(lib.Name)
		case strings.Contains(lower, "net.minecraftforge:forge:"):
			version = mavenVersion(lib.Name)
			if !strings.Contains(lowerID, "neoforge") {
				loader = "forge"
			}
		}
	}
	if loader == "vanilla" {
		switch {
		case strings.Contains(lowerID, "neoforge"):
			loader = "neoforge"
		case strings.Contains(lowerID, "fabric"):
			loader = "fabric"
		case strings.Contains(lowerID, "quilt"):
			loader = "quilt"
		case strings.Contains(lowerID, "forge"):
			loader = "forge"
		case strings.Contains(lowerMain, "fabricmc"):
			loader = "fabric"
		case strings.Contains(lowerMain, "quiltmc"):
			loader = "quilt"
		case strings.Contains(lowerMain, "minecraftforge") || strings.Contains(lowerMain, "neoforged"):
			loader = "forge"
		}
	}
	if version == "" {
		switch loader {
		case "fabric":
			version = matchOrEmpty(`fabric-loader-([^-]+)-`, id)
		case "quilt":
			version = matchOrEmpty(`quilt-loader-([^-]+)-`, id)
		case "forge":
			version = matchOrEmpty(`(?i)-forge-?(.+)$`, id)
		case "neoforge":
			version = matchOrEmpty(`(?i)neoforge-(.+)$`, id)
		}
	}
	if (loader == "forge" || loader == "neoforge") && version != "" {
		parts := strings.SplitN(version, "-", 2)
		if len(parts) == 2 && (strings.HasPrefix(parts[0], "1.") || regexp.MustCompile(`^\d+\.\d+$`).MatchString(parts[0])) {
			version = parts[1]
		}
	}
	return loader, version
}

func inferMCVersion(id, loader, loaderVersion string) string {
	lower := strings.ToLower(id)
	if loaderVersion != "" {
		lower = strings.ReplaceAll(lower, strings.ToLower(loaderVersion), "")
	}
	if idx := strings.Index(lower, "-forge"); idx > 0 {
		return id[:idx]
	}
	if loader == "fabric" || loader == "quilt" {
		parts := strings.Split(id, "-")
		if len(parts) >= 4 {
			return parts[len(parts)-1]
		}
	}
	if loader == "neoforge" {
		return ""
	}
	if loader == "vanilla" {
		// Official vanilla profiles use the Minecraft version as their ID,
		// including snapshots, pre-releases and old alpha/beta builds.
		return strings.TrimSpace(id)
	}
	normalized := strings.TrimSpace(id)
	if matched, _ := regexp.MatchString(`^(1\.[0-9]+(\.[0-9]+)?|[0-9]+\.[0-9]+(\.[0-9]+)?([-.].*)?|a[0-9].*|b[0-9].*|[0-9]{2}w[0-9]{2}[a-z])$`, normalized); matched {
		return normalized
	}
	return ""
}

func hasContent(dir string) bool {
	for _, name := range []string{"mods", "config", "defaultconfigs", "resourcepacks", "shaderpacks", "saves", "kubejs", "scripts", "datapacks", "options.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func countMods(dir string) int {
	entries, err := os.ReadDir(filepath.Join(dir, "mods"))
	if err != nil {
		return 0
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".jar") {
			count++
		}
	}
	return count
}

func readInstanceName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "name="))
		}
	}
	return ""
}

func mavenVersion(coordinate string) string {
	parts := strings.Split(coordinate, ":")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func matchOrEmpty(pattern, value string) string {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ""
	}
	match := re.FindStringSubmatch(value)
	if len(match) >= 2 {
		return match[1]
	}
	return ""
}

func detectLauncherName(path string) string {
	if util.Exists(filepath.Join(path, "hmcl.json")) {
		return "HMCL"
	}
	if util.Exists(filepath.Join(path, "profile.json")) || strings.Contains(strings.ToLower(path), "modrinth") {
		return "Modrinth App"
	}
	if util.Exists(filepath.Join(path, "minecraftinstance.json")) || strings.Contains(strings.ToLower(path), "curseforge") {
		return "CurseForge"
	}
	if util.Exists(filepath.Join(path, "mmc-pack.json")) || util.Exists(filepath.Join(path, "instance.cfg")) {
		return "Prism / MultiMC"
	}
	if strings.Contains(strings.ToLower(path), "atlauncher") {
		return "ATLauncher"
	}
	if strings.Contains(strings.ToLower(path), "gdlauncher") {
		return "GDLauncher"
	}
	if strings.Contains(strings.ToLower(path), "ftbapp") {
		return "FTB App"
	}
	entries, err := os.ReadDir(path)
	if err == nil {
		for _, entry := range entries {
			name := strings.ToLower(entry.Name())
			if strings.Contains(name, "pcl") && !strings.Contains(name, "logs") {
				return "PCL2"
			}
			if strings.Contains(name, "hmcl") {
				return "HMCL"
			}
			if strings.Contains(name, "prismlauncher") {
				return "Prism Launcher"
			}
			if strings.Contains(name, "multimc") {
				return "MultiMC"
			}
		}
	}
	return "其他启动器"
}

// launcherSource names a detected game directory. It walks a few ancestors so
// that paths such as %APPDATA%/.hmcl/.minecraft are recognised as well.
func launcherSource(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return "其他启动器"
	}
	official := OfficialMinecraftDir()
	current := filepath.Clean(dir)
	for i := 0; i < 4; i++ {
		if official != "" && samePath(current, official) {
			return "官方启动器"
		}
		if source := detectLauncherName(current); source != "其他启动器" {
			return source
		}
		switch base := strings.ToLower(filepath.Base(current)); {
		case strings.Contains(base, "hmcl"):
			return "HMCL"
		case strings.Contains(base, "pcl"):
			return "PCL2"
		case strings.Contains(base, "prismlauncher"):
			return "Prism Launcher"
		case strings.Contains(base, "multimc"):
			return "MultiMC"
		case strings.Contains(base, "com.modrinth.theseus"), strings.Contains(base, "modrinth"):
			return "Modrinth App"
		case strings.Contains(base, "curseforge"):
			return "CurseForge"
		case strings.Contains(base, "atlauncher"):
			return "ATLauncher"
		case strings.Contains(base, "gdlauncher"):
			return "GDLauncher"
		case strings.Contains(base, "ftbapp"):
			return "FTB App"
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	return "其他启动器"
}

// CopyStats summarises imported content.
type CopyStats struct {
	Files         int `json:"files"`
	Mods          int `json:"mods"`
	ResourcePacks int `json:"resourcePacks"`
	ShaderPacks   int `json:"shaderPacks"`
	Saves         int `json:"saves"`
}

var copyDirs = []string{"mods", "config", "defaultconfigs", "resourcepacks", "shaderpacks", "saves", "kubejs", "scripts", "datapacks"}
var copyFiles = []string{"options.txt", "optionsof.txt", "servers.dat", "iris.properties"}

// CopyContent copies the user-facing parts of an instance into dest.
func CopyContent(candidate Candidate, dest string) (CopyStats, error) {
	stats := CopyStats{}
	for _, name := range copyDirs {
		source := sourceFor(candidate, name)
		if source == "" {
			continue
		}
		target := filepath.Join(dest, name)
		if samePath(source, target) {
			continue
		}
		if err := util.CopyDirTree(source, target); err != nil {
			return stats, fmt.Errorf("复制 %s 失败: %w", name, err)
		}
		files, err := countFiles(target)
		if err != nil {
			return stats, err
		}
		stats.Files += files
		switch name {
		case "mods":
			stats.Mods = countMods(dest)
		case "resourcepacks":
			stats.ResourcePacks = countEntries(target)
		case "shaderpacks":
			stats.ShaderPacks = countEntries(target)
		case "saves":
			stats.Saves = countEntries(target)
		}
	}
	for _, name := range copyFiles {
		source := sourceFor(candidate, name)
		if source == "" {
			continue
		}
		target := filepath.Join(dest, name)
		if samePath(source, target) {
			continue
		}
		if err := util.CopyFile(source, target); err != nil {
			return stats, err
		}
		stats.Files++
	}
	return stats, nil
}

// CopyContentFrom copies user-facing content from an arbitrary game or
// instance directory. It accepts a .minecraft root, a Prism-style instance
// folder containing .minecraft, or a version folder under versions/.
func CopyContentFrom(sourceDir, dest string) (CopyStats, error) {
	stats := CopyStats{}
	root := customContentRoot(sourceDir)
	if root == "" {
		return stats, errors.New("来源目录不存在")
	}
	for _, name := range copyDirs {
		source := filepath.Join(root, name)
		if stat, err := os.Stat(source); err != nil || !stat.IsDir() {
			continue
		}
		target := filepath.Join(dest, name)
		if samePath(source, target) {
			continue
		}
		if err := util.CopyDirTree(source, target); err != nil {
			return stats, fmt.Errorf("复制 %s 失败: %w", name, err)
		}
		files, err := countFiles(target)
		if err != nil {
			return stats, err
		}
		stats.Files += files
		switch name {
		case "mods":
			stats.Mods = countMods(dest)
		case "resourcepacks":
			stats.ResourcePacks = countEntries(target)
		case "shaderpacks":
			stats.ShaderPacks = countEntries(target)
		case "saves":
			stats.Saves = countEntries(target)
		}
	}
	for _, name := range copyFiles {
		source := filepath.Join(root, name)
		if stat, err := os.Stat(source); err != nil || stat.IsDir() {
			continue
		}
		target := filepath.Join(dest, name)
		if samePath(source, target) {
			continue
		}
		if err := util.CopyFile(source, target); err != nil {
			return stats, err
		}
		stats.Files++
	}
	return stats, nil
}

func customContentRoot(sourceDir string) string {
	sourceDir = filepath.Clean(strings.TrimSpace(sourceDir))
	if stat, err := os.Stat(sourceDir); err != nil || !stat.IsDir() {
		return ""
	}
	// A versions folder itself lives directly under the game root.
	if strings.EqualFold(filepath.Base(sourceDir), "versions") {
		return filepath.Dir(sourceDir)
	}
	// Prism / MultiMC instances usually keep content under .minecraft.
	nested := filepath.Join(sourceDir, ".minecraft")
	if stat, err := os.Stat(nested); err == nil && stat.IsDir() {
		return nested
	}
	// A version folder normally shares content with its parent game root, but
	// some custom packs keep mods/configs inside the version folder itself.
	parent := filepath.Dir(sourceDir)
	if strings.EqualFold(filepath.Base(parent), "versions") {
		for _, name := range copyDirs {
			if stat, err := os.Stat(filepath.Join(sourceDir, name)); err == nil && stat.IsDir() {
				return sourceDir
			}
		}
		return filepath.Dir(parent)
	}
	return sourceDir
}

func sourceFor(candidate Candidate, name string) string {
	if candidate.contentDir != "" {
		path := filepath.Join(candidate.contentDir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if candidate.gameRoot != "" && candidate.gameRoot != candidate.contentDir {
		path := filepath.Join(candidate.gameRoot, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func countFiles(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Type().IsRegular() {
			count++
		}
		return nil
	})
	return count, err
}

func countEntries(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".") {
			n++
		}
	}
	return n
}

// FindCandidate re-scans root and returns a candidate by ID.
func FindCandidate(root, id string) (Candidate, error) {
	result, err := Scan(root)
	if err != nil {
		return Candidate{}, err
	}
	for _, candidate := range result.Candidates {
		if candidate.ID == id {
			return candidate, nil
		}
	}
	return Candidate{}, errors.New("所选版本已不存在，请重新扫描")
}
