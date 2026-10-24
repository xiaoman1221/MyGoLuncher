// Package gameoptions updates Minecraft's options.txt before launch.
package gameoptions

import (
	"os"
	"path/filepath"
	"strings"
)

// LanguageCode maps a launcher language to a Minecraft locale code.
func LanguageCode(language string) string {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "zh-cn", "zh_cn", "zh", "zh-hans":
		return "zh_cn"
	case "zh-tw", "zh_tw", "zh-hk", "zh-mo", "zh-hant":
		return "zh_tw"
	case "en", "en-us", "en_us":
		return "en_us"
	case "ja", "ja-jp", "ja_jp":
		return "ja_jp"
	case "ko", "ko-kr", "ko_kr":
		return "ko_kr"
	default:
		return ""
	}
}

// SetLanguage writes the Minecraft language into options.txt while preserving
// all other settings. It returns the applied Minecraft locale code.
func SetLanguage(gameDir, launcherLanguage string) (string, error) {
	code := LanguageCode(launcherLanguage)
	if code == "" {
		return "", nil
	}
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(gameDir, "options.txt")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	raw := string(data)
	newline := "\n"
	if strings.Contains(raw, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	replaced := false
	for idx, line := range lines {
		if strings.HasPrefix(line, "lang:") {
			lines[idx] = "lang:" + code
			replaced = true
		}
	}
	if !replaced {
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines[len(lines)-1] = "lang:" + code
			lines = append(lines, "")
		} else {
			lines = append(lines, "lang:"+code)
		}
	}
	content := strings.Join(lines, newline)
	if !strings.HasSuffix(content, newline) {
		content += newline
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return code, nil
}
