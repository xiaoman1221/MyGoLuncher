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
	case "darwin":
		add("official", "官方启动器", filepath.Join(home, "Library", "Application Support", "minecraft"))
		add("hmcl", "HMCL", filepath.Join(home, ".hmcl", ".minecraft"))
		add("prism", "Prism Launcher", filepath.Join(home, "Library", "Application Support", "PrismLauncher"))
		add("multimc", "MultiMC", filepath.Join(home, "Library", "Application Support", "MultiMC"))
	default:
		add("official", "官方启动器", filepath.Join(home, ".minecraft"))
		add("hmcl", "HMCL", filepath.Join(home, ".hmcl", ".minecraft"))
		add("prism", "Prism Launcher", filepath.Join(home, ".local", "share", "PrismLauncher"))
		add("prism-flatpak", "Prism Launcher", filepath.Join(home, ".var", "app", "org.prismlauncher.PrismLauncher", "data", "PrismLauncher"))
		add("multimc", "MultiMC", filepath.Join(home, ".local", "share", "multimc"))
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

// Scan inspects a .minecraft directory, a versions directory, one version
// folder, a specific launcher instance or a Prism/MultiMC instances directory.
func Scan(rawPath string) (ScanResult, error) {
	path, err := filepath.Abs(strings.TrimSpace(rawPath))
	if err != nil {
		return ScanResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ScanResult{}, errors.New("请选择游戏目录、versions 目录或版本文件夹")
	}
	result := ScanResult{Root: path, Source: launcherSource(path)}

	// A Prism/MultiMC instance directly selected by the user.
	if util.Exists(filepath.Join(path, "mmc-pack.json")) {
		candidate, err := prismCandidate(path, path, path, filepath.Base(path))
		if err != nil {
			return ScanResult{}, err
		}
		candidate.Source = "Prism / MultiMC"
		result.Source = "Prism / MultiMC"
		result.Candidates = []Candidate{candidate}
		return result, nil
	}

	// A folder containing Prism/MultiMC instances.
	instancesDir := filepath.Join(path, "instances")
	if entries, err := os.ReadDir(instancesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			instanceDir := filepath.Join(instancesDir, entry.Name())
			if !util.Exists(filepath.Join(instanceDir, "mmc-pack.json")) {
				continue
			}
			candidate, err := prismCandidate(instanceDir, instanceDir, path, entry.Name())
			if err == nil {
				candidate.Source = "Prism / MultiMC"
				result.Candidates = append(result.Candidates, candidate)
			}
		}
		if len(result.Candidates) > 0 {
			result.Source = "Prism / MultiMC"
			sortCandidates(result.Candidates)
			return result, nil
		}
	}

	// A version folder selected directly, e.g. .minecraft/versions/1.21.1.
	if candidate, ok := directVersionCandidate(path); ok {
		result.Source = candidate.Source
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
		return ScanResult{}, errors.New("没有找到 versions 目录。请选择包含 versions/ 的 .minecraft 目录、versions 文件夹、单个版本文件夹或 Prism 版本")
	}

	result.Source = launcherSource(mcRoot)
	return scanVersionDirectory(filepath.Join(mcRoot, "versions"), mcRoot, result)
}

func directVersionCandidate(versionDir string) (Candidate, bool) {
	base := filepath.Base(versionDir)
	jsonPath := filepath.Join(versionDir, base+".json")
	if !util.Exists(jsonPath) {
		return Candidate{}, false
	}
	// A version folder normally lives in <game>/.minecraft/versions/<id>.
	// When it was copied somewhere else, only that folder is used as content.
	gameRoot := versionDir
	if strings.EqualFold(filepath.Base(filepath.Dir(versionDir)), "versions") {
		gameRoot = filepath.Dir(filepath.Dir(versionDir))
	}
	source := launcherSource(gameRoot)
	candidate, err := versionCandidate(versionDir, jsonPath, gameRoot, source)
	if err != nil {
		return Candidate{}, false
	}
	candidate.ID = "version:" + base
	return candidate, true
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
		jsonPath := filepath.Join(versionDir, entry.Name()+".json")
		if !util.Exists(jsonPath) {
			continue
		}
		candidate, err := versionCandidate(versionDir, jsonPath, mcRoot, result.Source)
		if err != nil {
			continue
		}
		rel, relErr := filepath.Rel(result.Root, versionDir)
		if relErr != nil {
			rel = filepath.Base(versionDir)
		}
		candidate.ID = filepath.ToSlash(rel)
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
	id := filepath.Base(versionDir)
	if meta.ID != "" {
		id = meta.ID
	}
	loader, loaderVersion := detectLoader(id, meta.Libraries, meta.MainClass)
	mcVersion := strings.TrimSpace(meta.InheritsFrom)
	if mcVersion == "" {
		mcVersion = inferMCVersion(id, loader, loaderVersion)
	}
	warnings := []string{}
	if mcVersion == "" {
		warnings = append(warnings, "无法从版本档案推断 Minecraft 版本")
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
	for _, name := range []string{"mods", "config", "defaultconfigs", "resourcepacks", "shaderpacks", "saves", "options.txt"} {
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

var copyDirs = []string{"mods", "config", "defaultconfigs", "resourcepacks", "shaderpacks", "saves"}
var copyFiles = []string{"options.txt", "servers.dat", "iris.properties"}

// CopyContent copies the user-facing parts of an instance into dest.
func CopyContent(candidate Candidate, dest string) (CopyStats, error) {
	stats := CopyStats{}
	for _, name := range copyDirs {
		source := sourceFor(candidate, name)
		if source == "" {
			continue
		}
		target := filepath.Join(dest, name)
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
		if err := util.CopyFile(source, filepath.Join(dest, name)); err != nil {
			return stats, err
		}
		stats.Files++
	}
	return stats, nil
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
