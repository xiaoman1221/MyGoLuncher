package lan

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

const maxAnnouncementSize = 4096

var (
	motdPattern = regexp.MustCompile(`\[MOTD\](.*?)\[/MOTD\]`)
	adPattern   = regexp.MustCompile(`\[AD\](\d{1,5})\[/AD\]`)
	colorCode   = regexp.MustCompile(`(?i)§[0-9a-fk-or]`)
)

type announcement struct {
	MOTD string
	Port int
}

// parseAnnouncement parses Minecraft's Open to LAN broadcast payload:
// [MOTD]...[/MOTD][AD]port[/AD]
func parseAnnouncement(payload []byte) (announcement, error) {
	if len(payload) == 0 || len(payload) > maxAnnouncementSize {
		return announcement{}, errors.New("局域网广播长度无效")
	}
	text := string(payload)
	ad := adPattern.FindStringSubmatch(text)
	if len(ad) != 2 {
		return announcement{}, errors.New("局域网广播缺少端口")
	}
	port, err := strconv.Atoi(ad[1])
	if err != nil || port < 1 || port > 65535 {
		return announcement{}, errors.New("局域网广播端口无效")
	}
	motd := ""
	if match := motdPattern.FindStringSubmatch(text); len(match) == 2 {
		motd = cleanMOTD(match[1])
	}
	return announcement{MOTD: motd, Port: port}, nil
}

func cleanMOTD(text string) string {
	text = colorCode.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	if len([]rune(text)) > 120 {
		text = string([]rune(text)[:120])
	}
	return text
}
