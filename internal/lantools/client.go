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
	"strings"
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
