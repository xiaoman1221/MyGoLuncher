package mojang

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/mirror"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/util"
)

const (
	ManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"
	ObjectURL   = "https://resources.download.minecraft.net/"
)

// ProgressFunc receives coarse task updates. current/total are item counts.
type ProgressFunc func(stage, message string, current, total int)

// Client owns Mojang metadata and the shared game data layout.
type Client struct {
	paths store.Paths

	mu        sync.Mutex
	manifest  *Manifest
	fetchedAt time.Time

	sourcesMu sync.RWMutex
	sources   []mirror.Source
}

// New creates a Mojang metadata client.
func New(paths store.Paths) *Client {
	return &Client{paths: paths, sources: []mirror.Source{mirror.Official()}}
}

// SetSources configures the download sources used by metadata and installs.
func (c *Client) SetSources(sources []mirror.Source) {
	if len(sources) == 0 {
		sources = []mirror.Source{mirror.Official()}
	}
	c.sourcesMu.Lock()
	c.sources = append([]mirror.Source(nil), sources...)
	c.sourcesMu.Unlock()
}

func (c *Client) sourceList() []mirror.Source {
	c.sourcesMu.RLock()
	defer c.sourcesMu.RUnlock()
	if len(c.sources) == 0 {
		return []mirror.Source{mirror.Official()}
	}
	return append([]mirror.Source(nil), c.sources...)
}

// Manifest returns the version list, using a 10 minute memory cache and a
// disk fallback when the network is unavailable.
func (c *Client) Manifest(ctx context.Context) (Manifest, error) {
	c.mu.Lock()
	if c.manifest != nil && time.Since(c.fetchedAt) < 10*time.Minute {
		m := *c.manifest
		c.mu.Unlock()
		return m, nil
	}
	c.mu.Unlock()

	cachePath := filepath.Join(c.paths.Meta, "version_manifest_v2.json")
	urls := mirror.ManifestURLs(c.sourceList())
	if len(urls) == 0 {
		urls = []string{ManifestURL}
	}
	var err error
	for _, endpoint := range urls {
		raw, getErr := httpx.GetRaw(ctx, endpoint)
		if getErr != nil {
			err = getErr
		} else {
			var m Manifest
			// A mirror returning 200 with an error page must not prevent the
			// next candidate (including the official source) from being tried.
			if jsonErr := json.Unmarshal(raw, &m); jsonErr == nil && len(m.Versions) > 0 {
				_ = os.WriteFile(cachePath, raw, 0o644)
				SortVersions(m.Versions)
				c.mu.Lock()
				c.manifest = &m
				c.fetchedAt = time.Now()
				c.mu.Unlock()
				return m, nil
			}
			err = errors.New("版本清单格式异常")
		}
		if ctx.Err() != nil {
			return Manifest{}, ctx.Err()
		}
	}

	var m Manifest
	if jsonErr := util.ReadJSON(cachePath, &m); jsonErr == nil && len(m.Versions) > 0 {
		SortVersions(m.Versions)
		c.mu.Lock()
		c.manifest = &m
		c.fetchedAt = time.Now()
		c.mu.Unlock()
		return m, nil
	}
	return Manifest{}, fmt.Errorf("无法获取 Minecraft 版本清单: %w", err)
}

// ManifestEntry finds a version in the manifest.
func (c *Client) ManifestEntry(ctx context.Context, id string) (VersionEntry, error) {
	m, err := c.Manifest(ctx)
	if err != nil {
		return VersionEntry{}, err
	}
	for _, entry := range m.Versions {
		if entry.ID == id {
			return entry, nil
		}
	}
	return VersionEntry{}, fmt.Errorf("未找到版本 %s", id)
}

// LatestRelease returns the latest stable version ID.
func (c *Client) LatestRelease(ctx context.Context) (string, error) {
	m, err := c.Manifest(ctx)
	if err != nil {
		return "", err
	}
	return m.Latest.Release, nil
}

// LocalVersion loads a version profile from disk.
func (c *Client) LocalVersion(id string) (Version, error) {
	path := filepath.Join(c.paths.Versions, id, id+".json")
	var v Version
	if err := util.ReadJSON(path, &v); err != nil {
		return Version{}, err
	}
	return v, nil
}

// EnsureVersion downloads and caches the version JSON if needed.
func (c *Client) EnsureVersion(ctx context.Context, id string) (Version, error) {
	if v, err := c.LocalVersion(id); err == nil && v.ID != "" {
		return v, nil
	}
	entry, err := c.ManifestEntry(ctx, id)
	if err != nil {
		return Version{}, err
	}
	var raw []byte
	var v Version
	var lastErr error
	for _, endpoint := range mirror.CandidateURLs(c.sourceList(), mirror.ResourceVersion, entry.URL, id) {
		candidate, getErr := httpx.GetRaw(ctx, endpoint)
		if getErr != nil {
			lastErr = getErr
			continue
		}
		if entry.SHA1 != "" && !verifySHA1(candidate, entry.SHA1) {
			lastErr = errors.New("版本元数据校验失败")
			continue
		}
		var parsed Version
		if jsonErr := json.Unmarshal(candidate, &parsed); jsonErr != nil {
			lastErr = fmt.Errorf("解析版本元数据失败: %w", jsonErr)
			continue
		}
		raw, v = candidate, parsed
		lastErr = nil
		break
	}
	if raw == nil {
		if lastErr == nil {
			lastErr = errors.New("没有可用的下载地址")
		}
		return Version{}, fmt.Errorf("获取版本元数据失败: %w", lastErr)
	}
	if v.ID == "" {
		v.ID = id
	}
	dir := filepath.Join(c.paths.Versions, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Version{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), raw, 0o644); err != nil {
		return Version{}, err
	}
	return v, nil
}

// Chain returns the version followed by its inheritance parents.
func (c *Client) Chain(id string) ([]Version, error) {
	var chain []Version
	seen := map[string]bool{}
	current := id
	for current != "" {
		if seen[current] {
			return nil, fmt.Errorf("版本继承链存在循环: %s", current)
		}
		seen[current] = true
		v, err := c.LocalVersion(current)
		if err != nil {
			return nil, fmt.Errorf("版本 %s 尚未安装: %w", current, err)
		}
		chain = append(chain, v)
		current = v.InheritsFrom
		if len(chain) > 8 {
			return nil, errors.New("版本继承链过深")
		}
	}
	return chain, nil
}

// Merged returns the effective profile for a launch version.
func (c *Client) Merged(id string) (Version, error) {
	chain, err := c.Chain(id)
	if err != nil {
		return Version{}, err
	}
	return Merge(chain), nil
}

// IsInstalled reports whether a full game install completed.
func (c *Client) IsInstalled(id string) bool {
	_, err := os.Stat(filepath.Join(c.paths.Versions, id, ".installed.json"))
	return err == nil
}

// MarkProfileInstalled records that a loader profile finished installing.
func (c *Client) MarkProfileInstalled(id string) error {
	v, err := c.LocalVersion(id)
	if err != nil {
		return err
	}
	return util.WriteJSON(filepath.Join(c.paths.Versions, id, ".installed.json"), installedMarker{
		VersionID: id, JavaMajor: v.JavaMajor(), InstalledAt: time.Now(), Loader: true,
	})
}

// IsProfileInstalled reports whether a loader profile was installed and its
// version JSON is still readable. A leftover marker without a usable profile
// must trigger a reinstall instead of being trusted.
func (c *Client) IsProfileInstalled(id string) bool {
	if !c.IsInstalled(id) {
		return false
	}
	profile, err := c.LocalVersion(id)
	return err == nil && strings.TrimSpace(profile.MainClass) != ""
}

// ClientJar returns the path to a version's client jar.
func (c *Client) ClientJar(id string) string {
	return filepath.Join(c.paths.Versions, id, id+".jar")
}

// NativeDir returns the shared native extraction directory for a version.
func (c *Client) NativeDir(id string) string {
	return filepath.Join(c.paths.Versions, id, "natives")
}

type installedMarker struct {
	VersionID   string    `json:"versionId"`
	AssetsID    string    `json:"assetsId"`
	JavaMajor   int       `json:"javaMajor"`
	InstalledAt time.Time `json:"installedAt"`
	Loader      bool      `json:"loader,omitempty"`
}

// Install downloads a complete vanilla version. Enabled mirrors are speed
// tested using a real asset before bulk downloads begin.
func (c *Client) Install(ctx context.Context, id string, workers int, progress ProgressFunc) error {
	return c.install(ctx, id, workers, progress, false)
}

// Repair verifies every vanilla version file against its checksum and
// re-downloads missing or corrupted files. Existing valid files are kept.
func (c *Client) Repair(ctx context.Context, id string, workers int, progress ProgressFunc) error {
	return c.install(ctx, id, workers, progress, true)
}

func (c *Client) install(ctx context.Context, id string, workers int, progress ProgressFunc, repair bool) error {
	if !repair && c.IsInstalled(id) && util.NonEmptyFile(c.ClientJar(id)) {
		report(progress, "done", "版本已安装", 1, 1)
		return nil
	}
	markerPath := filepath.Join(c.paths.Versions, id, ".installed.json")
	if repair {
		_ = os.Remove(markerPath)
	}
	v, err := c.EnsureVersion(ctx, id)
	if err != nil {
		return err
	}
	if workers < 1 {
		workers = 8
	}
	if workers > 24 {
		workers = 24
	}
	report(progress, "metadata", "正在准备版本 "+id, 0, 1)

	assetID := v.AssetIndex.ID
	if assetID == "" {
		assetID = v.Assets
	}
	indexPath := ""
	var catalog *AssetCatalog
	if v.AssetIndex.URL != "" {
		report(progress, "assets", "正在下载资源索引…", 0, 1)
		indexPath = filepath.Join(c.paths.Assets, "indexes", assetID+".json")
		urls := mirror.CandidateURLs(c.sourceList(), mirror.ResourceAssetIndex, v.AssetIndex.URL, assetID)
		if err := httpx.DownloadAny(ctx, urls, httpx.File{
			Dest: indexPath, SHA1: v.AssetIndex.SHA1, Size: v.AssetIndex.Size, Verify: repair,
		}); err != nil {
			return fmt.Errorf("下载资源索引失败: %w", err)
		}
		var loaded AssetCatalog
		if err := util.ReadJSON(indexPath, &loaded); err == nil {
			catalog = &loaded
		}
	}

	// Measure real mirrors before downloading the game files.
	ranked := c.rankSources(ctx, c.sourceList(), catalog, progress)

	// Client jar.
	if client, ok := v.Downloads["client"]; ok && client.URL != "" {
		report(progress, "client", "正在下载游戏本体…", 0, 1)
		dest := c.ClientJar(id)
		urls := mirror.CandidateURLs(ranked, mirror.ResourceClient, client.URL, id)
		if err := httpx.DownloadAny(ctx, urls, httpx.File{Dest: dest, SHA1: client.SHA1, Size: client.Size, Verify: repair}); err != nil {
			return fmt.Errorf("下载游戏本体失败: %w", err)
		}
		report(progress, "client", "游戏本体下载完成", 1, 1)
	} else {
		return errors.New("该版本没有可用的客户端下载")
	}

	// Libraries and natives.
	report(progress, "libraries", "正在解析运行库…", 0, 1)
	files, natives, err := c.libraryDownloads(v, ranked, repair)
	if err != nil {
		return err
	}
	report(progress, "libraries", "正在下载运行库…", 0, len(files))
	if err := c.downloadFiles(ctx, files, workers, func(done int) {
		report(progress, "libraries", fmt.Sprintf("运行库 %d/%d", done, len(files)), done, len(files))
	}); err != nil {
		return err
	}

	nativeDir := c.NativeDir(id)
	if suffix := NativeDirectorySuffix(v.Arguments.JVM); suffix != "" {
		nativeDir = filepath.Join(nativeDir, suffix)
	}
	if len(natives) > 0 {
		report(progress, "natives", "正在释放本地库…", 0, len(natives))
		for idx, native := range natives {
			if err := ExtractNativeJar(native.jarPath, nativeDir, native.excludes); err != nil {
				return fmt.Errorf("释放本地库 %s 失败: %w", filepath.Base(native.jarPath), err)
			}
			report(progress, "natives", "正在释放本地库…", idx+1, len(natives))
		}
	}

	if indexPath != "" {
		report(progress, "assets", "正在同步游戏资源…", 0, 1)
		if err := c.downloadAssets(ctx, indexPath, workers, ranked, progress, repair); err != nil {
			return err
		}
	}

	// Logging configuration.
	if v.Logging.Client != nil && v.Logging.Client.File.URL != "" {
		report(progress, "logging", "正在准备日志配置…", 0, 1)
		f := v.Logging.Client.File
		dest := filepath.Join(c.paths.Assets, "log_configs", f.ID)
		urls := mirror.CandidateURLs(ranked, mirror.ResourceLogging, f.URL, id)
		if err := httpx.DownloadAny(ctx, urls, httpx.File{Dest: dest, SHA1: f.SHA1, Size: f.Size, Verify: repair}); err != nil {
			return fmt.Errorf("下载日志配置失败: %w", err)
		}
	}

	marker := installedMarker{
		VersionID:   id,
		AssetsID:    assetID,
		JavaMajor:   v.JavaMajor(),
		InstalledAt: time.Now(),
	}
	if err := util.WriteJSON(filepath.Join(c.paths.Versions, id, ".installed.json"), marker); err != nil {
		return err
	}
	report(progress, "done", "版本安装完成", 1, 1)
	return nil
}

// rankSources speed-tests enabled sources with a representative asset and
// returns them from fastest to slowest. Official remains available as a final
// per-file fallback.
func (c *Client) rankSources(ctx context.Context, sources []mirror.Source, catalog *AssetCatalog, progress ProgressFunc) []mirror.Source {
	probeURL := ""
	if catalog != nil {
		hash := pickProbeAsset(*catalog)
		if hash != "" {
			probeURL = ObjectURL + hash[:2] + "/" + hash
		}
	}
	resource := mirror.ResourceAsset
	if probeURL == "" {
		probeURL = ManifestURL
		resource = mirror.ResourceManifest
	}
	report(progress, "mirror", "正在测试镜像下载速度…", 0, len(sources))
	results := mirror.Test(ctx, sources, func(source mirror.Source) (string, bool) {
		return source.URL(resource, probeURL, "")
	}, 10*time.Second)
	var parts []string
	okCount := 0
	for _, result := range results {
		if result.OK {
			okCount++
			parts = append(parts, result.Name+" "+mirror.HumanSpeed(result.BytesPerSec))
		}
	}
	ranked := mirror.Ranked(results)
	message := "镜像测速失败，使用官方回退"
	if okCount > 0 && len(ranked) > 0 {
		message = "测速完成 · " + strings.Join(parts, " · ") + " → 使用 " + ranked[0].Name
	}
	report(progress, "mirror", message, okCount, len(results))
	return ranked
}

func pickProbeAsset(catalog AssetCatalog) string {
	bestHash := ""
	var bestSize int64
	for _, object := range catalog.Objects {
		if len(object.Hash) < 2 {
			continue
		}
		if object.Size >= 32<<10 && (bestHash == "" || object.Size < bestSize) {
			bestHash, bestSize = object.Hash, object.Size
		}
	}
	if bestHash != "" {
		return bestHash
	}
	for _, object := range catalog.Objects {
		if len(object.Hash) >= 2 && (bestHash == "" || object.Size < bestSize) {
			bestHash, bestSize = object.Hash, object.Size
		}
	}
	return bestHash
}

// InstallProfile downloads a client loader profile (Fabric/Quilt) and its
// libraries. The inherited vanilla version must be installed separately.
func (c *Client) InstallProfile(ctx context.Context, profile Version, workers int, progress ProgressFunc) error {
	return c.InstallProfileWithSources(ctx, profile, workers, progress, c.sourceList())
}

// InstallProfileWithSources is InstallProfile with an explicit ranked mirror
// list, so loader libraries follow the same network selection as metadata.
func (c *Client) InstallProfileWithSources(ctx context.Context, profile Version, workers int, progress ProgressFunc, sources []mirror.Source) error {
	if profile.ID == "" {
		return errors.New("加载器档案缺少 ID")
	}
	if profile.InheritsFrom == "" {
		return errors.New("加载器档案缺少父版本")
	}
	if _, err := c.LocalVersion(profile.InheritsFrom); err != nil {
		return fmt.Errorf("需要先安装原版 %s", profile.InheritsFrom)
	}
	if workers < 1 {
		workers = 8
	}
	dir := filepath.Join(c.paths.Versions, profile.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, profile.ID+".json"), data, 0o644); err != nil {
		return err
	}
	report(progress, "loader", "正在解析加载器运行库…", 0, 1)
	files, natives, err := c.libraryDownloads(profile, sources, true)
	if err != nil {
		return err
	}
	report(progress, "loader", "正在下载加载器运行库…", 0, len(files))
	if err := c.downloadFiles(ctx, files, workers, func(done int) {
		report(progress, "loader", fmt.Sprintf("加载器运行库 %d/%d", done, len(files)), done, len(files))
	}); err != nil {
		return err
	}
	if len(natives) > 0 {
		for _, native := range natives {
			if err := ExtractNativeJar(native.jarPath, c.NativeDir(profile.ID), native.excludes); err != nil {
				return err
			}
		}
	}
	marker := installedMarker{
		VersionID:   profile.ID,
		JavaMajor:   profile.JavaMajor(),
		InstalledAt: time.Now(),
		Loader:      true,
	}
	return util.WriteJSON(filepath.Join(dir, ".installed.json"), marker)
}

type fileTask struct {
	file httpx.File
	urls []string
}

type nativeTask struct {
	jarPath  string
	excludes []string
}

// RepairProfile verifies a locally installed loader/game profile and
// re-downloads its libraries and natives. The vanilla parent should be repaired
// separately with Repair.
func (c *Client) RepairProfile(ctx context.Context, id string, workers int, progress ProgressFunc) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("版本 ID 不能为空")
	}
	profile, err := c.Merged(id)
	if err != nil {
		return fmt.Errorf("读取版本档案失败: %w", err)
	}
	if workers < 1 {
		workers = 8
	}
	if workers > 24 {
		workers = 24
	}
	dir := filepath.Join(c.paths.Versions, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(dir, ".installed.json"))
	report(progress, "libraries", "正在校验版本运行库…", 0, 1)
	files, natives, err := c.libraryDownloads(profile, c.sourceList(), true)
	if err != nil {
		return err
	}
	if err := c.downloadFiles(ctx, files, workers, func(done int) {
		report(progress, "libraries", fmt.Sprintf("运行库 %d/%d", done, len(files)), done, len(files))
	}); err != nil {
		return err
	}
	for _, native := range natives {
		if err := ExtractNativeJar(native.jarPath, c.NativeDir(id), native.excludes); err != nil {
			return fmt.Errorf("补充本地库失败: %w", err)
		}
	}
	marker := installedMarker{
		VersionID: id, JavaMajor: profile.JavaMajor(), InstalledAt: time.Now(), Loader: true,
	}
	if err := util.WriteJSON(filepath.Join(dir, ".installed.json"), marker); err != nil {
		return err
	}
	report(progress, "done", "版本运行库校验完成", 1, 1)
	return nil
}

func (c *Client) libraryDownloads(v Version, sources []mirror.Source, verify bool) ([]fileTask, []nativeTask, error) {
	seen := map[string]bool{}
	var files []fileTask
	var natives []nativeTask
	add := func(url, dest, sha1 string, size int64) {
		if url == "" || dest == "" || seen[dest] {
			return
		}
		seen[dest] = true
		urls := mirror.CandidateURLs(sources, mirror.ResourceLibrary, url, v.ID)
		files = append(files, fileTask{
			file: httpx.File{URL: url, Dest: dest, SHA1: sha1, Size: size, Verify: verify},
			urls: urls,
		})
	}
	for _, lib := range v.Libraries {
		if !RulesAllow(lib.Rules, nil) {
			continue
		}
		if lib.Downloads.Artifact != nil {
			dl := lib.Downloads.Artifact
			dest := c.libraryPath(dl.Path, lib)
			if dest != "" {
				add(dl.URL, dest, dl.SHA1, dl.Size)
				// Modern native libraries are separate Maven artifacts rather
				// than classifiers, so they are both downloaded and unpacked.
				if len(lib.Natives) == 0 && IsNativeLibrary(lib) {
					natives = append(natives, nativeTask{jarPath: dest, excludes: extractExcludes(lib)})
				}
			}
		} else if len(lib.Natives) == 0 {
			// Loader profiles often provide only a Maven repository base URL.
			dl, err := LibraryDownload(lib)
			if err != nil {
				return nil, nil, err
			}
			dest := c.libraryPath(dl.Path, lib)
			if dest != "" {
				add(dl.URL, dest, dl.SHA1, dl.Size)
			}
		}
		// Legacy libraries keep native binaries in a classifier next to the
		// regular classes artifact. Both need to be installed.
		if len(lib.Natives) > 0 {
			native := NativeArtifact(lib)
			if native == nil {
				continue
			}
			dest := c.libraryPath(native.Path, lib)
			if dest == "" {
				continue
			}
			add(native.URL, dest, native.SHA1, native.Size)
			natives = append(natives, nativeTask{jarPath: dest, excludes: extractExcludes(lib)})
		}
	}
	return files, natives, nil
}

func (c *Client) libraryPath(mavenPath string, lib Library) string {
	if mavenPath == "" {
		var err error
		mavenPath, err = ArtifactPath(lib.Name)
		if err != nil {
			return ""
		}
	}
	return filepath.Join(c.paths.Libraries, filepath.FromSlash(mavenPath))
}

func extractExcludes(lib Library) []string {
	if lib.Extract == nil {
		return nil
	}
	return lib.Extract.Exclude
}

func (c *Client) downloadFiles(ctx context.Context, files []fileTask, workers int, onProgress func(done int)) error {
	if len(files) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 8
	}
	if workers > len(files) {
		workers = len(files)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := 0
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := httpx.DownloadAny(ctx, files[idx].urls, files[idx].file); err != nil {
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
	for idx := range files {
		select {
		case jobs <- idx:
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

func (c *Client) downloadAssets(ctx context.Context, indexPath string, workers int, sources []mirror.Source, progress ProgressFunc, verify bool) error {
	var catalog AssetCatalog
	if err := util.ReadJSON(indexPath, &catalog); err != nil {
		return fmt.Errorf("解析资源索引失败: %w", err)
	}
	type assetRef struct {
		hash string
		size int64
	}
	unique := map[string]assetRef{}
	for _, obj := range catalog.Objects {
		unique[obj.Hash] = assetRef{hash: obj.Hash, size: obj.Size}
	}
	total := len(unique)
	if total == 0 {
		report(progress, "assets", "该版本没有额外资源", 0, 0)
		return nil
	}
	if workers < 1 {
		workers = 8
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan assetRef)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	done := 0
	report(progress, "assets", fmt.Sprintf("游戏资源 0/%d", total), 0, total)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range jobs {
				if ctx.Err() != nil {
					return
				}
				if len(ref.hash) < 2 {
					continue
				}
				dest := filepath.Join(c.paths.Assets, "objects", ref.hash[:2], ref.hash)
				url := ObjectURL + ref.hash[:2] + "/" + ref.hash
				urls := mirror.CandidateURLs(sources, mirror.ResourceAsset, url, "")
				err := httpx.DownloadAny(ctx, urls, httpx.File{Dest: dest, SHA1: ref.hash, Size: ref.size, Verify: verify})
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
				if current%8 == 0 || current == total {
					report(progress, "assets", fmt.Sprintf("游戏资源 %d/%d", current, total), current, total)
				}
			}
		}()
	}
	for _, ref := range unique {
		select {
		case jobs <- ref:
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
		return fmt.Errorf("下载游戏资源失败: %w", firstErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if catalog.Virtual {
		report(progress, "assets", "正在构建旧版资源目录…", total, total)
		indexID := strings.TrimSuffix(filepath.Base(indexPath), ".json")
		if err := MaterializeAssets(catalog, filepath.Join(c.paths.Assets, "objects"), filepath.Join(c.paths.Assets, "virtual", indexID)); err != nil {
			return fmt.Errorf("构建旧版资源目录失败: %w", err)
		}
	}
	return nil
}

func verifySHA1(raw []byte, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return true
	}
	sum := sha1.Sum(raw)
	return strings.EqualFold(hex.EncodeToString(sum[:]), want)
}

func report(fn ProgressFunc, stage, message string, current, total int) {
	if fn != nil {
		fn(stage, message, current, total)
	}
}

// JavaMajorForMerged returns the Java requirement for a merged version.
func JavaMajorForMerged(v Version) int {
	return v.JavaMajor()
}

// VersionLabel trims a loader profile to something readable.
func VersionLabel(id string) string {
	return strings.TrimSpace(id)
}
