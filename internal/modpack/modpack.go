package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/util"
)

// Format identifies a supported modpack container.
type Format string

const (
	FormatMRPack     Format = "mrpack"
	FormatCurseForge Format = "curseforge"
	FormatMultiMC    Format = "multimc"
	FormatGeneric    Format = "generic"
)

// FormatLabel returns a UI label.
func FormatLabel(f Format) string {
	switch f {
	case FormatMRPack:
		return "Modrinth (.mrpack)"
	case FormatCurseForge:
		return "CurseForge (.zip)"
	case FormatMultiMC:
		return "Prism / MultiMC"
	default:
		return "通用压缩包"
	}
}

// Inspection is the result of peeking into a modpack without importing it.
type Inspection struct {
	Path          string   `json:"path"`
	Format        Format   `json:"format"`
	FormatLabel   string   `json:"formatLabel"`
	Name          string   `json:"name"`
	MCVersion     string   `json:"mcVersion"`
	Loader        string   `json:"loader"`
	LoaderVersion string   `json:"loaderVersion"`
	FileCount     int      `json:"fileCount"`
	Size          int64    `json:"size"`
	HasOverrides  bool     `json:"hasOverrides"`
	Warnings      []string `json:"warnings"`

	basePrefix string
}

// ImportOptions provides fallbacks for ambiguous archives.
type ImportOptions struct {
	NameOverride          string `json:"nameOverride"`
	FallbackMCVersion     string `json:"fallbackMcVersion"`
	FallbackLoader        string `json:"fallbackLoader"`
	FallbackLoaderVersion string `json:"fallbackLoaderVersion"`
	CurseForgeAPIKey      string `json:"-"`
	GameDir               string `json:"-"`
}

// ProgressFunc receives import/export progress.
type ProgressFunc func(stage, message string, current, total int)

// ImportResult describes a completed import.
type ImportResult struct {
	Instance     store.Instance `json:"instance"`
	Format       Format         `json:"format"`
	FormatLabel  string         `json:"formatLabel"`
	MissingFiles int            `json:"missingFiles"`
	Warnings     []string       `json:"warnings"`
}

type mrpackIndex struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Summary       string            `json:"summary"`
	Files         []mrpackFile      `json:"files"`
	Dependencies  map[string]string `json:"dependencies"`
}

type mrpackFile struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       map[string]string `json:"env"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

type curseManifest struct {
	Minecraft struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID      string `json:"id"`
			Primary bool   `json:"primary"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	ManifestType string `json:"manifestType"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	Overrides    string `json:"overrides"`
	Files        []struct {
		ProjectID int  `json:"projectID"`
		FileID    int  `json:"fileID"`
		Required  bool `json:"required"`
	} `json:"files"`
}

type mmcPack struct {
	FormatVersion int `json:"formatVersion"`
	Components    []struct {
		UID     string `json:"uid"`
		Version string `json:"version"`
	} `json:"components"`
}

// Inspect detects the modpack format and extracts metadata.
func Inspect(path string) (Inspection, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Inspection{}, fmt.Errorf("无法读取整合包: %w", err)
	}
	if info.IsDir() || !info.Mode().IsRegular() {
		return Inspection{}, errors.New("请选择整合包压缩文件")
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Inspection{}, fmt.Errorf("无法打开压缩包（仅支持 .zip/.mrpack）: %w", err)
	}
	defer zr.Close()

	ins := Inspection{Path: path, Size: info.Size(), Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	entries := map[string]*zip.File{}
	var totalSize int64
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		entries[name] = f
		if !f.FileInfo().IsDir() {
			ins.FileCount++
			totalSize += int64(f.UncompressedSize64)
		}
	}
	top := commonTopLevel(entries)
	root := top
	prefix := ""
	if root != "" {
		prefix = root + "/"
	}
	find := func(name string) *zip.File {
		if f := entries[name]; f != nil {
			return f
		}
		if prefix != "" {
			return entries[prefix+name]
		}
		return nil
	}
	ins.basePrefix = prefix

	if f := find("modrinth.index.json"); f != nil {
		var index mrpackIndex
		if err := decodeZipJSON(f, &index); err != nil {
			return Inspection{}, fmt.Errorf("Modrinth 整合包索引损坏: %w", err)
		}
		ins.Format = FormatMRPack
		ins.FormatLabel = FormatLabel(ins.Format)
		ins.Name = firstNonEmpty(index.Name, ins.Name)
		ins.MCVersion = index.Dependencies["minecraft"]
		ins.Loader, ins.LoaderVersion = loaderFromDependencies(index.Dependencies)
		ins.HasOverrides = entries[prefix+"overrides/"] != nil || hasPrefix(entries, prefix+"overrides/") ||
			hasPrefix(entries, prefix+"client-overrides/")
		return ins, nil
	}
	if f := find("manifest.json"); f != nil {
		var manifest curseManifest
		if err := decodeZipJSON(f, &manifest); err == nil && manifest.Minecraft.Version != "" {
			ins.Format = FormatCurseForge
			ins.FormatLabel = FormatLabel(ins.Format)
			ins.Name = firstNonEmpty(manifest.Name, ins.Name)
			ins.MCVersion = manifest.Minecraft.Version
			for _, l := range manifest.Minecraft.ModLoaders {
				if l.Primary || ins.Loader == "" {
					ins.Loader, ins.LoaderVersion = parseLoaderID(l.ID)
				}
			}
			overrideDir := firstNonEmpty(manifest.Overrides, "overrides")
			ins.HasOverrides = hasPrefix(entries, prefix+overrideDir+"/")
			if ins.Loader == "" {
				ins.Loader = "vanilla"
			}
			return ins, nil
		}
	}
	if f := find("mmc-pack.json"); f != nil {
		var pack mmcPack
		if err := decodeZipJSON(f, &pack); err == nil {
			ins.Format = FormatMultiMC
			ins.FormatLabel = FormatLabel(ins.Format)
			for _, comp := range pack.Components {
				switch comp.UID {
				case "net.minecraft":
					ins.MCVersion = comp.Version
				case "net.fabricmc.fabric-loader":
					ins.Loader, ins.LoaderVersion = "fabric", comp.Version
				case "org.quiltmc.quilt-loader":
					ins.Loader, ins.LoaderVersion = "quilt", comp.Version
				case "net.minecraftforge":
					ins.Loader, ins.LoaderVersion = "forge", comp.Version
				case "net.neoforged", "net.neoforged.fancymodloader":
					ins.Loader, ins.LoaderVersion = "neoforge", comp.Version
				}
			}
			if f := find("instance.cfg"); f != nil {
				if name := iniValue(f, "name"); name != "" {
					ins.Name = name
				}
			}
			ins.HasOverrides = hasPrefix(entries, prefix+"minecraft/") || hasPrefix(entries, prefix+".minecraft/")
			if ins.Loader == "" {
				ins.Loader = "vanilla"
			}
			return ins, nil
		}
	}
	ins.Format = FormatGeneric
	ins.FormatLabel = FormatLabel(ins.Format)
	ins.Warnings = append(ins.Warnings, "未能识别整合包元数据，将按普通文件包导入；请确认游戏版本")
	return ins, nil
}

// Import creates an instance and restores pack files.
func Import(ctx context.Context, st *store.Store, path string, opts ImportOptions, progress ProgressFunc) (ImportResult, error) {
	ins, err := Inspect(path)
	if err != nil {
		return ImportResult{}, err
	}
	mcVersion := firstNonEmpty(ins.MCVersion, opts.FallbackMCVersion)
	if mcVersion == "" {
		return ImportResult{}, errors.New("无法确定整合包对应的 Minecraft 版本")
	}
	loaderType := ins.Loader
	if loaderType == "" {
		loaderType = opts.FallbackLoader
	}
	if loaderType == "" {
		loaderType = "vanilla"
	}
	loaderVersion := ins.LoaderVersion
	if loaderVersion == "" {
		loaderVersion = opts.FallbackLoaderVersion
	}
	name := firstNonEmpty(opts.NameOverride, ins.Name, filepath.Base(path))
	inst, err := st.CreateInstance(store.Instance{
		Name:            name,
		MCVersion:       mcVersion,
		Loader:          loaderType,
		LoaderVersion:   loaderVersion,
		LaunchVersionID: mcVersion,
		MemoryMB:        st.Settings().DefaultMemoryMB,
		Source:          string(ins.Format),
		Notes:           "从 " + filepath.Base(path) + " 导入",
	})
	if err != nil {
		return ImportResult{}, err
	}
	gameDir := strings.TrimSpace(opts.GameDir)
	if gameDir == "" {
		gameDir = st.InstanceDir(inst.ID)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = st.DeleteInstance(inst.ID, true)
		}
	}()

	result := ImportResult{
		Instance:    inst,
		Format:      ins.Format,
		FormatLabel: FormatLabel(ins.Format),
		Warnings:    append([]string{}, ins.Warnings...),
	}
	switch ins.Format {
	case FormatMRPack:
		missing, warnings, err := importMRPack(ctx, path, ins, gameDir, opts, progress)
		if err != nil {
			return ImportResult{}, err
		}
		result.MissingFiles = missing
		result.Warnings = append(result.Warnings, warnings...)
	case FormatCurseForge:
		missing, warnings, err := importCurseForge(ctx, path, ins, gameDir, opts, progress)
		if err != nil {
			return ImportResult{}, err
		}
		result.MissingFiles = missing
		result.Warnings = append(result.Warnings, warnings...)
	case FormatMultiMC:
		if err := extractMultiMC(path, ins, gameDir, progress); err != nil {
			return ImportResult{}, err
		}
	case FormatGeneric:
		if err := extractGeneric(path, ins, gameDir, progress); err != nil {
			return ImportResult{}, err
		}
	}
	cleanup = false
	result.Instance = inst
	return result, nil
}

func importMRPack(ctx context.Context, path string, ins Inspection, gameDir string, opts ImportOptions, progress ProgressFunc) (int, []string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, nil, err
	}
	defer zr.Close()
	var index *zip.File
	for _, f := range zr.File {
		if strings.ReplaceAll(f.Name, "\\", "/") == ins.basePrefix+"modrinth.index.json" {
			index = f
			break
		}
	}
	if index == nil {
		return 0, nil, errors.New("缺少 modrinth.index.json")
	}
	var idx mrpackIndex
	if err := decodeZipJSON(index, &idx); err != nil {
		return 0, nil, err
	}
	report(progress, "extract", "正在展开整合包配置…", 0, 1)
	if err := extractZipPrefix(path, gameDir, ins.basePrefix+"overrides"); err != nil {
		return 0, nil, err
	}
	if err := extractZipPrefix(path, gameDir, ins.basePrefix+"client-overrides"); err != nil {
		return 0, nil, err
	}
	var downloads []genericDownload
	missing := 0
	var warnings []string
	for _, f := range idx.Files {
		if strings.EqualFold(f.Env["client"], "unsupported") {
			continue
		}
		if len(f.Downloads) == 0 {
			missing++
			if len(warnings) < 6 {
				warnings = append(warnings, "缺少外部文件下载地址: "+f.Path)
			}
			continue
		}
		dest, err := util.SafeJoin(gameDir, f.Path)
		if err != nil {
			return 0, nil, err
		}
		downloads = append(downloads, genericDownload{
			url: f.Downloads[0], dest: dest,
			sha512: f.Hashes["sha512"], sha1: f.Hashes["sha1"], size: f.FileSize,
		})
	}
	if len(downloads) > 0 {
		report(progress, "download", fmt.Sprintf("正在下载整合包文件 0/%d", len(downloads)), 0, len(downloads))
		if err := downloadGeneric(ctx, downloads, 6, func(done int) {
			report(progress, "download", fmt.Sprintf("正在下载整合包文件 %d/%d", done, len(downloads)), done, len(downloads))
		}); err != nil {
			return missing, warnings, err
		}
	}
	return missing, warnings, nil
}

type genericDownload struct {
	url    string
	dest   string
	sha512 string
	sha1   string
	size   int64
}

func importCurseForge(ctx context.Context, path string, ins Inspection, gameDir string, opts ImportOptions, progress ProgressFunc) (int, []string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return 0, nil, err
	}
	var manifestFile *zip.File
	for _, f := range zr.File {
		if strings.ReplaceAll(f.Name, "\\", "/") == ins.basePrefix+"manifest.json" {
			manifestFile = f
			break
		}
	}
	if manifestFile == nil {
		zr.Close()
		return 0, nil, errors.New("缺少 manifest.json")
	}
	var manifest curseManifest
	err = decodeZipJSON(manifestFile, &manifest)
	zr.Close()
	if err != nil {
		return 0, nil, err
	}
	overrideDir := firstNonEmpty(manifest.Overrides, "overrides")
	report(progress, "extract", "正在展开 CurseForge 覆盖文件…", 0, 1)
	if err := extractZipPrefix(path, gameDir, ins.basePrefix+overrideDir); err != nil {
		return 0, nil, err
	}
	if len(manifest.Files) == 0 {
		return 0, nil, nil
	}
	if strings.TrimSpace(opts.CurseForgeAPIKey) == "" {
		return len(manifest.Files), []string{fmt.Sprintf("未配置 CurseForge API Key，已跳过 %d 个模组文件；可在设置中填写后重新导入", len(manifest.Files))}, nil
	}
	report(progress, "download", fmt.Sprintf("正在解析 CurseForge 文件 0/%d", len(manifest.Files)), 0, len(manifest.Files))
	missing := 0
	var downloads []genericDownload
	for i, f := range manifest.Files {
		if f.ProjectID == 0 || f.FileID == 0 {
			missing++
			continue
		}
		url, filename, err := curseFileURL(ctx, opts.CurseForgeAPIKey, f.ProjectID, f.FileID)
		name := filepath.Base(strings.TrimSpace(filename))
		if err != nil || url == "" || name == "" || name == "." || name == string(filepath.Separator) {
			missing++
		} else {
			downloads = append(downloads, genericDownload{
				url:  url,
				dest: filepath.Join(gameDir, "mods", name),
			})
		}
		report(progress, "download", fmt.Sprintf("正在解析 CurseForge 文件 %d/%d", i+1, len(manifest.Files)), i+1, len(manifest.Files))
		time.Sleep(70 * time.Millisecond)
	}
	if len(downloads) > 0 {
		report(progress, "download", fmt.Sprintf("正在下载 CurseForge 文件 0/%d", len(downloads)), 0, len(downloads))
		if err := downloadGeneric(ctx, downloads, 4, func(done int) {
			report(progress, "download", fmt.Sprintf("正在下载 CurseForge 文件 %d/%d", done, len(downloads)), done, len(downloads))
		}); err != nil {
			return missing, nil, err
		}
	}
	var warnings []string
	if missing > 0 {
		warnings = append(warnings, fmt.Sprintf("有 %d 个 CurseForge 文件无法自动下载（作者禁止第三方下载或文件已删除）", missing))
	}
	return missing, warnings, nil
}

func curseFileURL(ctx context.Context, apiKey string, projectID, fileID int) (string, string, error) {
	endpoint := fmt.Sprintf("https://api.curseforge.com/v1/mods/%d/files/%d", projectID, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("CurseForge API HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Data struct {
			FileName    string `json:"fileName"`
			DownloadURL string `json:"downloadUrl"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&payload); err != nil {
		return "", "", err
	}
	return payload.Data.DownloadURL, payload.Data.FileName, nil
}

func extractMultiMC(path string, ins Inspection, gameDir string, progress ProgressFunc) error {
	report(progress, "extract", "正在恢复 MultiMC 版本…", 0, 1)
	prefixes := []string{
		ins.basePrefix + "minecraft",
		ins.basePrefix + ".minecraft",
	}
	for _, prefix := range prefixes {
		if zipHasPrefix(path, prefix+"/") {
			return extractZipPrefix(path, gameDir, prefix)
		}
	}
	// Some exports put the game directory at the archive root.
	return extractZipPrefix(path, gameDir, strings.TrimSuffix(ins.basePrefix, "/"))
}

func extractGeneric(path string, ins Inspection, gameDir string, progress ProgressFunc) error {
	report(progress, "extract", "正在展开压缩包…", 0, 1)
	prefix := strings.TrimSuffix(ins.basePrefix, "/")
	if prefix == "mods" || prefix == "minecraft" || prefix == ".minecraft" {
		return extractZipPrefix(path, gameDir, prefix)
	}
	return extractZipPrefix(path, gameDir, prefix)
}

func extractZipPrefix(zipPath, dest, prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return util.ExtractZip(zipPath, dest, "")
	}
	return util.ExtractZip(zipPath, dest, prefix)
}

func zipHasPrefix(zipPath, prefix string) bool {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return false
	}
	defer zr.Close()
	for _, f := range zr.File {
		if strings.HasPrefix(strings.ReplaceAll(f.Name, "\\", "/"), prefix) {
			return true
		}
	}
	return false
}

func downloadGeneric(ctx context.Context, downloads []genericDownload, workers int, onProgress func(done int)) error {
	if len(downloads) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 4
	}
	if workers > len(downloads) {
		workers = len(downloads)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan genericDownload)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				if ctx.Err() != nil {
					return
				}
				err := httpx.Download(ctx, httpx.File{
					URL: d.url, Dest: d.dest, SHA512: d.sha512, SHA1: d.sha1, Size: d.size,
					Verify: d.sha512 != "" || d.sha1 != "",
				})
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					cancel()
					return
				}
				mu.Lock()
				done++
				current := done
				mu.Unlock()
				if onProgress != nil {
					onProgress(current)
				}
			}
		}()
	}
	for _, d := range downloads {
		select {
		case jobs <- d:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func decodeZipJSON(f *zip.File, out any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	dec := json.NewDecoder(io.LimitReader(rc, 32<<20))
	return dec.Decode(out)
}

func iniValue(f *zip.File, key string) string {
	rc, err := f.Open()
	if err != nil {
		return ""
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key+"=") {
			return strings.TrimSpace(strings.TrimPrefix(line, key+"="))
		}
	}
	return ""
}

func commonTopLevel(entries map[string]*zip.File) string {
	first := ""
	for name, f := range entries {
		if f.FileInfo().IsDir() {
			continue
		}
		name = strings.TrimPrefix(name, "./")
		parts := strings.SplitN(name, "/", 2)
		if len(parts) != 2 {
			return ""
		}
		if first == "" {
			first = parts[0]
		} else if first != parts[0] {
			return ""
		}
	}
	if first == "" {
		return ""
	}
	// Never strip well-known content folders on their own.
	switch first {
	case "mods", "config", "minecraft", ".minecraft", "overrides", "saves":
		return ""
	}
	return first
}

func hasPrefix(entries map[string]*zip.File, prefix string) bool {
	for name := range entries {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func loaderFromDependencies(deps map[string]string) (string, string) {
	switch {
	case deps["fabric-loader"] != "":
		return "fabric", deps["fabric-loader"]
	case deps["quilt-loader"] != "":
		return "quilt", deps["quilt-loader"]
	case deps["neoforge"] != "":
		return "neoforge", deps["neoforge"]
	case deps["forge"] != "":
		return "forge", deps["forge"]
	default:
		return "vanilla", ""
	}
}

func parseLoaderID(id string) (string, string) {
	id = strings.TrimSpace(id)
	lower := strings.ToLower(id)
	for _, loader := range []string{"neoforge", "fabric", "quilt", "forge"} {
		if strings.HasPrefix(lower, loader+"-") {
			return loader, strings.TrimPrefix(id, loader+"-")
		}
	}
	return "vanilla", ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func report(fn ProgressFunc, stage, message string, current, total int) {
	if fn != nil {
		fn(stage, message, current, total)
	}
}

// ExportMRPack writes a portable Modrinth pack containing all instance files
// as overrides. This keeps custom configs and local mods intact.
func ExportMRPack(ctx context.Context, inst store.Instance, gameDir, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deps := map[string]string{"minecraft": inst.MCVersion}
	switch strings.ToLower(inst.Loader) {
	case "fabric":
		deps["fabric-loader"] = inst.LoaderVersion
	case "quilt":
		deps["quilt-loader"] = inst.LoaderVersion
	case "forge":
		deps["forge"] = inst.LoaderVersion
	case "neoforge":
		deps["neoforge"] = inst.LoaderVersion
	}
	index := mrpackIndex{
		FormatVersion: 1,
		Game:          "minecraft",
		VersionID:     "1.0.0",
		Name:          inst.Name,
		Summary:       "由 MyGoLuncher 导出的整合包",
		Files:         []mrpackFile{},
		Dependencies:  deps,
	}
	indexData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	if err := addZipBytes(zw, "modrinth.index.json", indexData); err != nil {
		return err
	}
	if err := addDirToZip(zw, gameDir, "overrides", exportSkip); err != nil {
		return err
	}
	return zw.Close()
}

// ExportZip writes a Prism/MultiMC-compatible instance archive.
func ExportZip(ctx context.Context, inst store.Instance, gameDir, dest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	components := []map[string]string{{"uid": "net.minecraft", "version": inst.MCVersion}}
	switch strings.ToLower(inst.Loader) {
	case "fabric":
		components = append(components, map[string]string{"uid": "net.fabricmc.fabric-loader", "version": inst.LoaderVersion})
	case "quilt":
		components = append(components, map[string]string{"uid": "org.quiltmc.quilt-loader", "version": inst.LoaderVersion})
	case "forge":
		components = append(components, map[string]string{"uid": "net.minecraftforge", "version": inst.LoaderVersion})
	case "neoforge":
		components = append(components, map[string]string{"uid": "net.neoforged", "version": inst.LoaderVersion})
	}
	pack := map[string]any{"formatVersion": 1, "components": components}
	packData, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	cfg := fmt.Sprintf("InstanceType=OneSix\nname=%s\n", inst.Name)
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	if err := addZipBytes(zw, "mmc-pack.json", packData); err != nil {
		return err
	}
	if err := addZipBytes(zw, "instance.cfg", []byte(cfg)); err != nil {
		return err
	}
	if err := addDirToZip(zw, gameDir, "minecraft", exportSkip); err != nil {
		return err
	}
	return zw.Close()
}

func exportSkip(rel string, info os.FileInfo) bool {
	if info.IsDir() {
		return false
	}
	first := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
	switch first {
	case "logs", "crash-reports", "natives", "screenshots", ".cache":
		return true
	}
	base := filepath.Base(rel)
	switch base {
	case "usercache.json", "launcher_profiles.json", "realms_persistence.json":
		return true
	}
	return strings.HasSuffix(base, ".log")
}

func addDirToZip(zw *zip.Writer, root, prefix string, skip func(rel string, info os.FileInfo) bool) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	return filepath.Walk(rootAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(rootAbs, path)
		if err != nil || rel == "." {
			return nil
		}
		if skip != nil && skip(rel, info) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		zipName := prefix + "/" + filepath.ToSlash(rel)
		w, err := zw.Create(zipName)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func addZipBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// SortedFormats returns formats for UI hints.
func SortedFormats() []string {
	formats := []string{".mrpack", "CurseForge .zip", "Prism / MultiMC .zip", "通用 .zip"}
	sort.Strings(formats)
	return formats
}
