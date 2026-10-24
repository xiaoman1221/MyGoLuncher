package curseforge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/httpx"
)

const (
	API                = "https://api.curseforge.com/v1"
	MinecraftID        = 432
	ClassMods          = 6
	ClassResourcePacks = 12
	ClassModpacks      = 4471
	ClassShaders       = 6552
	ClassDataPacks     = 6945

	relationRequired = 3
)

var ErrNoAPIKey = errors.New("未配置 CurseForge API Key")

// Client talks to the CurseForge v1 API.
type Client struct {
	APIKey  string
	baseURL string
}

// New creates a CurseForge API client.
func New(apiKey string) *Client {
	return &Client{APIKey: strings.TrimSpace(apiKey), baseURL: API}
}

// ClassIDFor maps a unified resource type to CurseForge's class id.
func ClassIDFor(resourceType string) (int, bool) {
	switch resourceType {
	case "mod":
		return ClassMods, true
	case "resourcepack":
		return ClassResourcePacks, true
	case "shader":
		return ClassShaders, true
	case "datapack":
		return ClassDataPacks, true
	case "modpack":
		return ClassModpacks, true
	default:
		return 0, false
	}
}

// TypeForClass maps a CurseForge class id back to a unified type.
func TypeForClass(classID int) string {
	switch classID {
	case ClassMods:
		return "mod"
	case ClassResourcePacks:
		return "resourcepack"
	case ClassShaders:
		return "shader"
	case ClassDataPacks:
		return "datapack"
	case ClassModpacks:
		return "modpack"
	default:
		return "mod"
	}
}

// LoaderID maps instance loader names to the API enum.
func LoaderID(loader string) (int, bool) {
	switch strings.ToLower(loader) {
	case "forge":
		return 1, true
	case "fabric":
		return 4, true
	case "quilt":
		return 5, true
	case "neoforge":
		return 6, true
	default:
		return 0, false
	}
}

// SearchRequest is a CurseForge catalog query.
type SearchRequest struct {
	Query       string
	ClassID     int
	GameVersion string
	Loader      string
	Offset      int
	Limit       int
	Sort        string
}

// SearchResult is a page of CurseForge projects.
type SearchResult struct {
	Projects []Project
	Total    int
}

// Project is a CurseForge mod/project.
type Project struct {
	ID                   int64      `json:"id"`
	GameID               int        `json:"gameId"`
	ClassID              int        `json:"classId"`
	Name                 string     `json:"name"`
	Slug                 string     `json:"slug"`
	Links                Links      `json:"links"`
	Summary              string     `json:"summary"`
	Status               int        `json:"status"`
	DownloadCount        int64      `json:"downloadCount"`
	LatestFiles          []File     `json:"latestFiles"`
	DateModified         string     `json:"dateModified"`
	DateCreated          string     `json:"dateCreated"`
	Categories           []Category `json:"categories"`
	Authors              []Author   `json:"authors"`
	Logo                 Logo       `json:"logo"`
	AllowModDistribution bool       `json:"allowModDistribution"`
}

// Links contains the CurseForge website links.
type Links struct {
	WebsiteURL string `json:"websiteUrl"`
}

// Category is a CurseForge category.
type Category struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Author is a project author.
type Author struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Logo contains project artwork.
type Logo struct {
	ThumbnailURL string `json:"thumbnailUrl"`
	URL          string `json:"url"`
}

// File is a downloadable CurseForge file.
type File struct {
	ID            int64            `json:"id"`
	ModID         int64            `json:"modId"`
	DisplayName   string           `json:"displayName"`
	FileName      string           `json:"fileName"`
	ReleaseType   int              `json:"releaseType"`
	FileStatus    int              `json:"fileStatus"`
	Hashes        []Hash           `json:"hashes"`
	FileDate      string           `json:"fileDate"`
	FileLength    int64            `json:"fileLength"`
	DownloadCount int64            `json:"downloadCount"`
	DownloadURL   string           `json:"downloadUrl"`
	GameVersions  []string         `json:"gameVersions"`
	Dependencies  []FileDependency `json:"dependencies"`
	IsServerPack  bool             `json:"isServerPack"`
}

// Hash is a file checksum.
type Hash struct {
	Value string `json:"value"`
	Algo  int    `json:"algo"` // 1 = SHA-1, 2 = MD5
}

// FileDependency is a relationship declared by a file.
type FileDependency struct {
	ModID        int64 `json:"modId"`
	FileID       int64 `json:"fileId"`
	RelationType int   `json:"relationType"`
}

// ReleaseLabel returns a short release type label.
func (f File) ReleaseLabel() string {
	switch f.ReleaseType {
	case 1:
		return "release"
	case 2:
		return "beta"
	case 3:
		return "alpha"
	default:
		return "release"
	}
}

// Compatible checks the game version in a file's gameVersions list. Loader
// filtering is requested server-side via modLoaderType; when the response
// includes loader tokens they are checked as a second line of defence.
func (f File) Compatible(gameVersion, loader string) bool {
	if gameVersion != "" && gameVersion != "all" {
		found := false
		for _, value := range f.GameVersions {
			if strings.EqualFold(value, gameVersion) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if loader != "" && loader != "all" && loader != "vanilla" {
		want := strings.ToLower(strings.ReplaceAll(loader, "-", ""))
		hasLoaderToken := false
		for _, value := range f.GameVersions {
			token := strings.ToLower(strings.ReplaceAll(value, "-", ""))
			if token == "forge" || token == "fabric" || token == "quilt" || token == "neoforge" {
				hasLoaderToken = true
				if token == want {
					return true
				}
			}
		}
		if hasLoaderToken {
			return false
		}
	}
	return true
}

// SHA1 returns the file's SHA-1 checksum.
func (f File) SHA1() string {
	for _, hash := range f.Hashes {
		if hash.Algo == 1 {
			return hash.Value
		}
	}
	return ""
}

// Search queries the CurseForge catalog.
func (c *Client) Search(ctx context.Context, req SearchRequest) (SearchResult, error) {
	if c.APIKey == "" {
		return SearchResult{}, ErrNoAPIKey
	}
	limit := req.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q := url.Values{}
	q.Set("gameId", strconv.Itoa(MinecraftID))
	if req.ClassID != 0 {
		q.Set("classId", strconv.Itoa(req.ClassID))
	}
	if strings.TrimSpace(req.Query) != "" {
		q.Set("searchFilter", strings.TrimSpace(req.Query))
	}
	if req.GameVersion != "" && req.GameVersion != "all" {
		q.Set("gameVersion", req.GameVersion)
	}
	if req.ClassID == ClassMods {
		if loaderID, ok := LoaderID(req.Loader); ok {
			q.Set("modLoaderType", strconv.Itoa(loaderID))
		}
	}
	q.Set("index", strconv.Itoa(max(req.Offset, 0)))
	q.Set("pageSize", strconv.Itoa(limit))
	q.Set("sortOrder", "desc")
	q.Set("sortField", strconv.Itoa(sortField(req.Sort)))
	var payload struct {
		Data       []Project `json:"data"`
		Pagination struct {
			Index       int `json:"index"`
			PageSize    int `json:"pageSize"`
			ResultCount int `json:"resultCount"`
			TotalCount  int `json:"totalCount"`
		} `json:"pagination"`
	}
	if err := c.getJSON(ctx, c.baseURL+"/mods/search?"+q.Encode(), &payload); err != nil {
		return SearchResult{}, err
	}
	if payload.Data == nil {
		payload.Data = []Project{}
	}
	return SearchResult{Projects: payload.Data, Total: payload.Pagination.TotalCount}, nil
}

// Files lists files for a project, optionally filtered by game version/loader.
func (c *Client) Files(ctx context.Context, projectID int64, gameVersion, loader string) ([]File, error) {
	if c.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	q := url.Values{}
	q.Set("index", "0")
	q.Set("pageSize", "50")
	if gameVersion != "" && gameVersion != "all" {
		q.Set("gameVersion", gameVersion)
	}
	if loaderID, ok := LoaderID(loader); ok {
		q.Set("modLoaderType", strconv.Itoa(loaderID))
	}
	endpoint := fmt.Sprintf("%s/mods/%d/files?%s", c.baseURL, projectID, q.Encode())
	var payload struct {
		Data []File `json:"data"`
	}
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return nil, err
	}
	return payload.Data, nil
}

// FileByID fetches one file.
func (c *Client) FileByID(ctx context.Context, projectID, fileID int64) (File, error) {
	if c.APIKey == "" {
		return File{}, ErrNoAPIKey
	}
	var payload struct {
		Data File `json:"data"`
	}
	endpoint := fmt.Sprintf("%s/mods/%d/files/%d", c.baseURL, projectID, fileID)
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return File{}, err
	}
	return payload.Data, nil
}

// Mod fetches project details.
func (c *Client) Mod(ctx context.Context, projectID int64) (Project, error) {
	if c.APIKey == "" {
		return Project{}, ErrNoAPIKey
	}
	var payload struct {
		Data Project `json:"data"`
	}
	if err := c.getJSON(ctx, fmt.Sprintf("%s/mods/%d", c.baseURL, projectID), &payload); err != nil {
		return Project{}, err
	}
	return payload.Data, nil
}

// DownloadURL resolves a file's direct download URL.
func (c *Client) DownloadURL(ctx context.Context, file File) (string, error) {
	if file.DownloadURL != "" {
		return file.DownloadURL, nil
	}
	if c.APIKey == "" {
		return "", ErrNoAPIKey
	}
	var payload struct {
		Data string `json:"data"`
	}
	endpoint := fmt.Sprintf("%s/mods/%d/files/%d/download-url", c.baseURL, file.ModID, file.ID)
	if err := c.getJSON(ctx, endpoint, &payload); err != nil {
		return "", fmt.Errorf("获取 CurseForge 下载地址失败: %w", err)
	}
	if payload.Data == "" {
		return "", errors.New("CurseForge 文件未提供下载地址（作者可能禁止第三方下载）")
	}
	return payload.Data, nil
}

// DownloadFile downloads one project file into dir and returns its path.
func (c *Client) DownloadFile(ctx context.Context, file File, dir string) (string, error) {
	return c.DownloadFileAny(ctx, file, dir, nil)
}

// DownloadFileAny downloads one project file through optional mirror
// candidates. The original URL is always the final fallback.
func (c *Client) DownloadFileAny(ctx context.Context, file File, dir string, rewriter func(string) []string) (string, error) {
	downloadURL, err := c.DownloadURL(ctx, file)
	if err != nil {
		return "", err
	}
	name := filepath.Base(strings.TrimSpace(file.FileName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = fmt.Sprintf("curseforge-%d.jar", file.ID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, name)
	urls := []string{downloadURL}
	if rewriter != nil {
		if candidates := rewriter(downloadURL); len(candidates) > 0 {
			urls = candidates
		}
	}
	if err := httpx.DownloadAny(ctx, urls, httpx.File{
		Dest: dest, SHA1: file.SHA1(), Size: file.FileLength, Verify: file.SHA1() != "",
	}); err != nil {
		return "", err
	}
	return dest, nil
}

// InstallDependencies downloads all required dependencies of a CurseForge file.
func (c *Client) InstallDependencies(ctx context.Context, parent File, gameVersion, loader, dir string, maxDepth int, onProgress func(string)) ([]string, error) {
	if maxDepth <= 0 {
		return nil, nil
	}
	visited := map[int64]bool{parent.ModID: true}
	var installed []string
	var walk func(file File, depth int) error
	walk = func(file File, depth int) error {
		if depth > maxDepth {
			return nil
		}
		for _, dep := range file.Dependencies {
			if dep.RelationType != relationRequired || dep.ModID == 0 || visited[dep.ModID] {
				continue
			}
			visited[dep.ModID] = true
			var child File
			var err error
			if dep.FileID > 0 {
				child, err = c.FileByID(ctx, dep.ModID, dep.FileID)
				if err == nil && !child.Compatible(gameVersion, loader) {
					child, err = c.latestFile(ctx, dep.ModID, gameVersion, loader)
				}
			} else {
				child, err = c.latestFile(ctx, dep.ModID, gameVersion, loader)
			}
			if err != nil {
				return fmt.Errorf("补全 CurseForge 前置依赖失败（项目 %d）: %w", dep.ModID, err)
			}
			if onProgress != nil {
				name := child.DisplayName
				if name == "" {
					name = child.FileName
				}
				onProgress(fmt.Sprintf("正在补全前置依赖：%s", name))
			}
			if _, err := c.DownloadFile(ctx, child, dir); err != nil {
				return fmt.Errorf("下载 CurseForge 前置依赖失败: %w", err)
			}
			installed = append(installed, displayName(child))
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

// FingerprintMatch is an exact CurseForge fingerprint lookup result.
type FingerprintMatch struct {
	ID          int64  `json:"id"`
	File        File   `json:"file"`
	LatestFiles []File `json:"latestFiles"`
}

// FingerprintMatches resolves installed file fingerprints. CurseForge limits
// each request, so callers should keep the batch around 100 items.
func (c *Client) FingerprintMatches(ctx context.Context, fingerprints []uint32) ([]FingerprintMatch, error) {
	if c.APIKey == "" {
		return nil, ErrNoAPIKey
	}
	if len(fingerprints) == 0 {
		return []FingerprintMatch{}, nil
	}
	var payload struct {
		Data struct {
			ExactMatches []FingerprintMatch `json:"exactMatches"`
		} `json:"data"`
	}
	body := map[string]any{"fingerprints": fingerprints}
	if err := c.postJSON(ctx, c.baseURL+"/fingerprints", body, &payload); err != nil {
		return nil, err
	}
	if payload.Data.ExactMatches == nil {
		return []FingerprintMatch{}, nil
	}
	return payload.Data.ExactMatches, nil
}

// LatestCompatibleFile returns the newest preferred release for a project.
func (c *Client) LatestCompatibleFile(ctx context.Context, projectID int64, gameVersion, loader string) (File, error) {
	return c.latestFile(ctx, projectID, gameVersion, loader)
}

func (c *Client) latestFile(ctx context.Context, projectID int64, gameVersion, loader string) (File, error) {
	files, err := c.Files(ctx, projectID, gameVersion, loader)
	if err != nil {
		return File{}, err
	}
	rank := map[int]int{1: 3, 2: 2, 3: 1}
	bestIdx := -1
	bestRank := 0
	for idx, file := range files {
		if !file.Compatible(gameVersion, loader) || file.FileName == "" {
			continue
		}
		r := rank[file.ReleaseType]
		if bestIdx < 0 || r > bestRank {
			bestIdx = idx
			bestRank = r
		}
	}
	if bestIdx >= 0 {
		return files[bestIdx], nil
	}
	for _, file := range files {
		if file.FileName != "" {
			return file, nil
		}
	}
	return File{}, errors.New("没有兼容的文件")
}

func displayName(file File) string {
	if file.DisplayName != "" {
		return file.DisplayName
	}
	return file.FileName
}

func sortField(sort string) int {
	switch sort {
	case "downloads":
		return 6
	case "newest", "updated":
		return 3
	default:
		return 2
	}
}

func (c *Client) postJSON(ctx context.Context, endpoint string, body any, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return fmt.Errorf("CurseForge 网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 300 {
			message = message[:300]
		}
		return fmt.Errorf("CurseForge API HTTP %d: %s", resp.StatusCode, message)
	}
	if err := json.Unmarshal(responseBody, out); err != nil {
		return fmt.Errorf("解析 CurseForge 响应失败: %w", err)
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", httpx.UserAgent)
	resp, err := httpx.Client.Do(req)
	if err != nil {
		return fmt.Errorf("CurseForge 网络请求失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var failure struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &failure)
		message := strings.TrimSpace(failure.Error.Message)
		if message == "" {
			message = strings.TrimSpace(string(body))
			if len(message) > 300 {
				message = message[:300]
			}
		}
		return fmt.Errorf("CurseForge API HTTP %d: %s", resp.StatusCode, message)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("解析 CurseForge 响应失败: %w", err)
	}
	return nil
}
