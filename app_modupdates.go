package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/curseforge"
	"MyMinecraftGoLuncher/internal/modfilter"
	"MyMinecraftGoLuncher/internal/modrinth"
)

// ModUpdateFile is one installed mod jar and, when it could be identified, its
// available update.
type ModUpdateFile struct {
	FileName        string `json:"fileName"`
	RelativePath    string `json:"relativePath"`
	Name            string `json:"name"`
	CurrentVersion  string `json:"currentVersion"`
	Source          string `json:"source,omitempty"`
	ProjectID       string `json:"projectId,omitempty"`
	VersionID       string `json:"versionId,omitempty"`
	LatestVersion   string `json:"latestVersion,omitempty"`
	LatestName      string `json:"latestName,omitempty"`
	FileSize        int64  `json:"fileSize"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Identified      bool   `json:"identified"`
	Error           string `json:"error,omitempty"`
}

// ModUpdateReport summarizes one update check.
type ModUpdateReport struct {
	InstanceID   string          `json:"instanceId"`
	Files        []ModUpdateFile `json:"files"`
	Identified   int             `json:"identified"`
	Updates      int             `json:"updates"`
	Unidentified int             `json:"unidentified"`
	Warnings     []string        `json:"warnings"`
}

type installedModJar struct {
	path      string
	relative  string
	fileName  string
	size      int64
	meta      modfilter.Metadata
	sha1      string
	fp        uint32
	cfFile    curseforge.File
	cfProject int64
}

// CheckModUpdates identifies mods by Modrinth SHA-1 and CurseForge fingerprints
// and resolves the newest compatible file for each project.
func (a *App) CheckModUpdates(instanceID string) (ModUpdateReport, error) {
	if err := a.ready(); err != nil {
		return ModUpdateReport{}, err
	}
	instance, ok := a.store.Instance(instanceID)
	if !ok {
		return ModUpdateReport{}, errors.New("版本不存在")
	}
	report := ModUpdateReport{InstanceID: instanceID, Files: []ModUpdateFile{}, Warnings: []string{}}
	instanceDir := a.instanceDir(instance)
	jars, err := scanModJars(instanceDir)
	if err != nil {
		return report, err
	}
	if len(jars) == 0 {
		return report, nil
	}
	ctx, cancel := context.WithTimeout(a.context(), 3*time.Minute)
	defer cancel()

	var hashes []string
	fingerprints := make([]uint32, 0, len(jars))
	for index := range jars {
		if jars[index].sha1 != "" {
			hashes = append(hashes, strings.ToLower(jars[index].sha1))
		}
		fingerprints = append(fingerprints, jars[index].fp)
	}

	modrinthVersions := map[string]modrinth.Version{}
	for start := 0; start < len(hashes); start += 100 {
		end := minInt(start+100, len(hashes))
		found, apiErr := modrinth.VersionsByHashes(ctx, hashes[start:end], "sha1")
		if apiErr != nil {
			report.Warnings = append(report.Warnings, "Modrinth 更新查询失败："+apiErr.Error())
			break
		}
		for key, version := range found {
			modrinthVersions[strings.ToLower(key)] = version
		}
	}

	settings := a.store.Settings()
	var cfClient *curseforge.Client
	if strings.TrimSpace(settings.CurseForgeAPIKey) == "" {
		report.Warnings = append(report.Warnings, "未配置 CurseForge API Key，仅使用 Modrinth 哈希识别；可在设置中填写 Key 以识别 CurseForge 模组。")
	}
	if strings.TrimSpace(settings.CurseForgeAPIKey) != "" {
		cfClient = curseforge.New(settings.CurseForgeAPIKey)
		var matches []curseforge.FingerprintMatch
		for start := 0; start < len(fingerprints); start += 100 {
			end := minInt(start+100, len(fingerprints))
			found, apiErr := cfClient.FingerprintMatches(ctx, fingerprints[start:end])
			if apiErr != nil {
				report.Warnings = append(report.Warnings, "CurseForge 指纹查询失败："+apiErr.Error())
				break
			}
			matches = append(matches, found...)
		}
		// The fingerprint endpoint does not guarantee the order of its matches,
		// so map returned files back by SHA-1 first and exact file name second.
		for _, match := range matches {
			matchSHA1 := ""
			for _, hash := range match.File.Hashes {
				if hash.Algo == 1 {
					matchSHA1 = strings.ToLower(hash.Value)
					break
				}
			}
			projectID := match.File.ModID
			if projectID == 0 {
				projectID = match.ID
			}
			for index := range jars {
				sameHash := matchSHA1 != "" && strings.EqualFold(matchSHA1, jars[index].sha1)
				sameName := match.File.FileName != "" && strings.EqualFold(match.File.FileName, jars[index].fileName)
				if sameHash || (matchSHA1 == "" && sameName) {
					jars[index].cfFile = match.File
					jars[index].cfProject = projectID
				}
			}
		}
	}

	latestModrinth := map[string]modrinth.Version{}
	latestCurseForge := map[int64]curseforge.File{}
	modrinthLoader := instance.Loader
	if normalizeLoader(modrinthLoader) == "vanilla" {
		modrinthLoader = ""
	}
	for index := range jars {
		jar := &jars[index]
		entry := ModUpdateFile{
			FileName: jar.fileName, RelativePath: jar.relative, FileSize: jar.size,
			Name:           firstString(jar.meta.Name, jar.meta.ID, strings.TrimSuffix(jar.fileName, filepath.Ext(jar.fileName))),
			CurrentVersion: firstString(jar.meta.Version, strings.TrimSuffix(jar.fileName, filepath.Ext(jar.fileName))),
		}
		if version, ok := modrinthVersions[strings.ToLower(jar.sha1)]; ok {
			entry.Source = sourceModrinth
			entry.ProjectID = version.ProjectID
			entry.VersionID = version.ID
			if version.VersionNumber != "" {
				entry.CurrentVersion = version.VersionNumber
			}
			if version.Name != "" {
				entry.Name = firstString(jar.meta.Name, version.Name)
			}
			entry.Identified = true
			latest, cached := latestModrinth[version.ProjectID]
			var lookupErr error
			if !cached {
				latest, lookupErr = modrinth.BestVersion(ctx, version.ProjectID, instance.MCVersion, modrinthLoader)
				if lookupErr == nil {
					latestModrinth[version.ProjectID] = latest
				}
			}
			if lookupErr != nil {
				entry.Error = lookupErr.Error()
			} else {
				entry.LatestVersion = latest.VersionNumber
				entry.LatestName = latest.Name
				entry.UpdateAvailable = latest.ID != version.ID
			}
		} else if jar.cfFile.ID != 0 && cfClient != nil {
			entry.Source = sourceCurseForge
			entry.ProjectID = strconv.FormatInt(jar.cfProject, 10)
			entry.VersionID = strconv.FormatInt(jar.cfFile.ID, 10)
			entry.CurrentVersion = firstString(jar.cfFile.DisplayName, jar.meta.Version, entry.CurrentVersion)
			entry.Identified = true
			latest, cached := latestCurseForge[jar.cfProject]
			var lookupErr error
			if !cached {
				latest, lookupErr = cfClient.LatestCompatibleFile(ctx, jar.cfProject, instance.MCVersion, instance.Loader)
				if lookupErr == nil {
					latestCurseForge[jar.cfProject] = latest
				}
			}
			if lookupErr != nil {
				entry.Error = lookupErr.Error()
			} else {
				entry.LatestVersion = firstString(latest.DisplayName, latest.FileName)
				entry.LatestName = latest.DisplayName
				entry.UpdateAvailable = latest.ID != jar.cfFile.ID
			}
		} else {
			entry.Error = "未通过 Modrinth SHA-1 或 CurseForge 指纹识别"
		}
		if entry.Error != "" {
			report.Warnings = appendUniqueString(report.Warnings, entry.Error)
		}
		if entry.Identified {
			report.Identified++
		} else {
			report.Unidentified++
		}
		if entry.UpdateAvailable {
			report.Updates++
		}
		report.Files = append(report.Files, entry)
	}
	sort.SliceStable(report.Files, func(i, j int) bool {
		if report.Files[i].UpdateAvailable != report.Files[j].UpdateAvailable {
			return report.Files[i].UpdateAvailable
		}
		if report.Files[i].Identified != report.Files[j].Identified {
			return report.Files[i].Identified
		}
		return report.Files[i].FileName < report.Files[j].FileName
	})
	return report, nil
}

// ModUpdateApplyRequest is one user-approved replacement.
type ModUpdateApplyRequest struct {
	InstanceID   string `json:"instanceId"`
	RelativePath string `json:"relativePath"`
	Source       string `json:"source"`
	ProjectID    string `json:"projectId"`
	VersionID    string `json:"versionId"`
	Force        bool   `json:"force"`
}

// ApplyModUpdate downloads the selected updated file, verifies conflicts against
// the other installed mods, and replaces the old jar after the new file has
// passed verification.
func (a *App) ApplyModUpdate(req ModUpdateApplyRequest) (result InstallProjectResult, err error) {
	if err := a.ready(); err != nil {
		return result, err
	}
	instance, ok := a.store.Instance(req.InstanceID)
	if !ok {
		return result, errors.New("版本不存在")
	}
	a.procMu.Lock()
	_, running := a.procs[req.InstanceID]
	a.procMu.Unlock()
	if running {
		return result, errors.New("请先关闭游戏再替换模组")
	}
	instanceDir := a.instanceDir(instance)
	relative := filepath.Clean(filepath.FromSlash(strings.TrimSpace(req.RelativePath)))
	if filepath.IsAbs(relative) || strings.HasPrefix(relative, "..") {
		return result, errors.New("要替换的文件路径无效")
	}
	slashed := filepath.ToSlash(relative)
	if !strings.HasPrefix(slashed, "mods/") {
		return result, errors.New("只能替换 mods/ 目录中的模组文件")
	}
	oldPath := filepath.Join(instanceDir, filepath.FromSlash(slashed))
	info, statErr := os.Stat(oldPath)
	if statErr != nil || !info.Mode().IsRegular() {
		return result, errors.New("要替换的模组文件不存在")
	}
	source := normalizeResourceSource(req.Source)
	cacheDir := filepath.Join(a.paths.Cache, "resources")
	rewriter := resourceRewriter(a.resourceSource(""))
	task := a.newTask("更新模组 · " + filepath.Base(oldPath))
	defer func() { task.finish(err) }()
	task.update("download", "正在下载更新…", 0, 0)

	var tempPath string
	var incompatibleFiles, incompatibleIDs []string
	var modrinthVersion modrinth.Version
	var curseClient *curseforge.Client
	var curseFile curseforge.File
	switch source {
	case sourceModrinth:
		version, fetchErr := modrinth.VersionByID(task.context(), req.VersionID)
		if fetchErr != nil {
			return result, fetchErr
		}
		if version.ProjectID != "" && req.ProjectID != "" && !strings.EqualFold(version.ProjectID, req.ProjectID) {
			return result, errors.New("更新版本与项目不匹配")
		}
		if !version.Compatible(instance.MCVersion, instance.Loader) {
			return result, errors.New("更新版本不兼容当前游戏版本或加载器")
		}
		modrinthVersion = version
		incompatibleFiles, incompatibleIDs = modrinthIncompatibilities(version)
		tempPath, err = modrinth.DownloadFileAny(task.context(), version, cacheDir, rewriter)
		result = InstallProjectResult{
			VersionID: version.ID, VersionName: version.Name,
			GameVersion: instance.MCVersion, Loader: instance.Loader, Dependencies: []string{},
		}
	case sourceCurseForge:
		projectID, parseErr := strconv.ParseInt(req.ProjectID, 10, 64)
		if parseErr != nil {
			return result, errors.New("CurseForge 项目 ID 无效")
		}
		fileID, parseErr := strconv.ParseInt(req.VersionID, 10, 64)
		if parseErr != nil {
			return result, errors.New("CurseForge 文件 ID 无效")
		}
		key := strings.TrimSpace(a.store.Settings().CurseForgeAPIKey)
		if key == "" {
			return result, errors.New("未配置 CurseForge API Key")
		}
		curseClient = curseforge.New(key)
		file, fetchErr := curseClient.FileByID(task.context(), projectID, fileID)
		if fetchErr != nil {
			return result, fetchErr
		}
		if file.ModID != projectID {
			return result, errors.New("更新文件与项目不匹配")
		}
		if !file.Compatible(instance.MCVersion, instance.Loader) {
			return result, errors.New("更新文件不兼容当前游戏版本或加载器")
		}
		curseFile = file
		incompatibleIDs = curseForgeIncompatibilities(file)
		tempPath, err = curseClient.DownloadFileAny(task.context(), file, cacheDir, rewriter)
		result = InstallProjectResult{
			VersionID: strconv.FormatInt(file.ID, 10), VersionName: firstString(file.DisplayName, file.FileName),
			GameVersion: instance.MCVersion, Loader: instance.Loader, Dependencies: []string{},
		}
	default:
		return result, errors.New("请选择更新来源")
	}
	if err != nil {
		return result, err
	}
	conflicts := detectModConflictsExcluding(instanceDir, tempPath, incompatibleFiles, incompatibleIDs, oldPath)
	if len(conflicts) > 0 && !req.Force {
		_ = os.Remove(tempPath)
		return result, &ResourceConflictError{Conflicts: conflicts}
	}
	rollbackPath, archiveErr := archiveOldMod(oldPath)
	if archiveErr != nil {
		_ = os.Remove(tempPath)
		return result, fmt.Errorf("保留旧模组失败（未替换任何文件）: %w", archiveErr)
	}
	installedPath, installErr := installDownloadedFile(tempPath, filepath.Dir(oldPath))
	if installErr != nil {
		partialPath := filepath.Join(filepath.Dir(oldPath), filepath.Base(tempPath))
		if partialPath != rollbackPath {
			_ = os.Remove(partialPath)
		}
		restoreErr := os.Rename(rollbackPath, oldPath)
		if restoreErr != nil {
			return result, fmt.Errorf("写入新模组失败: %w；旧模组保留在 %s，请手动改回 .jar", installErr, rollbackPath)
		}
		return result, fmt.Errorf("写入新模组失败，已恢复旧模组: %w", installErr)
	}
	result.FileName = filepath.Base(installedPath)
	result.Path = installedPath
	result.RollbackPath = rollbackPath
	if source == sourceModrinth {
		if deps, depErr := modrinth.InstallDependencies(task.context(), modrinthVersion, instance.MCVersion, instance.Loader, filepath.Dir(oldPath), 6, func(message string) {
			task.update("dependencies", message, 0, 0)
		}); depErr != nil {
			return result, depErr
		} else if deps != nil {
			result.Dependencies = deps
		}
	} else if curseClient != nil {
		if deps, depErr := curseClient.InstallDependencies(task.context(), curseFile, instance.MCVersion, instance.Loader, filepath.Dir(oldPath), 6, func(message string) {
			task.update("dependencies", message, 0, 0)
		}); depErr != nil {
			return result, depErr
		} else if deps != nil {
			result.Dependencies = deps
		}
	}
	task.update("done", "更新完成", 1, 1)
	a.emitInstancesChanged()
	return result, nil
}

func scanModJars(instanceDir string) ([]installedModJar, error) {
	modsDir := filepath.Join(instanceDir, "mods")
	info, err := os.Stat(modsDir)
	if err != nil || !info.IsDir() {
		return []installedModJar{}, nil
	}
	var out []installedModJar
	err = filepath.WalkDir(modsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || !modfilter.IsModJar(path) {
			return nil
		}
		relative, relErr := filepath.Rel(instanceDir, path)
		if relErr != nil {
			return nil
		}
		fileInfo, statErr := entry.Info()
		if statErr != nil {
			return nil
		}
		jar := installedModJar{
			path: path, relative: filepath.ToSlash(relative), fileName: entry.Name(), size: fileInfo.Size(),
		}
		jar.meta, _ = modfilter.ReadMetadata(path)
		jar.sha1, _ = hashFileSHA1(path)
		if data, readErr := os.ReadFile(path); readErr == nil {
			jar.fp = curseforge.Fingerprint(data)
		}
		out = append(out, jar)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].relative < out[j].relative })
	return out, nil
}

// archiveOldMod renames a replaced jar in place instead of deleting it. The
// ".old" file is invisible to mod loaders and update scans, but can be renamed
// back to ".jar" for a manual rollback. Existing backups get a numeric suffix.
func archiveOldMod(path string) (string, error) {
	candidate := path + ".old"
	if _, err := os.Lstat(candidate); !os.IsNotExist(err) {
		found := false
		for index := 1; index < 1000; index++ {
			candidate = fmt.Sprintf("%s.old.%d", path, index)
			if _, lookupErr := os.Lstat(candidate); os.IsNotExist(lookupErr) {
				found = true
				break
			}
		}
		if !found {
			candidate = fmt.Sprintf("%s.old.%d", path, time.Now().UnixNano())
		}
	}
	if err := os.Rename(path, candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

func hashFileSHA1(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha1.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func firstString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
