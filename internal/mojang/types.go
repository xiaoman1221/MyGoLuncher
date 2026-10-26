package mojang

import (
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Manifest is Mojang's list of every published game version.
type Manifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []VersionEntry `json:"versions"`
}

// FlexTime accepts the RFC3339 timestamps used by Mojang as well as the
// "+0000" form used by Fabric and Quilt loader profiles.
type FlexTime struct {
	time.Time
}

func (t *FlexTime) UnmarshalJSON(data []byte) error {
	raw := strings.TrimSpace(string(data))
	if raw == "" || raw == "null" {
		t.Time = time.Time{}
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		t.Time = time.Time{}
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.000-0700",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("无法解析时间 %q", value)
}

func (t FlexTime) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.Time)
}

func (t FlexTime) IsZero() bool {
	return t.Time.IsZero()
}

func (t FlexTime) After(other FlexTime) bool {
	return t.Time.After(other.Time)
}

// VersionEntry is one manifest row.
type VersionEntry struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	URL             string   `json:"url"`
	Time            FlexTime `json:"time"`
	ReleaseTime     FlexTime `json:"releaseTime"`
	SHA1            string   `json:"sha1"`
	ComplianceLevel int      `json:"complianceLevel"`
}

// Version is the metadata needed to install or launch a version. It is also
// used for loader profiles which inherit from a vanilla version.
type Version struct {
	ID                 string              `json:"id"`
	Type               string              `json:"type"`
	MainClass          string              `json:"mainClass"`
	InheritsFrom       string              `json:"inheritsFrom,omitempty"`
	MinecraftArguments string              `json:"minecraftArguments"`
	MinimumLauncherVer int                 `json:"minimumLauncherVersion"`
	Arguments          Arguments           `json:"arguments"`
	AssetIndex         AssetIndex          `json:"assetIndex"`
	Assets             string              `json:"assets"`
	Downloads          map[string]Download `json:"downloads"`
	Libraries          []Library           `json:"libraries"`
	JavaVersion        *JavaVersion        `json:"javaVersion"`
	Logging            Logging             `json:"logging"`
	ReleaseTime        FlexTime            `json:"releaseTime"`
	Time               FlexTime            `json:"time"`
	ComplianceLevel    int                 `json:"complianceLevel"`
}

// Arguments holds the conditional game and JVM argument lists.
type Arguments struct {
	Game []Argument `json:"game"`
	JVM  []Argument `json:"jvm"`
}

// Argument can be a plain string or a rule-gated string/list.
type Argument struct {
	Rules []Rule
	Value []string
}

// UnmarshalJSON accepts both argument formats used by Mojang.
func (a *Argument) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "\"") {
		var single string
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		a.Value = []string{single}
		return nil
	}
	var obj struct {
		Rules []Rule          `json:"rules"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	a.Rules = obj.Rules
	var single string
	if err := json.Unmarshal(obj.Value, &single); err == nil {
		a.Value = []string{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(obj.Value, &list); err != nil {
		return fmt.Errorf("无法解析参数: %w", err)
	}
	a.Value = list
	return nil
}

// MarshalJSON preserves the compact array form when possible.
func (a Argument) MarshalJSON() ([]byte, error) {
	if len(a.Rules) == 0 && len(a.Value) == 1 {
		return json.Marshal(a.Value[0])
	}
	return json.Marshal(struct {
		Rules []Rule   `json:"rules,omitempty"`
		Value []string `json:"value"`
	}{a.Rules, a.Value})
}

// Rule is Mojang's allow/disallow rule.
type Rule struct {
	Action   string          `json:"action"`
	OS       RuleOS          `json:"os"`
	Features map[string]bool `json:"features"`
}

// RuleOS constrains a rule to an operating system family or architecture.
type RuleOS struct {
	Name    string `json:"name"`
	Arch    string `json:"arch"`
	Version string `json:"version"`
}

// Library is a classpath dependency.
type Library struct {
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	Downloads LibraryDownloads  `json:"downloads"`
	Natives   map[string]string `json:"natives"`
	Extract   *LibraryExtract   `json:"extract"`
	Rules     []Rule            `json:"rules"`
}

// LibraryDownloads contains the artifact and legacy native classifiers.
type LibraryDownloads struct {
	Artifact    *Download           `json:"artifact"`
	Classifiers map[string]Download `json:"classifiers"`
}

// LibraryExtract controls what is excluded while unpacking natives.
type LibraryExtract struct {
	Exclude []string `json:"exclude"`
}

// Download describes a downloadable artifact.
type Download struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	URL    string `json:"url"`
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512"`
	Size   int64  `json:"size"`
}

// AssetIndex points to the asset catalog.
type AssetIndex struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	TotalSize int64  `json:"totalSize"`
	URL       string `json:"url"`
}

// AssetCatalog is the asset index payload. map_to_resources is historically
// either a boolean or an object, so it is kept raw.
type AssetCatalog struct {
	Objects        map[string]AssetObject `json:"objects"`
	MapToResources json.RawMessage        `json:"map_to_resources"`
	Virtual        bool                   `json:"virtual"`
}

// MapsToResources reports whether assets must be copied into the game's
// resources directory (Minecraft 1.5 and earlier).
func (c AssetCatalog) MapsToResources() bool {
	raw := strings.TrimSpace(string(c.MapToResources))
	return raw != "" && raw != "false" && raw != "null" && raw != "{}"
}

// AssetObject is one hashed asset.
type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

// JavaVersion is Mojang's runtime declaration.
type JavaVersion struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

// Logging configures the log4j file.
type Logging struct {
	Client *LoggingClient `json:"client"`
}

// LoggingClient is a downloadable logging configuration.
type LoggingClient struct {
	Argument string   `json:"argument"`
	File     Download `json:"file"`
	Type     string   `json:"type"`
}

// OSName returns Mojang's name for the host OS.
func OSName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "osx"
	default:
		return "linux"
	}
}

// OSArch returns Mojang's architecture token.
func OSArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "386":
		return "x86"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

// RulesAllow evaluates all rules for the current machine. Features that are
// not explicitly provided evaluate to false, matching the official launcher.
func RulesAllow(rules []Rule, features map[string]bool) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, rule := range rules {
		if !ruleMatches(rule, features) {
			continue
		}
		if strings.EqualFold(rule.Action, "allow") {
			allowed = true
		} else if strings.EqualFold(rule.Action, "disallow") {
			allowed = false
		}
	}
	return allowed
}

func ruleMatches(rule Rule, features map[string]bool) bool {
	if rule.OS.Name != "" && rule.OS.Name != OSName() {
		return false
	}
	if rule.OS.Arch != "" && rule.OS.Arch != OSArch() {
		return false
	}
	if rule.OS.Version != "" {
		re, err := regexp.Compile(rule.OS.Version)
		if err == nil && !re.MatchString(hostOSVersion()) {
			return false
		}
	}
	for name, want := range rule.Features {
		if features[name] != want {
			return false
		}
	}
	return true
}

func hostOSVersion() string {
	// Mojang's OS version rules are Windows-specific and rarely used. Returning
	// a modern Windows marker keeps those rules permissive.
	if runtime.GOOS == "windows" {
		return "10.0"
	}
	return ""
}

// ArtifactPath converts a Maven coordinate to a repository path.
func ArtifactPath(name string) (string, error) {
	ext := ".jar"
	if at := strings.LastIndex(name, "@"); at >= 0 {
		ext = "." + name[at+1:]
		name = name[:at]
	}
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return "", fmt.Errorf("无效的 Maven 坐标: %s", name)
	}
	group, artifact, version := parts[0], parts[1], parts[2]
	classifier := ""
	if len(parts) >= 4 {
		classifier = parts[3]
	}
	file := artifact + "-" + version
	if classifier != "" {
		file += "-" + classifier
	}
	file += ext
	return strings.ReplaceAll(group, ".", "/") + "/" + artifact + "/" + version + "/" + file, nil
}

// ClassifierFromName extracts the optional classifier from a Maven coordinate.
func ClassifierFromName(name string) string {
	base := name
	if at := strings.LastIndex(base, "@"); at >= 0 {
		base = base[:at]
	}
	parts := strings.Split(base, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return ""
}

// IsNativeLibrary reports whether a library artifact is a native bundle.
func IsNativeLibrary(lib Library) bool {
	if len(lib.Natives) > 0 {
		return true
	}
	classifier := ClassifierFromName(lib.Name)
	return strings.HasPrefix(classifier, "natives-")
}

// NativeClassifier returns the platform classifier for a legacy style lib.
func NativeClassifier(lib Library) string {
	if len(lib.Natives) == 0 {
		return ""
	}
	classifier := lib.Natives[OSName()]
	if classifier == "" {
		return ""
	}
	arch := "32"
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		arch = "64"
	}
	return strings.ReplaceAll(classifier, "${arch}", arch)
}

// NativeArtifact picks the platform-specific download for a native library.
func NativeArtifact(lib Library) *Download {
	if classifier := NativeClassifier(lib); classifier != "" {
		if dl, ok := lib.Downloads.Classifiers[classifier]; ok {
			return &dl
		}
	}
	if lib.Downloads.Artifact != nil && IsNativeLibrary(lib) {
		return lib.Downloads.Artifact
	}
	return nil
}

// LibraryDownload resolves the artifact download for a library, filling in
// Maven repository metadata used by loader profiles.
func LibraryDownload(lib Library) (*Download, error) {
	if lib.Downloads.Artifact != nil {
		dl := *lib.Downloads.Artifact
		if dl.Path == "" {
			path, err := ArtifactPath(lib.Name)
			if err != nil {
				return nil, err
			}
			dl.Path = path
		}
		return &dl, nil
	}
	path, err := ArtifactPath(lib.Name)
	if err != nil {
		return nil, err
	}
	base := strings.TrimSpace(lib.URL)
	if base == "" {
		base = "https://repo1.maven.org/maven2/"
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return &Download{Path: path, URL: base + path}, nil
}

// Merge combines a version profile with its inheritance chain. Child values
// win; libraries preserve the loader's priority by coming first.
func Merge(versions []Version) Version {
	if len(versions) == 0 {
		return Version{}
	}
	merged := versions[0]
	for _, parent := range versions[1:] {
		if merged.MainClass == "" {
			merged.MainClass = parent.MainClass
		}
		if merged.AssetIndex.ID == "" {
			merged.AssetIndex = parent.AssetIndex
		}
		if merged.Assets == "" {
			merged.Assets = parent.Assets
		}
		if merged.InheritsFrom == "" && parent.InheritsFrom != "" {
			merged.InheritsFrom = parent.InheritsFrom
		}
		if merged.JavaVersion == nil {
			merged.JavaVersion = parent.JavaVersion
		}
		if merged.Logging.Client == nil {
			merged.Logging = parent.Logging
		}
		if merged.MinecraftArguments == "" {
			merged.MinecraftArguments = parent.MinecraftArguments
		}
		if merged.ReleaseTime.IsZero() {
			merged.ReleaseTime = parent.ReleaseTime
		}
		if merged.Type == "" {
			merged.Type = parent.Type
		}
		if merged.MinimumLauncherVer == 0 {
			merged.MinimumLauncherVer = parent.MinimumLauncherVer
		}
		merged.Downloads = mergeDownloads(merged.Downloads, parent.Downloads)
		merged.Libraries = append(merged.Libraries, parent.Libraries...)
		merged.Arguments.Game = append(parent.Arguments.Game, merged.Arguments.Game...)
		merged.Arguments.JVM = append(parent.Arguments.JVM, merged.Arguments.JVM...)
	}
	// Deduplicate libraries by Maven identity. The first (youngest child) wins.
	seen := map[string]bool{}
	libs := make([]Library, 0, len(merged.Libraries))
	for _, lib := range merged.Libraries {
		key := lib.Name
		if key == "" {
			key = lib.URL
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		libs = append(libs, lib)
	}
	merged.Libraries = libs
	return merged
}

func mergeDownloads(child, parent map[string]Download) map[string]Download {
	out := map[string]Download{}
	for k, v := range parent {
		out[k] = v
	}
	for k, v := range child {
		out[k] = v
	}
	return out
}

// JavaMajor returns the required Java feature version, with a fallback for
// old profiles that do not declare javaVersion.
func (v Version) JavaMajor() int {
	if v.JavaVersion != nil && v.JavaVersion.MajorVersion > 0 {
		return v.JavaVersion.MajorVersion
	}
	id := v.ID
	if v.InheritsFrom != "" {
		id = v.InheritsFrom
	}
	return FallbackJavaMajor(id)
}

// FallbackJavaMajor maps game versions to their Java requirement when the
// metadata omits it.
func FallbackJavaMajor(id string) int {
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "1.") {
		parts := strings.Split(id, ".")
		if len(parts) >= 2 {
			minor, _ := parseLeadingInt(parts[1])
			patch := 0
			if len(parts) >= 3 {
				patch, _ = parseLeadingInt(parts[2])
			}
			switch {
			case minor >= 21:
				return 21
			case minor == 20 && patch >= 5:
				return 21
			case minor >= 18:
				return 17
			case minor == 17:
				return 16
			default:
				return 8
			}
		}
	}
	// New calendar-style versions (26.x+) all target modern runtimes.
	return 25
}

func parseLeadingInt(s string) (int, error) {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, fmt.Errorf("not a number")
	}
	n := 0
	for _, c := range s[:end] {
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// SortVersions puts newest release entries first.
func SortVersions(versions []VersionEntry) {
	sort.SliceStable(versions, func(i, j int) bool {
		return versions[i].ReleaseTime.After(versions[j].ReleaseTime)
	})
}

// NativeDirectorySuffix inspects JVM arguments for the modern layout where
// LWJGL natives live under a `java` subdirectory.
func NativeDirectorySuffix(arguments []Argument) string {
	for _, arg := range arguments {
		for _, value := range arg.Value {
			if strings.Contains(value, "${natives_directory}/java") {
				return "java"
			}
		}
	}
	return ""
}
