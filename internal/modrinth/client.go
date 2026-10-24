package modrinth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
	"MyMinecraftGoLuncher/internal/util"
)

const API = "https://api.modrinth.com/v2"

// ProjectType values accepted by Modrinth.
const (
	TypeMod          = "mod"
	TypeShader       = "shader"
	TypeResourcepack = "resourcepack"
	TypeModpack      = "modpack"
)

// SearchRequest describes a catalog query.
type SearchRequest struct {
	Query       string `json:"query"`
	ProjectType string `json:"projectType"`
	GameVersion string `json:"gameVersion"`
	Loader      string `json:"loader"`
	Offset      int    `json:"offset"`
	Limit       int    `json:"limit"`
	Sort        string `json:"sort"`
}

// SearchResult is a page of projects.
type SearchResult struct {
	Hits      []Project `json:"hits"`
	TotalHits int       `json:"total_hits"`
	Offset    int       `json:"offset"`
	Limit     int       `json:"limit"`
}

// Project is a Modrinth project summary.
type Project struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	ProjectType  string   `json:"project_type"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Author       string   `json:"author"`
	Categories   []string `json:"categories"`
	Downloads    int64    `json:"downloads"`
	Follows      int64    `json:"follows"`
	IconURL      string   `json:"icon_url"`
	Versions     []string `json:"versions"`
	DateModified string   `json:"date_modified"`
	LatestVer    string   `json:"latest_version"`
	Color        int64    `json:"color"`
}

// Version is one published project version.
type Version struct {
	ID            string       `json:"id"`
	ProjectID     string       `json:"project_id"`
	Name          string       `json:"name"`
	VersionNumber string       `json:"version_number"`
	GameVersions  []string     `json:"game_versions"`
	Loaders       []string     `json:"loaders"`
	DatePublished string       `json:"date_published"`
	VersionType   string       `json:"version_type"`
	Downloads     int64        `json:"downloads"`
	Files         []File       `json:"files"`
	Dependencies  []Dependency `json:"dependencies"`
}

// File is a downloadable version artifact.
type File struct {
	URL      string            `json:"url"`
	Filename string            `json:"filename"`
	Primary  bool              `json:"primary"`
	Size     int64             `json:"size"`
	Hashes   map[string]string `json:"hashes"`
}

// Dependency is a project relationship declared by a version.
type Dependency struct {
	VersionID      string `json:"version_id"`
	ProjectID      string `json:"project_id"`
	FileName       string `json:"file_name"`
	DependencyType string `json:"dependency_type"`
}

// Primary returns the main file for a version.
func (v Version) Primary() (*File, error) {
	for idx := range v.Files {
		if v.Files[idx].Primary {
			return &v.Files[idx], nil
		}
	}
	if len(v.Files) > 0 {
		return &v.Files[0], nil
	}
	return nil, errors.New("该版本没有可下载文件")
}

// Search queries Modrinth projects.
func Search(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if req.Limit <= 0 || req.Limit > 50 {
		req.Limit = 20
	}
	facets := [][]string{}
	if req.ProjectType != "" {
		facets = append(facets, []string{"project_type:" + req.ProjectType})
	}
	if req.GameVersion != "" && req.GameVersion != "all" {
		facets = append(facets, []string{"versions:" + req.GameVersion})
	}
	if req.Loader != "" && req.Loader != "all" {
		facets = append(facets, []string{"categories:" + req.Loader})
	}
	q := url.Values{}
	q.Set("query", req.Query)
	q.Set("limit", fmt.Sprintf("%d", req.Limit))
	q.Set("offset", fmt.Sprintf("%d", max(req.Offset, 0)))
	sortBy := req.Sort
	if sortBy == "" {
		if strings.TrimSpace(req.Query) == "" {
			sortBy = "downloads"
		} else {
			sortBy = "relevance"
		}
	}
	q.Set("index", sortBy)
	if len(facets) > 0 {
		raw, _ := json.Marshal(facets)
		q.Set("facets", string(raw))
	}
	var out SearchResult
	endpoint := API + "/search?" + q.Encode()
	if err := getJSON(ctx, endpoint, &out); err != nil {
		return SearchResult{}, err
	}
	if out.Hits == nil {
		out.Hits = []Project{}
	}
	return out, nil
}

// ListVersions returns versions filtered by game version and loader.
func ListVersions(ctx context.Context, projectID, gameVersion, loader string) ([]Version, error) {
	q := url.Values{}
	if gameVersion != "" && gameVersion != "all" {
		raw, _ := json.Marshal([]string{gameVersion})
		q.Set("game_versions", string(raw))
	}
	if loader != "" && loader != "all" {
		raw, _ := json.Marshal([]string{loader})
		q.Set("loaders", string(raw))
	}
	endpoint := fmt.Sprintf("%s/project/%s/version", API, url.PathEscape(projectID))
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	var versions []Version
	if err := getJSON(ctx, endpoint, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

// VersionsByHashes resolves installed file hashes to Modrinth versions using
// the batch endpoint. Algorithm is normally "sha1" or "sha512".
func VersionsByHashes(ctx context.Context, hashes []string, algorithm string) (map[string]Version, error) {
	return versionsByHashesAt(ctx, API, hashes, algorithm)
}

func versionsByHashesAt(ctx context.Context, base string, hashes []string, algorithm string) (map[string]Version, error) {
	if len(hashes) == 0 {
		return map[string]Version{}, nil
	}
	if algorithm == "" {
		algorithm = "sha1"
	}
	body, err := json.Marshal(map[string]any{"hashes": hashes, "algorithm": algorithm})
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, base+"/version_files", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Modrinth 失败: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		if len(message) > 300 {
			message = message[:300]
		}
		return nil, fmt.Errorf("Modrinth HTTP %d: %s", resp.StatusCode, message)
	}
	var out map[string]Version
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("解析 Modrinth 版本响应失败: %w", err)
	}
	if out == nil {
		out = map[string]Version{}
	}
	return out, nil
}

// VersionByID fetches one published version.
func VersionByID(ctx context.Context, id string) (Version, error) {
	var v Version
	if err := getJSON(ctx, API+"/version/"+url.PathEscape(id), &v); err != nil {
		return Version{}, err
	}
	return v, nil
}

// ProjectByID fetches project details.
func ProjectByID(ctx context.Context, id string) (Project, error) {
	var p Project
	if err := getJSON(ctx, API+"/project/"+url.PathEscape(id), &p); err != nil {
		return Project{}, err
	}
	if p.ProjectID == "" {
		p.ProjectID = p.ID
	}
	return p, nil
}

// Compatible reports whether a version supports the target game version and
// loader. An empty loader means the project type does not depend on a loader
// (for example a shader pack).
func (v Version) Compatible(gameVersion, loader string) bool {
	if gameVersion != "" && gameVersion != "all" {
		found := false
		for _, version := range v.GameVersions {
			if strings.EqualFold(version, gameVersion) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if loader != "" && loader != "all" && loader != "vanilla" {
		found := false
		for _, value := range v.Loaders {
			if strings.EqualFold(value, loader) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	_, err := v.Primary()
	return err == nil
}

// FilterCompatible removes versions that do not match the target instance.
func FilterCompatible(versions []Version, gameVersion, loader string) []Version {
	out := make([]Version, 0, len(versions))
	for _, version := range versions {
		if version.Compatible(gameVersion, loader) {
			out = append(out, version)
		}
	}
	return out
}

// BestVersion picks the newest release-compatible version, falling back to
// beta and alpha builds if necessary. Compatibility is re-checked locally so a
// caller cannot accidentally install a version for another loader.
func BestVersion(ctx context.Context, projectID, gameVersion, loader string) (Version, error) {
	versions, err := ListVersions(ctx, projectID, gameVersion, loader)
	if err != nil {
		return Version{}, err
	}
	versions = FilterCompatible(versions, gameVersion, loader)
	if len(versions) == 0 {
		return Version{}, errors.New("没有找到兼容的版本")
	}
	rank := map[string]int{"release": 3, "beta": 2, "alpha": 1}
	best := versions[0]
	bestRank := rank[best.VersionType]
	for _, v := range versions[1:] {
		r := rank[v.VersionType]
		if r > bestRank {
			best = v
			bestRank = r
		}
	}
	return best, nil
}

// DownloadFile downloads a version's primary file into dir and returns its path.
func DownloadFile(ctx context.Context, v Version, dir string) (string, error) {
	return DownloadFileAny(ctx, v, dir, nil)
}

// DownloadFileAny downloads a version's primary file through optional mirror
// candidates. The original URL is always the final fallback.
func DownloadFileAny(ctx context.Context, v Version, dir string, rewriter func(string) []string) (string, error) {
	f, err := v.Primary()
	if err != nil {
		return "", err
	}
	name := filepath.Base(strings.TrimSpace(f.Filename))
	if name == "." || name == "" || name == string(filepath.Separator) {
		return "", errors.New("文件名无效")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	urls := []string{f.URL}
	if rewriter != nil {
		if candidates := rewriter(f.URL); len(candidates) > 0 {
			urls = candidates
		}
	}
	err = httpx.DownloadAny(ctx, urls, httpx.File{
		Dest:   dest,
		SHA512: f.Hashes["sha512"],
		SHA1:   f.Hashes["sha1"],
		Size:   f.Size,
		Verify: true,
	})
	if err != nil {
		return "", err
	}
	return dest, nil
}

// DownloadRaw downloads any URL to a destination with checksum support.
func DownloadRaw(ctx context.Context, rawURL, dest, sha512 string, size int64) error {
	return httpx.Download(ctx, httpx.File{URL: rawURL, Dest: dest, SHA512: sha512, Size: size, Verify: sha512 != ""})
}

// InstallDependencies resolves and downloads required dependencies recursively.
// It returns the names of installed dependency projects.
func InstallDependencies(ctx context.Context, parent Version, gameVersion, loader, dir string, maxDepth int, onProgress func(string)) ([]string, error) {
	if maxDepth <= 0 {
		return nil, nil
	}
	visited := map[string]bool{parent.ProjectID: true}
	var installed []string
	var walk func(parent Version, depth int) error
	walk = func(parent Version, depth int) error {
		if depth > maxDepth {
			return nil
		}
		for _, dep := range parent.Dependencies {
			if dep.DependencyType != "required" {
				continue
			}
			projectID := dep.ProjectID
			var child Version
			var err error
			if dep.VersionID != "" {
				child, err = VersionByID(ctx, dep.VersionID)
				if err != nil {
					return fmt.Errorf("解析前置依赖 %s 失败: %w", dependencyName(dep), err)
				}
				if projectID == "" {
					projectID = child.ProjectID
				}
				// The declared file may target another game version or loader.
				// Fall back to the newest compatible version in that case.
				if !child.Compatible(gameVersion, loader) {
					child, err = BestVersion(ctx, projectID, gameVersion, loader)
				}
			} else if projectID != "" {
				child, err = BestVersion(ctx, projectID, gameVersion, loader)
			} else {
				continue
			}
			if projectID == "" || visited[projectID] {
				continue
			}
			if err != nil {
				return fmt.Errorf("补全前置依赖 %s 失败（需要兼容版本 %s/%s）: %w", dependencyName(dep), gameVersion, loader, err)
			}
			visited[projectID] = true
			if onProgress != nil {
				display := child.Name
				if strings.TrimSpace(display) == "" {
					display = dependencyName(dep)
				}
				onProgress(fmt.Sprintf("正在补全前置依赖：%s", display))
			}
			if _, err := DownloadFile(ctx, child, dir); err != nil {
				return fmt.Errorf("下载前置依赖 %s 失败: %w", dependencyName(dep), err)
			}
			installed = append(installed, child.Name)
			if err := walk(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(parent, 0); err != nil {
		return installed, err
	}
	return installed, nil
}

func dependencyName(dep Dependency) string {
	if dep.FileName != "" {
		return dep.FileName
	}
	if dep.ProjectID != "" {
		return dep.ProjectID
	}
	return dep.VersionID
}

// DependenciesOf resolves the direct dependencies of a version for display.
func DependenciesOf(ctx context.Context, v Version) []Dependency {
	return v.Dependencies
}

func getJSON(ctx context.Context, endpoint string, out any) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 600 * time.Millisecond):
			}
		}
		lastErr = httpx.GetJSON(ctx, endpoint, out)
		if lastErr == nil {
			return nil
		}
		if strings.Contains(lastErr.Error(), "HTTP 404") {
			return fmt.Errorf("未找到项目或版本")
		}
	}
	return lastErr
}

// InstallPath maps a project type to an instance subdirectory.
func InstallPath(projectType string) (string, error) {
	switch projectType {
	case TypeMod:
		return "mods", nil
	case TypeShader:
		return "shaderpacks", nil
	case TypeResourcepack:
		return "resourcepacks", nil
	case "datapack":
		return "datapacks", nil
	default:
		return "", fmt.Errorf("不支持的项目类型: %s", projectType)
	}
}

// NormalizeProjectType maps UI tab values.
func NormalizeProjectType(value string) string {
	switch value {
	case "shaders":
		return TypeShader
	case "resourcepacks":
		return TypeResourcepack
	case "modpacks":
		return TypeModpack
	default:
		return value
	}
}

// ExistsInDir reports whether a downloaded file already exists.
func ExistsInDir(dir, filename string) bool {
	return util.NonEmptyFile(filepath.Join(dir, filepath.Base(filename)))
}
