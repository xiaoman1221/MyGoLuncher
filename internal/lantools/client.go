package lantools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var client = &http.Client{Timeout: 30 * time.Minute}

func endpoint(target Target, path string, query url.Values) string {
	host := target.Address
	port := target.Port
	u := url.URL{
		Scheme:   "http",
		Host:     net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		Path:     path,
		RawQuery: query.Encode(),
	}
	return u.String()
}

// SendChat sends one chat message to a peer.
func SendChat(ctx context.Context, target Target, fromID, fromName, text string) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("聊天内容为空")
	}
	body, err := json.Marshal(map[string]string{
		"from":     fromID,
		"fromName": fromName,
		"text":     text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/chat", nil), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, target)
}

// SendChatMessage sends a chat message scoped to a group or private chat.
func SendChatMessage(ctx context.Context, target Target, fromID, fromName, chatID, chatName string, members []string, text string) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("聊天内容为空")
	}
	body, err := json.Marshal(map[string]any{
		"from": fromID, "fromName": fromName, "chatId": chatID,
		"chatName": chatName, "members": members, "text": text,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/chat", nil), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, target)
}

// SendSignal sends one WebRTC signaling payload to a peer.
func SendSignal(ctx context.Context, target Target, fromID, fromName, session, kind, payload string) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	if len(payload) > MaxSignalBytes {
		return errors.New("信令数据过大")
	}
	body, err := json.Marshal(map[string]string{
		"from": fromID, "fromName": fromName, "session": session, "kind": kind, "payload": payload,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/signal", nil), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, target)
}

// SendFileOffer tells a peer about a file exposed by the local /share endpoint.
func SendFileOffer(ctx context.Context, target Target, fromID, fromName, fileID, name, chatID, chatName string, members []string, size int64) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	body, err := json.Marshal(map[string]any{
		"from": fromID, "fromName": fromName, "fileId": fileID, "name": name,
		"chatId": chatID, "chatName": chatName, "members": members, "size": size,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/file-offer", nil), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return doJSON(req, target)
}

// SendVoice sends a recorded voice clip to a peer.
func SendVoice(ctx context.Context, target Target, fromID, fromName, mime string, data []byte, durationMS int) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	if len(data) == 0 {
		return errors.New("语音内容为空")
	}
	if len(data) > MaxVoiceBytes {
		return errors.New("语音文件过大")
	}
	query := url.Values{}
	query.Set("from", fromID)
	query.Set("fromName", fromName)
	query.Set("duration", fmt.Sprintf("%d", durationMS))
	query.Set("mime", mime)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/voice", query), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mime)
	return doJSON(req, target)
}

// DownloadProgress reports 0-100 while a shared file is being fetched.
type DownloadProgress func(percent int)

// DownloadSharedFile downloads a peer file with multiple HTTP Range workers.
func DownloadSharedFile(ctx context.Context, target Target, fileID, dest string, size int64, workers int, progress DownloadProgress) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	if strings.TrimSpace(fileID) == "" || size <= 0 || size > MaxFileBytes {
		return errors.New("共享文件信息无效")
	}
	if workers < 1 {
		workers = 4
	}
	if workers > 8 {
		workers = 8
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), ".mgl-download-*.part")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Truncate(size); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	chunk := (size + int64(workers) - 1) / int64(workers)
	var wg sync.WaitGroup
	var written int64
	var firstErr error
	var errMu sync.Mutex
	setErr := func(err error) {
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errMu.Unlock()
	}
	for index := 0; index < workers; index++ {
		start := int64(index) * chunk
		if start >= size {
			break
		}
		end := start + chunk - 1
		if end >= size {
			end = size - 1
		}
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			query := url.Values{"id": {fileID}}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint(target, "/share", query), nil)
			if err != nil {
				setErr(err)
				return
			}
			req.Header.Set(tokenHeader, target.Token)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
			resp, err := client.Do(req)
			if err != nil {
				setErr(err)
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusPartialContent {
				setErr(fmt.Errorf("对方不支持分块下载 (%d)", resp.StatusCode))
				return
			}
			file, err := os.OpenFile(tempName, os.O_WRONLY, 0o644)
			if err != nil {
				setErr(err)
				return
			}
			_, copyErr := io.Copy(io.NewOffsetWriter(file, start), io.LimitReader(resp.Body, end-start+1))
			closeErr := file.Close()
			if copyErr != nil {
				setErr(copyErr)
				return
			}
			if closeErr != nil {
				setErr(closeErr)
				return
			}
			current := atomic.AddInt64(&written, end-start+1)
			if progress != nil {
				progress(int(current * 100 / size))
			}
		}(start, end)
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	if err := os.Rename(tempName, dest); err != nil {
		return err
	}
	if progress != nil {
		progress(100)
	}
	return nil
}

// SendFile streams a file to a peer without loading it into memory.
func SendFile(ctx context.Context, target Target, fromID, fromName, path string) error {
	if !target.Valid() {
		return errors.New("对方未开启局域网工具")
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		return errors.New("要发送的文件不存在")
	}
	if stat.Size() > MaxFileBytes {
		return errors.New("文件超过 512 MB 限制")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	query := url.Values{}
	query.Set("from", fromID)
	query.Set("fromName", fromName)
	query.Set("name", filepathBase(path))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(target, "/file", query), file)
	if err != nil {
		return err
	}
	req.ContentLength = stat.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	return doJSON(req, target)
}

func doJSON(req *http.Request, target Target) error {
	req.Header.Set(tokenHeader, target.Token)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("连接对方失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("对方拒绝请求: %s", message)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func filepathBase(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		value = value[index+1:]
	}
	if value == "" {
		return "file"
	}
	return value
}
