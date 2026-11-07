package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"MyMinecraftGoLuncher/internal/aisearch"
	"MyMinecraftGoLuncher/internal/curseforge"
	licensepkg "MyMinecraftGoLuncher/internal/license"
	"MyMinecraftGoLuncher/internal/modrinth"
	"MyMinecraftGoLuncher/internal/store"
)

const (
	sourceModrinth           = "modrinth"
	sourceCurseForge         = "curseforge"
	resourceTypeMod          = "mod"
	resourceTypeShader       = "shader"
	resourceTypeResourcepack = "resourcepack"
	resourceTypeDatapack     = "datapack"
	resourceTypeModpack      = "modpack"
)

// ResourceSearchRequest is a dual-provider catalog query.
type ResourceSearchRequest struct {
	InstanceID string `json:"instanceId"`
	Query      string `json:"query"`
	Type       string `json:"type"`
	Source     string `json:"source"`
	Sort       string `json:"sort"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	UseAI      bool   `json:"useAI"`
}

// ResourceProject is a normalized Modrinth/CurseForge result.
type ResourceProject struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	Source        string   `json:"source"`
	Type          string   `json:"type"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Author        string   `json:"author"`
	IconURL       string   `json:"iconUrl"`
	WebsiteURL    string   `json:"websiteUrl"`
	Downloads     int64    `json:"downloads"`
	Categories    []string `json:"categories"`
	GameVersions  []string `json:"gameVersions"`
	LatestVersion string   `json:"latestVersion"`
	Color         int64    `json:"color"`
	DateModified  string   `json:"dateModified"`
}

// ResourceSearchResult is a merged page from both platforms.
type ResourceSearchResult struct {
	Items             []ResourceProject `json:"items"`
	Total             int               `json:"total"`
	Offset            int               `json:"offset"`
	Limit             int               `json:"limit"`
	Warnings          []string          `json:"warnings"`
	CurseForgeReady   bool              `json:"curseForgeReady"`
	QueryUsed         string            `json:"queryUsed"`
	QueryAlternatives []string          `json:"queryAlternatives"`
	AIUsed            bool              `json:"aiUsed"`
	AIError           string            `json:"aiError"`
	CJKQuery          bool              `json:"cjkQuery"`
}

// ResourceVersion is a normalized download row.
type ResourceVersion struct {
	ID            string   `json:"id"`
	ProjectID     string   `json:"projectId"`
	Source        string   `json:"source"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"versionNumber"`
	ReleaseType   string   `json:"releaseType"`
	DatePublished string   `json:"datePublished"`
	GameVersions  []string `json:"gameVersions"`
	Loaders       []string `json:"loaders"`
	Downloads     int64    `json:"downloads"`
	FileName      string   `json:"fileName"`
	FileSize      int64    `json:"fileSize"`
	Downloadable  bool     `json:"downloadable"`
}

func normalizeResourceType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "mods":
		return resourceTypeMod
	case "shader", "shaders":
		return resourceTypeShader
	case "resourcepack", "resourcepacks", "texturepack":
		return resourceTypeResourcepack
	case "datapack", "datapacks":
		return resourceTypeDatapack
	case "modpack", "modpacks":
		return resourceTypeModpack
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func validResourceType(value string) bool {
	switch value {
	case resourceTypeMod, resourceTypeShader, resourceTypeResourcepack, resourceTypeDatapack, resourceTypeModpack:
		return true
	default:
		return false
	}
}

func normalizeResourceSource(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "curseforge":
		return sourceCurseForge
	case "modrinth":
		return sourceModrinth
	default:
		return "all"
	}
}

func modrinthType(resourceType string) string {
	switch resourceType {
	case resourceTypeShader:
		return modrinth.TypeShader
	case resourceTypeResourcepack:
		return modrinth.TypeResourcepack
	case resourceTypeDatapack:
		return "datapack"
	case resourceTypeModpack:
		return modrinth.TypeModpack
	default:
		return modrinth.TypeMod
	}
}

// SearchResources queries Modrinth and CurseForge concurrently and merges the
// two result sets. Missing CurseForge credentials degrade to Modrinth with a
// warning instead of breaking search.
func (a *App) SearchResources(req ResourceSearchRequest) (ResourceSearchResult, error) {
	if err := a.ready(); err != nil {
		return ResourceSearchResult{}, err
	}
	if req.UseAI {
		if err := a.requireFeature(licensepkg.FeatureAI); err != nil {
			return ResourceSearchResult{}, err
		}
	}
	req.Type = normalizeResourceType(req.Type)
	if !validResourceType(req.Type) {
		return ResourceSearchResult{}, errors.New("不支持的资源类型")
	}
	req.Source = normalizeResourceSource(req.Source)
	if req.Limit <= 0 || req.Limit > 30 {
		req.Limit = 12
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	settings := a.store.Settings()
	originalQuery := strings.TrimSpace(req.Query)
	normalizedQuery := aisearch.NormalizeQuery(originalQuery)
	searchQuery := normalizedQuery
	result := ResourceSearchResult{
		Items: []ResourceProject{}, Offset: req.Offset, Limit: req.Limit,
		CurseForgeReady: strings.TrimSpace(settings.CurseForgeAPIKey) != "", Warnings: []string{},
	}
	key := strings.TrimSpace(settings.CurseForgeAPIKey)
	includeMR := req.Source != sourceCurseForge
	includeCF := (req.Source == sourceCurseForge || req.Source == "all") && key != ""
	if req.Source == sourceCurseForge && key == "" {
		result.Warnings = append(result.Warnings, "未配置 CurseForge API Key，无法搜索 CurseForge。请在设置中填写 API Key。")
		return result, nil
	}
	if req.Source == "all" && key == "" {
		result.Warnings = append(result.Warnings, "未配置 CurseForge API Key，当前仅显示 Modrinth 结果。")
	}

	// Cache before the AI call so repeated searches (for example while
	// toggling tabs) do not pay for translation again.
	cacheKey := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%t|%s|%s|%s|%s",
		originalQuery, req.Type, req.Source, req.Sort, req.InstanceID, req.Offset, req.Limit,
		req.UseAI, settings.AISearchProvider, settings.AISearchModel,
		key, fmt.Sprint(includeMR)+":"+fmt.Sprint(includeCF))
	if cached, ok := a.loadResourceCache(cacheKey); ok {
		return cached, nil
	}

	if req.UseAI && settings.AISearchEnabled && aisearch.HasCJK(searchQuery) {
		translated, err := aisearch.TranslateQuery(a.context(), aisearch.Config{
			Provider: settings.AISearchProvider,
			APIKey:   settings.AISearchAPIKey,
			BaseURL:  settings.AISearchBaseURL,
			Model:    settings.AISearchModel,
		}, searchQuery)
		if err != nil {
			result.AIError = err.Error()
		} else if translated != "" {
			searchQuery = translated
			result.AIUsed = true
		}
	}
	// Search the normalized English keywords first, then the original Chinese
	// text (some projects have Chinese titles) and finally the unmodified query.
	// The first query is always the primary one used for pagination and sorting.
	queries := []string{}
	seenQueries := map[string]bool{}
	addQuery := func(value string) {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if seenQueries[strings.ToLower(value)] {
			return
		}
		seenQueries[strings.ToLower(value)] = true
		queries = append(queries, value)
	}
	addQuery(searchQuery)
	addQuery(normalizedQuery)
	addQuery(originalQuery)
	if len(queries) == 0 {
		queries = []string{""}
	}
	if req.Offset > 0 {
		// Multi-query result pages cannot be merged without duplicates. Keep
		// pagination stable and use the primary query for later pages.
		queries = queries[:1]
	}
	result.QueryAlternatives = append([]string(nil), queries...)
	result.QueryUsed = queries[0]
	result.CJKQuery = aisearch.HasCJK(queries[0]) && !result.AIUsed
	req.Query = queries[0]
	if req.Sort == "" {
		if req.Query == "" {
			req.Sort = "downloads"
		} else {
			req.Sort = "relevance"
		}
	}

	gameVersion := ""
	loaderName := ""
	if req.Type != resourceTypeModpack {
		if inst, ok := a.store.Instance(req.InstanceID); ok {
			gameVersion = inst.MCVersion
			if req.Type == resourceTypeMod && inst.Loader != "vanilla" {
				loaderName = inst.Loader
			}
		}
	}
	// Query candidates are independent; run them concurrently to keep Chinese
	// searches (which try up to three variants) from tripling latency.
	searchQueries := func(limit int, work func(query string) ([]ResourceProject, int, error)) (
		[]ResourceProject, int, int, error,
	) {
		type outcome struct {
			projects []ResourceProject
			total    int
			err      error
			ok       bool
		}
		results := make([]outcome, len(queries))
		var qwg sync.WaitGroup
		for index, query := range queries {
			qwg.Add(1)
			go func(index int, query string) {
				defer qwg.Done()
				projects, total, err := work(query)
				results[index] = outcome{projects: projects, total: total, err: err, ok: err == nil}
			}(index, query)
		}
		qwg.Wait()
		seen := map[string]bool{}
		merged := make([]ResourceProject, 0, limit*len(queries))
		total := 0
		successful := 0
		var lastErr error
		for _, item := range results {
			if !item.ok {
				lastErr = item.err
				continue
			}
			successful++
			if item.total > total {
				total = item.total
			}
			for _, project := range item.projects {
				if seen[project.ID] {
					continue
				}
				seen[project.ID] = true
				merged = append(merged, project)
			}
		}
		if successful == 0 {
			return nil, 0, 0, lastErr
		}
		rankResourceProjects(merged, queries[0])
		if limit > 0 && len(merged) > limit {
			merged = merged[:limit]
		}
		return merged, total, successful, nil
	}

	var wg sync.WaitGroup
	var mrProjects []ResourceProject
	var cfProjects []ResourceProject
	var mrTotal, cfTotal int
	var mrErr, cfErr error
	if includeMR {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mrProjects, mrTotal, _, mrErr = searchQueries(req.Limit, func(query string) ([]ResourceProject, int, error) {
				found, err := modrinth.Search(a.context(), modrinth.SearchRequest{
					Query: query, ProjectType: modrinthType(req.Type),
					GameVersion: gameVersion, Loader: loaderName,
					Offset: req.Offset, Limit: req.Limit, Sort: req.Sort,
				})
				if err != nil {
					return nil, 0, err
				}
				projects := make([]ResourceProject, 0, len(found.Hits))
				for _, project := range found.Hits {
					projects = append(projects, mapModrinthProject(project, req.Type))
				}
				return projects, found.TotalHits, nil
			})
		}()
	}
	if includeCF {
		wg.Add(1)
		go func() {
			defer wg.Done()
			classID, ok := curseforge.ClassIDFor(req.Type)
			if !ok {
				cfErr = errors.New("CurseForge 不支持该资源类型")
				return
			}
			cfGameVersion := gameVersion
			cfLoader := ""
			if req.Type == resourceTypeModpack {
				cfGameVersion = ""
			} else if req.Type == resourceTypeMod {
				cfLoader = loaderName
			}
			client := curseforge.New(key)
			cfProjects, cfTotal, _, cfErr = searchQueries(req.Limit, func(query string) ([]ResourceProject, int, error) {
				found, err := client.Search(a.context(), curseforge.SearchRequest{
					Query: query, ClassID: classID, GameVersion: cfGameVersion,
					Loader: cfLoader, Offset: req.Offset, Limit: req.Limit, Sort: req.Sort,
				})
				if err != nil {
					return nil, 0, err
				}
				projects := make([]ResourceProject, 0, len(found.Projects))
				for _, project := range found.Projects {
					projects = append(projects, mapCurseForgeProject(project, req.Type))
				}
				return projects, found.Total, nil
			})
		}()
	}
	wg.Wait()

	if mrErr != nil {
		result.Warnings = append(result.Warnings, "Modrinth 搜索失败："+mrErr.Error())
	}
	if cfErr != nil {
		result.Warnings = append(result.Warnings, "CurseForge 搜索失败："+cfErr.Error())
	}
	if includeMR && includeCF {
		result.Items = interleaveProjects(mrProjects, cfProjects)
	} else if includeMR {
		result.Items = mrProjects
	} else if includeCF {
		result.Items = cfProjects
	}
	result.Total = mrTotal + cfTotal
	if len(result.Items) == 0 && mrErr != nil && cfErr != nil {
		return result, errors.New("两个平台均搜索失败")
	}
	if len(result.Items) == 0 && mrErr != nil && !includeCF {
		return result, mrErr
	}
	if len(result.Items) == 0 && cfErr != nil && !includeMR {
		return result, cfErr
	}
	// Do not cache partial failures: a mirror/API hiccup should be retried.
	if (!includeMR || mrErr == nil) && (!includeCF || cfErr == nil) {
		a.storeResourceCache(cacheKey, result)
	}
	return result, nil
}

type resourceCacheEntry struct {
	result    ResourceSearchResult
	expiresAt time.Time
}

func (a *App) loadResourceCache(key string) (ResourceSearchResult, bool) {
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	entry, ok := a.resourceCache[key]
	if !ok || time.Now().After(entry.expiresAt) {
		if ok {
			delete(a.resourceCache, key)
		}
		return ResourceSearchResult{}, false
	}
	result := entry.result
	result.Items = append([]ResourceProject(nil), entry.result.Items...)
	result.Warnings = append([]string(nil), entry.result.Warnings...)
	result.QueryAlternatives = append([]string(nil), entry.result.QueryAlternatives...)
	return result, true
}

func (a *App) storeResourceCache(key string, result ResourceSearchResult) {
	if result.AIError != "" {
		return
	}
	a.resourceMu.Lock()
	defer a.resourceMu.Unlock()
	if a.resourceCache == nil || len(a.resourceCache) > 64 {
		a.resourceCache = map[string]resourceCacheEntry{}
	}
	a.resourceCache[key] = resourceCacheEntry{
		result: result, expiresAt: time.Now().Add(2 * time.Minute),
	}
}

func mapModrinthProject(project modrinth.Project, resourceType string) ResourceProject {
	websiteType := project.ProjectType
	if websiteType == "" {
		websiteType = resourceType
	}
	website := "https://modrinth.com/" + websiteType + "/" + project.Slug
	return ResourceProject{
		ID: "modrinth:" + project.ProjectID, ProjectID: project.ProjectID,
		Source: sourceModrinth, Type: resourceType, Slug: project.Slug,
		Title: project.Title, Description: project.Description, Author: project.Author,
		IconURL: project.IconURL, WebsiteURL: website, Downloads: project.Downloads,
		Categories: project.Categories, GameVersions: project.Versions,
		LatestVersion: project.LatestVer, Color: project.Color, DateModified: project.DateModified,
	}
}

func mapCurseForgeProject(project curseforge.Project, resourceType string) ResourceProject {
	categories := make([]string, 0, len(project.Categories))
	for _, category := range project.Categories {
		categories = append(categories, category.Name)
	}
	author := "CurseForge"
	if len(project.Authors) > 0 && project.Authors[0].Name != "" {
		author = project.Authors[0].Name
	}
	icon := project.Logo.ThumbnailURL
	if icon == "" {
		icon = project.Logo.URL
	}
	website := project.Links.WebsiteURL
	if website == "" {
		website = curseForgeWebsite(resourceType, project.Slug)
	}
	return ResourceProject{
		ID:        "curseforge:" + strconv.FormatInt(project.ID, 10),
		ProjectID: strconv.FormatInt(project.ID, 10), Source: sourceCurseForge,
		Type: resourceType, Slug: project.Slug, Title: project.Name,
		Description: project.Summary, Author: author, IconURL: icon,
		WebsiteURL: website, Downloads: project.DownloadCount,
		Categories: categories, DateModified: project.DateModified,
	}
}

func curseForgeWebsite(resourceType, slug string) string {
	section := "mc-mods"
	switch resourceType {
	case resourceTypeModpack:
		section = "modpacks"
	case resourceTypeResourcepack:
		section = "texture-packs"
	case resourceTypeShader:
		section = "shaders"
	case resourceTypeDatapack:
		section = "data-packs"
	}
	return "https://www.curseforge.com/minecraft/" + section + "/" + slug
}

func interleaveProjects(left, right []ResourceProject) []ResourceProject {
	out := make([]ResourceProject, 0, len(left)+len(right))
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for i := 0; i < maxLen; i++ {
		if i < len(left) {
			out = append(out, left[i])
		}
		if i < len(right) {
			out = append(out, right[i])
		}
	}
	return out
}

// ListResourceVersions returns compatible files from either platform.
func (a *App) ListResourceVersions(instanceID, source, projectID, resourceType string) ([]ResourceVersion, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	source = normalizeResourceSource(source)
	resourceType = normalizeResourceType(resourceType)
	if !validResourceType(resourceType) {
		return nil, errors.New("不支持的资源类型")
	}
	if source == "all" {
		return nil, errors.New("请选择具体平台以查看版本")
	}
	var inst store.Instance
	hasInstance := false
	if instanceID != "" {
		inst, hasInstance = a.store.Instance(instanceID)
	}
	if resourceType != resourceTypeModpack && !hasInstance {
		return nil, errors.New("请先选择一个版本")
	}
	gameVersion := ""
	loaderName := ""
	if resourceType != resourceTypeModpack {
		gameVersion = inst.MCVersion
		if resourceType == resourceTypeMod {
			if inst.Loader == "vanilla" {
				return nil, errors.New("原版游戏无法加载模组")
			}
			loaderName = inst.Loader
		}
	}
	switch source {
	case sourceModrinth:
		versions, err := modrinth.ListVersions(a.context(), projectID, gameVersion, loaderName)
		if err != nil {
			return nil, err
		}
		if resourceType != resourceTypeModpack {
			versions = modrinth.FilterCompatible(versions, gameVersion, loaderName)
		}
		out := make([]ResourceVersion, 0, len(versions))
		for _, version := range versions {
			file, _ := version.Primary()
			item := ResourceVersion{
				ID: version.ID, ProjectID: version.ProjectID, Source: sourceModrinth,
				Name: version.Name, VersionNumber: version.VersionNumber,
				ReleaseType: version.VersionType, DatePublished: version.DatePublished,
				GameVersions: version.GameVersions, Loaders: version.Loaders,
				Downloads: version.Downloads, Downloadable: file != nil,
			}
			if file != nil {
				item.FileName = file.Filename
				item.FileSize = file.Size
			}
			out = append(out, item)
		}
		return out, nil
	case sourceCurseForge:
		key := strings.TrimSpace(a.store.Settings().CurseForgeAPIKey)
		if key == "" {
			return nil, errors.New("未配置 CurseForge API Key")
		}
		id, err := strconv.ParseInt(projectID, 10, 64)
		if err != nil {
			return nil, errors.New("CurseForge 项目 ID 无效")
		}
		files, err := curseforge.New(key).Files(a.context(), id, gameVersion, loaderName)
		if err != nil {
			return nil, err
		}
		out := make([]ResourceVersion, 0, len(files))
		for _, file := range files {
			if resourceType != resourceTypeModpack && !file.Compatible(gameVersion, loaderName) {
				continue
			}
			out = append(out, ResourceVersion{
				ID: strconv.FormatInt(file.ID, 10), ProjectID: projectID, Source: sourceCurseForge,
				Name:          firstNonEmpty(file.DisplayName, file.FileName),
				VersionNumber: file.FileName, ReleaseType: file.ReleaseLabel(),
				DatePublished: file.FileDate, GameVersions: file.GameVersions,
				Loaders: loaderTokens(file.GameVersions), Downloads: file.DownloadCount,
				FileName: file.FileName, FileSize: file.FileLength, Downloadable: true,
			})
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].DatePublished > out[j].DatePublished })
		return out, nil
	default:
		return nil, errors.New("未知资源平台")
	}
}

func loaderTokens(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		lower := strings.ToLower(value)
		switch lower {
		case "fabric", "forge", "quilt", "neoforge":
			if !seen[lower] {
				seen[lower] = true
				out = append(out, lower)
			}
		}
	}
	return out
}

// InstallResource installs a Modrinth or CurseForge mod, shader, resource pack
// or data pack. Modpacks use InstallResourceModpack.
func (a *App) InstallResource(instanceID, source, projectID, versionID, mirrorID string, force bool) (result InstallProjectResult, err error) {
	if err := a.ready(); err != nil {
		return InstallProjectResult{}, err
	}
	source = normalizeResourceSource(source)
	if source == sourceModrinth {
		return a.installProject(instanceID, projectID, versionID, mirrorID, force)
	}
	if source != sourceCurseForge {
		return InstallProjectResult{}, errors.New("请选择具体资源平台")
	}
	inst, ok := a.store.Instance(instanceID)
	if !ok {
		return InstallProjectResult{}, errors.New("请先选择一个版本")
	}
	key := strings.TrimSpace(a.store.Settings().CurseForgeAPIKey)
	if key == "" {
		return InstallProjectResult{}, errors.New("未配置 CurseForge API Key")
	}
	projectIDInt, err := strconv.ParseInt(projectID, 10, 64)
	if err != nil {
		return InstallProjectResult{}, errors.New("CurseForge 项目 ID 无效")
	}
	client := curseforge.New(key)
	project, err := client.Mod(a.context(), projectIDInt)
	if err != nil {
		return InstallProjectResult{}, err
	}
	resourceType := curseforge.TypeForClass(project.ClassID)
	if resourceType == resourceTypeModpack {
		return InstallProjectResult{}, errors.New("整合包请使用 InstallResourceModpack")
	}
	gameVersion := inst.MCVersion
	loaderName := projectLoader(inst, resourceType)
	if resourceType == resourceTypeMod && loaderName == "" {
		return InstallProjectResult{}, errors.New("原版游戏无法安装模组")
	}
	var file curseforge.File
	if versionID != "" {
		fileID, parseErr := strconv.ParseInt(versionID, 10, 64)
		if parseErr != nil {
			return InstallProjectResult{}, errors.New("CurseForge 文件 ID 无效")
		}
		file, err = client.FileByID(a.context(), projectIDInt, fileID)
	} else {
		file, err = client.LatestCompatibleFile(a.context(), projectIDInt, gameVersion, loaderName)
	}
	if err != nil {
		return InstallProjectResult{}, err
	}
	if !file.Compatible(gameVersion, loaderName) {
		return InstallProjectResult{}, fmt.Errorf("所选文件不兼容 %s / %s", gameVersion, firstNonEmpty(loaderName, "原版"))
	}
	subdir, err := modrinth.InstallPath(resourceType)
	if err != nil {
		return InstallProjectResult{}, err
	}
	t := a.newTask("安装 " + project.Name)
	defer func() { t.finish(err) }()
	t.update("download", "正在下载 "+project.Name+"…", 0, 0)
	dir := filepath.Join(a.instanceDir(inst), subdir)
	rewriter := resourceRewriter(a.resourceSource(mirrorID))
	var path string
	if resourceType == resourceTypeMod {
		cacheDir := filepath.Join(a.paths.Cache, "resources")
		tempPath, downloadErr := client.DownloadFileAny(t.context(), file, cacheDir, rewriter)
		if downloadErr != nil {
			return InstallProjectResult{}, downloadErr
		}
		conflicts := detectModConflicts(a.instanceDir(inst), tempPath, nil, curseForgeIncompatibilities(file))
		if len(conflicts) > 0 && !force {
			_ = os.Remove(tempPath)
			return InstallProjectResult{}, &ResourceConflictError{Conflicts: conflicts}
		}
		path, err = installDownloadedFile(tempPath, dir)
	} else {
		path, err = client.DownloadFileAny(t.context(), file, dir, rewriter)
	}
	if err != nil {
		return InstallProjectResult{}, err
	}
	result = InstallProjectResult{
		FileName: filepath.Base(path), Path: path, VersionID: versionID,
		VersionName: firstNonEmpty(file.DisplayName, file.FileName), GameVersion: gameVersion, Loader: loaderName,
		Dependencies: []string{},
	}
	if resourceType == resourceTypeMod {
		t.update("dependencies", "正在检查前置模组…", 0, 0)
		deps, depErr := client.InstallDependencies(t.context(), file, gameVersion, loaderName, dir, 6, func(message string) {
			t.update("dependencies", message, 0, 0)
		})
		if deps == nil {
			deps = []string{}
		}
		result.Dependencies = deps
		if depErr != nil {
			return result, depErr
		}
	}
	t.update("done", "安装完成", 1, 1)
	a.emitInstancesChanged()
	return result, nil
}

// InstallResourceModpack installs a modpack from either platform and creates a
// new instance. It does not use an existing instance's game version.
func (a *App) InstallResourceModpack(source, projectID, versionID, mirrorID string) (resp ModpackImportResponse, err error) {
	if err := a.ready(); err != nil {
		return ModpackImportResponse{}, err
	}
	source = normalizeResourceSource(source)
	if source == sourceModrinth {
		return a.InstallModpackProject(projectID, versionID, mirrorID)
	}
	if source != sourceCurseForge {
		return ModpackImportResponse{}, errors.New("请选择具体资源平台")
	}
	key := strings.TrimSpace(a.store.Settings().CurseForgeAPIKey)
	if key == "" {
		return ModpackImportResponse{}, errors.New("未配置 CurseForge API Key")
	}
	projectIDInt, err := strconv.ParseInt(projectID, 10, 64)
	if err != nil {
		return ModpackImportResponse{}, errors.New("CurseForge 项目 ID 无效")
	}
	client := curseforge.New(key)
	var file curseforge.File
	if versionID != "" {
		fileID, parseErr := strconv.ParseInt(versionID, 10, 64)
		if parseErr != nil {
			return ModpackImportResponse{}, errors.New("CurseForge 文件 ID 无效")
		}
		file, err = client.FileByID(a.context(), projectIDInt, fileID)
	} else {
		file, err = client.LatestCompatibleFile(a.context(), projectIDInt, "", "")
	}
	if err != nil {
		return ModpackImportResponse{}, err
	}
	cacheDir := filepath.Join(a.paths.Cache, "modpacks")
	t := a.newTask("安装整合包")
	defer func() { t.finish(err) }()
	t.update("download", "正在下载 CurseForge 整合包…", 0, 0)
	sourceMirror := a.resourceSource(mirrorID)
	path, err := client.DownloadFileAny(t.context(), file, cacheDir, resourceRewriter(sourceMirror))
	if err != nil {
		return ModpackImportResponse{}, err
	}
	return a.importModpack(t.context(), path, t, sourceMirror)
}

// OpenResourceURL is kept for frontend links and validates the scheme.
func (a *App) OpenResourceURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return errors.New("链接无效")
	}
	return a.OpenURL(raw)
}
