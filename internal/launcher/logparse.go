package launcher

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var log4jEvent = regexp.MustCompile(`^<log4j:Event\s+logger="([^"]*)"\s+timestamp="([^"]*)"\s+level="([^"]*)"\s+thread="([^"]*)"`)

// logParser turns Mojang's LegacyXMLLayout console output into readable log
// lines while leaving ordinary output untouched.
type logParser struct {
	active bool
	buffer []string
}

func (p *logParser) Feed(line string) []string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "<log4j:Event") {
		var out []string
		if p.active {
			out = append(out, p.flushRaw()...)
		}
		p.active = true
		p.buffer = []string{trimmed}
		if strings.Contains(trimmed, "</log4j:Event>") {
			out = append(out, p.flushRaw()...)
		}
		return out
	}
	if p.active {
		p.buffer = append(p.buffer, trimmed)
		if len(p.buffer) > 400 {
			p.buffer = p.buffer[len(p.buffer)-400:]
		}
		if strings.Contains(trimmed, "</log4j:Event>") {
			return p.flushRaw()
		}
		return nil
	}
	return []string{line}
}

func (p *logParser) Flush() []string {
	if !p.active {
		return nil
	}
	return p.flushRaw()
}

func (p *logParser) flushRaw() []string {
	p.active = false
	if len(p.buffer) == 0 {
		return nil
	}
	raw := strings.Join(p.buffer, "\n")
	p.buffer = nil
	message := extractCDATA(raw, "log4j:Message")
	throwable := extractCDATA(raw, "log4j:Throwable")
	match := log4jEvent.FindStringSubmatch(raw)
	if match == nil || (message == "" && throwable == "") {
		return strings.Split(raw, "\n")
	}
	level := strings.ToUpper(match[3])
	thread := match[4]
	stamp := ""
	if ms, err := strconv.ParseInt(match[2], 10, 64); err == nil {
		stamp = time.UnixMilli(ms).Format("15:04:05")
	}
	prefix := ""
	if stamp != "" {
		prefix = "[" + stamp + "] "
	}
	prefix += "[" + thread + "/" + level + "] "
	var out []string
	if message != "" {
		for _, line := range strings.Split(message, "\n") {
			out = append(out, prefix+line)
		}
	}
	if throwable != "" {
		if len(out) == 0 {
			out = append(out, prefix+strings.SplitN(throwable, "\n", 2)[0])
			if idx := strings.Index(throwable, "\n"); idx >= 0 {
				throwable = throwable[idx+1:]
			} else {
				throwable = ""
			}
		}
		for _, line := range strings.Split(strings.TrimRight(throwable, "\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			out = append(out, "    "+line)
		}
	}
	if len(out) == 0 {
		out = append(out, prefix+"(empty log event)")
	}
	return out
}

func extractCDATA(raw, element string) string {
	open := "<" + element + "><![CDATA["
	start := strings.Index(raw, open)
	if start < 0 {
		return ""
	}
	start += len(open)
	end := strings.Index(raw[start:], "]]></"+element+">")
	if end < 0 {
		return ""
	}
	return raw[start : start+end]
}
