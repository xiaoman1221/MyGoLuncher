package modfilter

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Metadata is the small set of jar fields needed for duplicate/incompatibility
// checks. It intentionally does not try to model every loader feature.
type Metadata struct {
	ID          string
	Name        string
	Version     string
	Environment string
	Provides    []string
	Breaks      []string
	Source      string
}

// Installed describes one jar found in an instance mods directory.
type Installed struct {
	Path     string
	FileName string
	Metadata Metadata
}

// Conflict is a duplicate or declared incompatibility found before installing a
// new mod jar.
type Conflict struct {
	Kind      string
	ModID     string
	Existing  string
	Candidate string
	Message   string
}

// ReadMetadata reads common Fabric, Quilt, Forge, NeoForge and legacy mcmod
// metadata from a mod jar.
func ReadMetadata(jarPath string) (Metadata, error) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return Metadata{}, err
	}
	defer reader.Close()
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[strings.ToLower(filepathToSlash(file.Name))] = file
	}

	if file := files["fabric.mod.json"]; file != nil {
		var raw map[string]any
		if readJSON(file, &raw) == nil {
			id := mapString(raw, "id")
			if id != "" {
				return Metadata{
					ID: id, Name: mapString(raw, "name"), Version: mapString(raw, "version"),
					Environment: mapString(raw, "environment"),
					Provides:    anyIDs(raw["provides"]), Breaks: anyIDs(raw["breaks"]), Source: "fabric",
				}, nil
			}
		}
	}

	if file := files["quilt.mod.json"]; file != nil {
		var raw map[string]any
		if readJSON(file, &raw) == nil {
			if loader, ok := raw["quilt_loader"].(map[string]any); ok {
				id := mapString(loader, "id")
				if id != "" {
					name := ""
					if metadata, ok := loader["metadata"].(map[string]any); ok {
						name = mapString(metadata, "name")
					}
					return Metadata{
						ID: id, Name: name, Version: mapString(loader, "version"),
						Provides: anyIDs(loader["provides"]), Breaks: anyIDs(loader["breaks"]), Source: "quilt",
					}, nil
				}
			}
		}
	}

	for _, name := range []string{"meta-inf/mods.toml", "meta-inf/neoforge.mods.toml"} {
		if file := files[name]; file != nil {
			if meta, ok := parseForgeMetadata(string(readBytes(file)), name); ok {
				return meta, nil
			}
		}
	}

	if file := files["mcmod.info"]; file != nil {
		if meta, ok := parseMCMODInfo(readBytes(file)); ok {
			return meta, nil
		}
	}
	return Metadata{}, errors.New("未找到可识别的模组元数据")
}

func parseForgeMetadata(text, source string) (Metadata, bool) {
	meta := Metadata{Source: strings.TrimPrefix(source, "meta-inf/")}
	inDependency := false
	depID := ""
	depType := ""
	flushDependency := func() {
		if inDependency && strings.EqualFold(depType, "incompatible") && depID != "" {
			meta.Breaks = appendUnique(meta.Breaks, depID)
		}
		depID = ""
		depType = ""
		inDependency = false
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(stripTOMLComment(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[[dependencies.") {
			flushDependency()
			inDependency = true
			continue
		}
		if strings.HasPrefix(trimmed, "[[") {
			flushDependency()
			continue
		}
		key, value, ok := tomlPair(trimmed)
		if !ok {
			continue
		}
		if inDependency {
			switch key {
			case "modId":
				depID = value
			case "type":
				depType = value
			}
			continue
		}
		switch key {
		case "modId":
			if meta.ID == "" {
				meta.ID = value
			}
		case "displayName":
			if meta.Name == "" {
				meta.Name = value
			}
		case "version":
			if meta.Version == "" {
				meta.Version = value
			}
		}
	}
	flushDependency()
	if meta.ID == "" {
		return Metadata{}, false
	}
	if meta.Name == "" {
		meta.Name = meta.ID
	}
	return meta, true
}

func stripTOMLComment(line string) string {
	quoted := false
	for index, char := range line {
		if char == '"' {
			quoted = !quoted
		}
		if char == '#' && !quoted {
			return line[:index]
		}
	}
	return line
}

func tomlPair(line string) (string, string, bool) {
	index := strings.Index(line, "=")
	if index < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:index])
	value := strings.TrimSpace(line[index+1:])
	if key == "" || value == "" {
		return "", "", false
	}
	if len(value) >= 2 &&
		((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	return key, strings.TrimSpace(value), true
}

func parseMCMODInfo(data []byte) (Metadata, bool) {
	var list []map[string]any
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return metadataFromLegacy(list[0])
	}
	var single map[string]any
	if err := json.Unmarshal(data, &single); err == nil {
		return metadataFromLegacy(single)
	}
	return Metadata{}, false
}

func metadataFromLegacy(raw map[string]any) (Metadata, bool) {
	id := mapString(raw, "modid", "modId", "id")
	if id == "" {
		return Metadata{}, false
	}
	meta := Metadata{ID: id, Name: mapString(raw, "name"), Version: mapString(raw, "version"), Source: "mcmod.info"}
	if value, ok := raw["clientSideOnly"].(bool); ok && value {
		meta.Environment = "client"
	}
	return meta, true
}

func mapString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func anyIDs(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return uniqueStrings(out)
	case []string:
		return uniqueStrings(typed)
	case map[string]any:
		out := make([]string, 0, len(typed))
		for key := range typed {
			out = append(out, key)
		}
		sort.Strings(out)
		return out
	case string:
		return uniqueStrings([]string{typed})
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[strings.ToLower(value)] {
			continue
		}
		seen[strings.ToLower(value)] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.EqualFold(existing, value) {
			return values
		}
	}
	return append(values, value)
}

// ScanInstalled reads metadata from every jar in instanceDir/mods. Individual
// unreadable jars are ignored so one broken mod does not disable the checker.
func ScanInstalled(instanceDir string) []Installed {
	modsDir := filepath.Join(instanceDir, "mods")
	var out []Installed
	_ = filepath.WalkDir(modsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!entry.Type().IsRegular() || !IsModJar(path) {
			return nil
		}
		meta, metaErr := ReadMetadata(path)
		if metaErr != nil || strings.TrimSpace(meta.ID) == "" {
			return nil
		}
		out = append(out, Installed{Path: path, FileName: entry.Name(), Metadata: meta})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	return out
}

// DetectConflicts compares a candidate jar against installed mods. The same
// file name is treated as an update; a second file providing the same mod ID is
// treated as a duplicate. Declared breaks and incompatible dependency names/IDs
// are also checked.
func DetectConflicts(candidate Metadata, candidateFile string, installed []Installed, incompatibleFiles, incompatibleIDs []string) []Conflict {
	var conflicts []Conflict
	if strings.TrimSpace(candidate.ID) == "" {
		return conflicts
	}
	candidateName := filepath.Base(candidateFile)
	for _, mod := range installed {
		if strings.EqualFold(mod.FileName, candidateName) {
			continue
		}
		if strings.EqualFold(mod.Metadata.ID, candidate.ID) {
			conflicts = append(conflicts, Conflict{
				Kind: "duplicate", ModID: candidate.ID, Existing: mod.FileName,
				Message: "已安装同一模组的不同文件（" + mod.FileName + "），继续安装可能产生重复模组",
			})
			continue
		}
		if containsID(candidate.Breaks, mod.Metadata.ID) || containsID(mod.Metadata.Breaks, candidate.ID) {
			conflicts = append(conflicts, Conflict{
				Kind: "incompatible", ModID: candidate.ID, Existing: mod.FileName,
				Message: "与已安装模组 " + firstN(mod.Metadata.ID, mod.FileName) + " 声明了不兼容关系",
			})
			continue
		}
		if containsID(incompatibleIDs, mod.Metadata.ID) {
			conflicts = append(conflicts, Conflict{
				Kind: "incompatible", ModID: mod.Metadata.ID, Existing: mod.FileName,
				Message: "该版本声明与已安装模组 " + firstN(mod.Metadata.ID, mod.FileName) + " 不兼容",
			})
			continue
		}
		for _, file := range incompatibleFiles {
			if strings.EqualFold(filepath.Base(file), mod.FileName) {
				conflicts = append(conflicts, Conflict{
					Kind: "incompatible", ModID: mod.Metadata.ID, Existing: mod.FileName,
					Message: "该版本声明与已安装文件 " + mod.FileName + " 不兼容",
				})
			}
		}
	}
	return conflicts
}

func containsID(values []string, id string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(id)) {
			return true
		}
	}
	return false
}

func firstN(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "未知模组"
}
