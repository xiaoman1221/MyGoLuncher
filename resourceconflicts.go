package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"MyMinecraftGoLuncher/internal/curseforge"
	"MyMinecraftGoLuncher/internal/modfilter"
	"MyMinecraftGoLuncher/internal/modrinth"
	"MyMinecraftGoLuncher/internal/util"
)

// ResourceConflictError is returned before a mod file is moved into the
// instance when it duplicates or declares incompatibility with an installed
// mod. The frontend can ask for confirmation and retry with force=true.
type ResourceConflictError struct {
	Conflicts []modfilter.Conflict
}

func (e *ResourceConflictError) Error() string {
	messages := make([]string, 0, len(e.Conflicts))
	for _, conflict := range e.Conflicts {
		messages = append(messages, conflict.Message)
	}
	return "检测到模组冲突，安装已取消：" + strings.Join(messages, "；") + "。确认风险后可选择继续安装。"
}

func detectModConflicts(instanceDir, candidatePath string, incompatibleFiles, incompatibleIDs []string) []modfilter.Conflict {
	return detectModConflictsExcluding(instanceDir, candidatePath, incompatibleFiles, incompatibleIDs, "")
}

func detectModConflictsExcluding(instanceDir, candidatePath string, incompatibleFiles, incompatibleIDs []string, excludePath string) []modfilter.Conflict {
	metadata, err := modfilter.ReadMetadata(candidatePath)
	if err != nil || strings.TrimSpace(metadata.ID) == "" {
		return nil
	}
	installed := modfilter.ScanInstalled(instanceDir)
	if excludePath != "" {
		filtered := installed[:0]
		for _, mod := range installed {
			if filepath.Clean(mod.Path) == filepath.Clean(excludePath) {
				continue
			}
			filtered = append(filtered, mod)
		}
		installed = filtered
	}
	return modfilter.DetectConflicts(metadata, candidatePath, installed, incompatibleFiles, incompatibleIDs)
}

func modrinthIncompatibilities(version modrinth.Version) ([]string, []string) {
	var files []string
	var ids []string
	for _, dependency := range version.Dependencies {
		if dependency.DependencyType != "incompatible" {
			continue
		}
		if dependency.FileName != "" {
			files = append(files, dependency.FileName)
		}
		if dependency.ProjectID != "" {
			ids = append(ids, dependency.ProjectID)
		}
	}
	return files, ids
}

func curseForgeIncompatibilities(file curseforge.File) []string {
	var ids []string
	for _, dependency := range file.Dependencies {
		if dependency.RelationType == 5 && dependency.ModID > 0 {
			ids = append(ids, fmt.Sprintf("%d", dependency.ModID))
		}
	}
	return ids
}

// installDownloadedFile moves a verified cache download into its final mods or
// resource-pack directory, replacing a file with the same name.
func installDownloadedFile(tempPath, targetDir string) (string, error) {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(targetDir, filepath.Base(tempPath))
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err := os.Rename(tempPath, dest); err == nil {
		return dest, nil
	}
	if err := util.CopyFile(tempPath, dest); err != nil {
		return "", err
	}
	_ = os.Remove(tempPath)
	return dest, nil
}
