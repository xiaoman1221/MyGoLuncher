package mcsm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxResponseBytes = 32 << 20
	uploadChunkSize  = 2 << 20
)

type envelope struct {
	Status int             `json:"status"`
	Data   json.RawMessage `json:"data"`
	Time   int64           `json:"time"`
}

// Client is a small MCSManager panel API client.
type Client struct {
	base   *url.URL
	apiKey string
	http   *http.Client
}

// NewClient validates the panel URL and creates a client.
func NewClient(panelURL, apiKey string) (*Client, error) {
	normalized, err := normalizePanelURL(panelURL)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(normalized)
	if err != nil {
		return nil, err
	}
	return &Client{
		base:   base,
		apiKey: strings.TrimSpace(apiKey),
		http:   &http.Client{Timeout: 0},
	}, nil
}

// userInstancesPayload is the /api/auth response. permission is the panel
// role of the API key owner (10 means top-level administrator).
type userInstancesPayload struct {
	Instances  []RemoteInstance `json:"instances"`
	Permission int              `json:"permission"`
}

// UserInstances returns instances assigned to the API key's user together with
// the user's permission level. Admin API keys can legitimately have an empty
// instance list, so callers must use the permission value to decide whether to
// query the admin-wide endpoints.
func (c *Client) UserInstances(ctx context.Context) ([]RemoteInstance, int, error) {
	var payload userInstancesPayload
	if err := c.request(ctx, http.MethodGet, "/api/auth/", url.Values{"advanced": {"true"}}, nil, &payload); err != nil {
		return nil, 0, err
	}
	return payload.Instances, payload.Permission, nil
}

// ListInstances returns every instance assigned to the API key's user.
func (c *Client) ListInstances(ctx context.Context) ([]RemoteInstance, error) {
	instances, _, err := c.UserInstances(ctx)
	return instances, err
}

type instanceListEntry struct {
	InstanceUUID string `json:"instanceUuid"`
	Nickname     string `json:"nickname"`
	Status       int    `json:"status"`
	Config       struct {
		Nickname string `json:"nickname"`
	} `json:"config"`
}

func remoteInstanceFromEntry(daemonID string, entry instanceListEntry) (RemoteInstance, bool) {
	if strings.TrimSpace(entry.InstanceUUID) == "" {
		return RemoteInstance{}, false
	}
	nickname := strings.TrimSpace(entry.Nickname)
	if nickname == "" {
		nickname = strings.TrimSpace(entry.Config.Nickname)
	}
	return RemoteInstance{
		DaemonID:     strings.TrimSpace(daemonID),
		InstanceUUID: strings.TrimSpace(entry.InstanceUUID),
		Nickname:     nickname,
		Status:       entry.Status,
	}, true
}

// ListAllInstances lists instances across every daemon. The global endpoint is
// available on MCSManager 10 with an admin API key. Older panels are handled by
// the documented per-daemon /api/service/remote_service_instances endpoint.
func (c *Client) ListAllInstances(ctx context.Context) ([]RemoteInstance, error) {
	global, globalErr := c.listGlobalInstances(ctx)
	if globalErr == nil {
		return global, nil
	}
	perDaemon, daemonErr := c.listDaemonInstances(ctx)
	if daemonErr == nil {
		return perDaemon, nil
	}
	return nil, errors.Join(globalErr, daemonErr)
}

func (c *Client) listGlobalInstances(ctx context.Context) ([]RemoteInstance, error) {
	type globalGroup struct {
		Instances []instanceListEntry `json:"instances"`
		MaxPage   int                 `json:"maxPage"`
		Page      int                 `json:"page"`
	}
	var out []RemoteInstance
	seen := map[string]bool{}
	maxPage := 1
	for page := 1; page <= maxPage && page <= 20; page++ {
		var payload map[string]globalGroup
		query := url.Values{
			"page":      {strconv.Itoa(page)},
			"page_size": {"50"},
		}
		if err := c.request(ctx, http.MethodGet, "/api/service/remote_services_instances_global", query, nil, &payload); err != nil {
			return nil, err
		}
		added := 0
		for daemonID, group := range payload {
			if group.MaxPage > maxPage {
				maxPage = group.MaxPage
			}
			for _, entry := range group.Instances {
				instance, ok := remoteInstanceFromEntry(daemonID, entry)
				if !ok {
					continue
				}
				key := instanceKey(instance.DaemonID, instance.InstanceUUID)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, instance)
				added++
			}
		}
		if added == 0 {
			break
		}
	}
	return out, nil
}

func (c *Client) listDaemonInstances(ctx context.Context) ([]RemoteInstance, error) {
	var daemons []struct {
		UUID string `json:"uuid"`
	}
	if err := c.request(ctx, http.MethodGet, "/api/service/remote_services_system", nil, nil, &daemons); err != nil {
		return nil, err
	}
	var out []RemoteInstance
	seen := map[string]bool{}
	for _, daemon := range daemons {
		daemonID := strings.TrimSpace(daemon.UUID)
		if daemonID == "" {
			continue
		}
		type pagePayload struct {
			Data    []instanceListEntry `json:"data"`
			MaxPage int                 `json:"maxPage"`
		}
		maxPage := 1
		for page := 1; page <= maxPage && page <= 20; page++ {
			var payload pagePayload
			query := url.Values{
				"daemonId":  {daemonID},
				"page":      {strconv.Itoa(page)},
				"page_size": {"50"},
			}
			if err := c.request(ctx, http.MethodGet, "/api/service/remote_service_instances", query, nil, &payload); err != nil {
				return nil, err
			}
			if payload.MaxPage > maxPage {
				maxPage = payload.MaxPage
			}
			added := 0
			for _, entry := range payload.Data {
				instance, ok := remoteInstanceFromEntry(daemonID, entry)
				if !ok {
					continue
				}
				key := instanceKey(instance.DaemonID, instance.InstanceUUID)
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, instance)
				added++
			}
			if added == 0 {
				break
			}
		}
	}
	return out, nil
}

func instanceKey(daemonID, instanceUUID string) string {
	return strings.ToLower(strings.TrimSpace(daemonID)) + "\x00" +
		strings.ToLower(strings.TrimSpace(instanceUUID))
}

func findInstance(instances []RemoteInstance, daemonID, instanceUUID string) (RemoteInstance, bool) {
	want := instanceKey(daemonID, instanceUUID)
	for _, instance := range instances {
		if instanceKey(instance.DaemonID, instance.InstanceUUID) == want {
			return instance, true
		}
	}
	return RemoteInstance{}, false
}

// InstanceDetail verifies a daemon/instance binding and returns its state.
func (c *Client) InstanceDetail(ctx context.Context, daemonID, instanceUUID string) (RemoteInstance, error) {
	var payload struct {
		InstanceUUID string `json:"instanceUuid"`
		Nickname     string `json:"nickname"`
		Status       int    `json:"status"`
		Config       struct {
			Nickname string `json:"nickname"`
		} `json:"config"`
	}
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	if err := c.request(ctx, http.MethodGet, "/api/instance", query, nil, &payload); err != nil {
		return RemoteInstance{}, err
	}
	nickname := strings.TrimSpace(payload.Nickname)
	if nickname == "" {
		nickname = strings.TrimSpace(payload.Config.Nickname)
	}
	return RemoteInstance{
		DaemonID:     daemonID,
		InstanceUUID: instanceUUID,
		Nickname:     nickname,
		Status:       payload.Status,
	}, nil
}

// Status resolves one configured instance. It first uses the user's own
// instance list (the normal case), then the admin-wide list for top-level API
// keys, then the instance detail endpoint. The previous implementation only
// read /api/auth, which silently failed for admin keys that have no individually
// assigned instances.
func (c *Client) Status(ctx context.Context, daemonID, instanceUUID string) (RemoteInstance, error) {
	var lookupErr error
	triedAdminList := false
	instances, permission, err := c.UserInstances(ctx)
	if err == nil {
		if match, ok := findInstance(instances, daemonID, instanceUUID); ok {
			return match, nil
		}
		if permission >= 10 {
			triedAdminList = true
			if all, allErr := c.ListAllInstances(ctx); allErr == nil {
				if match, ok := findInstance(all, daemonID, instanceUUID); ok {
					return match, nil
				}
			} else {
				lookupErr = errors.Join(lookupErr, allErr)
			}
		}
	} else {
		lookupErr = err
	}
	if detail, detailErr := c.InstanceDetail(ctx, daemonID, instanceUUID); detailErr == nil {
		return detail, nil
	} else {
		lookupErr = errors.Join(lookupErr, detailErr)
	}
	// A key without a readable permission field may still be an admin key.
	if !triedAdminList {
		if all, allErr := c.ListAllInstances(ctx); allErr == nil {
			if match, ok := findInstance(all, daemonID, instanceUUID); ok {
				return match, nil
			}
		} else {
			lookupErr = errors.Join(lookupErr, allErr)
		}
	}
	if lookupErr != nil {
		return RemoteInstance{}, fmt.Errorf("MCSM 实例不存在，或当前 API Key 没有访问权限: %w", lookupErr)
	}
	return RemoteInstance{}, errors.New("MCSM 实例不存在，或当前 API Key 没有访问权限")
}

// Start opens the remote instance.
func (c *Client) Start(ctx context.Context, daemonID, instanceUUID string) error {
	return c.instanceAction(ctx, "open", daemonID, instanceUUID)
}

// Stop closes the remote instance.
func (c *Client) Stop(ctx context.Context, daemonID, instanceUUID string) error {
	return c.instanceAction(ctx, "stop", daemonID, instanceUUID)
}

// Restart restarts the remote instance.
func (c *Client) Restart(ctx context.Context, daemonID, instanceUUID string) error {
	return c.instanceAction(ctx, "restart", daemonID, instanceUUID)
}

func (c *Client) instanceAction(ctx context.Context, action, daemonID, instanceUUID string) error {
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	return c.request(ctx, http.MethodGet, "/api/protected_instance/"+action, query, nil, nil)
}

// ListDir lists one remote directory page by page.
func (c *Client) ListDir(ctx context.Context, daemonID, instanceUUID, target string) ([]RemoteFile, error) {
	if target == "" {
		target = "/"
	}
	var all []RemoteFile
	for page := 0; page < 200; page++ {
		var payload struct {
			Items []RemoteFile `json:"items"`
			Total int          `json:"total"`
		}
		query := url.Values{
			"daemonId":  {daemonID},
			"uuid":      {instanceUUID},
			"target":    {target},
			"page":      {strconv.Itoa(page)},
			"page_size": {"100"},
		}
		if err := c.request(ctx, http.MethodGet, "/api/files/list", query, nil, &payload); err != nil {
			return nil, err
		}
		all = append(all, payload.Items...)
		if len(payload.Items) == 0 || len(all) >= payload.Total {
			break
		}
	}
	return all, nil
}

// Mkdir creates a remote directory recursively.
func (c *Client) Mkdir(ctx context.Context, daemonID, instanceUUID, target string) error {
	body, _ := json.Marshal(map[string]string{"target": target})
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	return c.request(ctx, http.MethodPost, "/api/files/mkdir", query, body, nil)
}

// DeleteTargets removes remote files or directories.
func (c *Client) DeleteTargets(ctx context.Context, daemonID, instanceUUID string, targets []string) error {
	if len(targets) == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string][]string{"targets": targets})
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	return c.request(ctx, http.MethodDelete, "/api/files", query, body, nil)
}

// MoveTargets moves or renames remote files. Each pair is {source, target}.
func (c *Client) MoveTargets(ctx context.Context, daemonID, instanceUUID string, pairs [][2]string) error {
	if len(pairs) == 0 {
		return nil
	}
	targets := make([][2]string, 0, len(pairs))
	for _, pair := range pairs {
		if strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			continue
		}
		targets = append(targets, pair)
	}
	if len(targets) == 0 {
		return nil
	}
	body, _ := json.Marshal(map[string]any{"targets": targets})
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	return c.request(ctx, http.MethodPut, "/api/files/move", query, body, nil)
}

// Compress creates a zip archive on the remote host. type=1 compresses targets
// into source; type=0 extracts source into the first target directory.
func (c *Client) Compress(ctx context.Context, daemonID, instanceUUID, source string, targets []string, archiveType int) error {
	body, _ := json.Marshal(map[string]any{
		"source": source, "targets": targets, "type": archiveType, "code": "",
	})
	query := url.Values{"daemonId": {daemonID}, "uuid": {instanceUUID}}
	return c.request(ctx, http.MethodPost, "/api/files/compress", query, body, nil)
}

// UploadFile uploads a local file to a remote directory using MCSM's chunked
// upload flow. progress is called with bytes sent and the total file size.
func (c *Client) UploadFile(ctx context.Context, daemonID, instanceUUID, uploadDir, localPath string, progress func(sent, total int64)) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("读取本地文件失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("只能上传普通文件")
	}
	size := info.Size()

	query := url.Values{
		"daemonId":   {daemonID},
		"uuid":       {instanceUUID},
		"upload_dir": {uploadDir},
	}
	var config struct {
		Password string `json:"password"`
		Addr     string `json:"addr"`
	}
	if err := c.request(ctx, http.MethodPost, "/api/files/upload", query, nil, &config); err != nil {
		return err
	}
	if strings.TrimSpace(config.Password) == "" {
		return errors.New("MCSM 未返回上传凭证")
	}
	daemonBase, err := normalizeDaemonBase(config.Addr, c.base)
	if err != nil {
		return err
	}

	uploadID, err := c.initChunkedUpload(ctx, daemonBase, config.Password, info.Name(), size)
	if err != nil {
		if isMissingEndpoint(err) {
			// Older MCSM daemons only expose the legacy single-request upload.
			return c.uploadLegacy(ctx, daemonBase, config.Password, localPath, info, progress)
		}
		return fmt.Errorf("初始化上传失败: %w", err)
	}
	if size == 0 {
		if progress != nil {
			progress(0, 0)
		}
		return nil
	}

	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	var sent int64
	for sent < size {
		chunk := int64(uploadChunkSize)
		if remaining := size - sent; remaining < chunk {
			chunk = remaining
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", info.Name())
		if err != nil {
			return err
		}
		section := io.NewSectionReader(file, sent, chunk)
		if _, err := io.Copy(part, section); err != nil {
			return err
		}
		if err := writer.Close(); err != nil {
			return err
		}
		pieceURL := daemonBase + "/upload-piece/" + url.PathEscape(uploadID) + "?offset=" + strconv.FormatInt(sent, 10)
		if _, err := c.daemonRequest(ctx, http.MethodPost, pieceURL, &body, writer.FormDataContentType()); err != nil {
			return fmt.Errorf("上传分片失败: %w", err)
		}
		sent += chunk
		if progress != nil {
			progress(sent, size)
		}
	}
	return nil
}

func (c *Client) initChunkedUpload(ctx context.Context, daemonBase, password, filename string, size int64) (string, error) {
	initURL := daemonBase + "/upload-new/" + url.PathEscape(password)
	initQuery := url.Values{
		"filename":  {filename},
		"size":      {strconv.FormatInt(size, 10)},
		"overwrite": {"true"},
		"unzip":     {"0"},
	}
	raw, err := c.daemonRequest(ctx, http.MethodPost, initURL+"?"+initQuery.Encode(), nil, "")
	if err != nil {
		return "", err
	}
	var payload struct {
		ID   string `json:"id"`
		Data *struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &payload)
	uploadID := payload.ID
	if uploadID == "" && payload.Data != nil {
		uploadID = payload.Data.ID
	}
	if uploadID == "" {
		return "", fmt.Errorf("MCSM 上传会话无效: %s", sanitizeMessage(string(raw), c.apiKey))
	}
	return uploadID, nil
}

// uploadLegacy uploads a whole file through MCSM's pre-4.14 daemon endpoint.
func (c *Client) uploadLegacy(ctx context.Context, daemonBase, password, localPath string, info os.FileInfo, progress func(sent, total int64)) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		part, err := multipartWriter.CreateFormFile("file", info.Name())
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if closeErr := multipartWriter.Close(); err == nil {
			err = closeErr
		}
		_ = writer.CloseWithError(err)
	}()

	target := daemonBase + "/upload/" + url.PathEscape(password) + "?unzip=0"
	_, requestErr := c.daemonRequest(ctx, http.MethodPost, target, reader, multipartWriter.FormDataContentType())
	if requestErr != nil {
		_ = reader.CloseWithError(requestErr)
		return fmt.Errorf("上传文件失败: %w", requestErr)
	}
	if progress != nil {
		progress(info.Size(), info.Size())
	}
	return nil
}

func isMissingEndpoint(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "404") || strings.Contains(message, "not found") || strings.Contains(message, "找不到")
}

func (c *Client) request(ctx context.Context, method, relPath string, query url.Values, body []byte, out any) error {
	raw, err := c.doPanel(ctx, method, relPath, query, body, false)
	if err != nil && c.apiKey != "" && isAuthError(err) {
		// Older panels only accept the key as a query parameter.
		raw, err = c.doPanel(ctx, method, relPath, query, body, true)
	}
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("解析 MCSM 响应失败: %w", err)
	}
	return nil
}

func (c *Client) doPanel(ctx context.Context, method, relPath string, query url.Values, body []byte, queryAuth bool) ([]byte, error) {
	u := *c.base
	u.Path = joinURLPath(c.base.Path, relPath)
	values := url.Values{}
	for key, list := range query {
		values[key] = append([]string(nil), list...)
	}
	if queryAuth && c.apiKey != "" {
		values.Set("apikey", c.apiKey)
	}
	u.RawQuery = values.Encode()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	// MCSM's API documentation requires this content type even for GET calls.
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	if !queryAuth && c.apiKey != "" {
		req.Header.Set("X-Request-Api-Key", c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 MCSM 面板失败: %s", sanitizeMessage(err.Error(), c.apiKey))
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("MCSM 请求失败 (%d): %s", resp.StatusCode, sanitizeMessage(extractError(raw), c.apiKey))
	}
	data, status, msg := parseEnvelope(raw)
	if status != 0 && status != http.StatusOK {
		return nil, fmt.Errorf("MCSM 返回错误: %s", sanitizeMessage(msg, c.apiKey))
	}
	return data, nil
}

func (c *Client) daemonRequest(ctx context.Context, method, rawURL string, body io.Reader, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 MCSM 节点失败: %w", err)
	}
	defer resp.Body.Close()
	raw, err := readLimited(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("MCSM 节点返回错误 (%d): %s", resp.StatusCode, sanitizeMessage(extractError(raw), c.apiKey))
	}
	return raw, nil
}

func readLimited(reader io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, errors.New("MCSM 响应过大")
	}
	return raw, nil
}

func parseEnvelope(raw []byte) ([]byte, int, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, 0, ""
	}
	if trimmed[0] != '{' {
		return trimmed, 0, ""
	}
	var env envelope
	if err := json.Unmarshal(trimmed, &env); err != nil {
		return trimmed, 0, ""
	}
	if env.Status == 0 && len(env.Data) == 0 {
		// A plain JSON object that is not an MCSM envelope.
		return trimmed, 0, ""
	}
	msg := ""
	if len(env.Data) > 0 {
		msg = extractError(env.Data)
	}
	return env.Data, env.Status, msg
}

func extractError(raw []byte) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return text
	}
	var obj struct {
		Err     string `json:"err"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Err != "" {
			return obj.Err
		}
		if obj.Message != "" {
			return obj.Message
		}
	}
	text = strings.TrimSpace(string(raw))
	if len([]rune(text)) > 500 {
		text = string([]rune(text)[:500])
	}
	return text
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{
		"api key", "apikey", "api-key", "disabled", "forbidden",
		"permission", "unauthorized", "权限", "未授权", "401", "403",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

func sanitizeMessage(message, apiKey string) string {
	message = strings.TrimSpace(message)
	if apiKey != "" {
		message = strings.ReplaceAll(message, apiKey, "<redacted>")
	}
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500]) + "…"
	}
	return message
}

func joinURLPath(basePath, relPath string) string {
	basePath = strings.TrimRight(basePath, "/")
	if !strings.HasPrefix(relPath, "/") {
		relPath = "/" + relPath
	}
	return basePath + relPath
}

func normalizeDaemonBase(raw string, panel *url.URL) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("MCSM 未返回上传节点地址")
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "ws://") {
		raw = "http://" + raw[5:]
	} else if strings.HasPrefix(lower, "wss://") {
		raw = "https://" + raw[6:]
	} else if !strings.Contains(raw, "://") {
		raw = panel.Scheme + "://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("MCSM 上传节点地址无效")
	}
	host := u.Hostname()
	if isLoopbackHost(host) && !isLoopbackHost(panel.Hostname()) {
		port := u.Port()
		if port == "" {
			u.Host = panel.Hostname()
		} else {
			u.Host = net.JoinHostPort(panel.Hostname(), port)
		}
	}
	// Proxy mode returns the panel's own host without a path. Preserve a panel
	// installed under a sub-path (for example https://example.com/mcsm).
	if u.Path == "" && panel.Path != "" && strings.EqualFold(u.Host, panel.Host) {
		u.Path = panel.Path
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return strings.TrimRight(u.String(), "/"), nil
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0" || host == "::1" || host == "[::1]"
}

// UploadTimeout returns a context suitable for a single file upload.
func UploadTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 10*time.Minute)
}
