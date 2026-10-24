package launcher

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9._-]*[A-Za-z0-9])?$`)

// ServerAddress is a normalized Minecraft server endpoint used for direct
// connect launches.
type ServerAddress struct {
	Host string
	Port int
}

// ParseServerAddress accepts host, host:port, [ipv6]:port and bare IPv6
// literals. A missing port defaults to 25565.
func ParseServerAddress(raw string) (*ServerAddress, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("服务器地址不能为空")
	}
	if len(raw) > 255 {
		return nil, errors.New("服务器地址过长")
	}
	for _, r := range raw {
		if r <= ' ' || r == 0x7f {
			return nil, errors.New("服务器地址包含空白或控制字符")
		}
	}
	if strings.ContainsAny(raw, "/\\@?#") {
		return nil, errors.New("服务器地址包含非法字符")
	}

	host := raw
	port := 25565
	if strings.HasPrefix(raw, "[") {
		parsedHost, parsedPort, err := net.SplitHostPort(raw)
		if err != nil {
			return nil, errors.New("IPv6 地址格式无效，应写成 [地址]:端口")
		}
		host, port, err = validateHostPort(parsedHost, parsedPort)
		if err != nil {
			return nil, err
		}
	} else if strings.Count(raw, ":") == 1 {
		parsedHost, parsedPort, err := net.SplitHostPort(raw)
		if err != nil {
			// Accept a trailing colon as the default port.
			if strings.HasSuffix(raw, ":") {
				parsedHost = strings.TrimSuffix(raw, ":")
				parsedPort = ""
			} else {
				return nil, errors.New("服务器地址格式无效，应为 主机:端口")
			}
		}
		host, port, err = validateHostPort(parsedHost, parsedPort)
		if err != nil {
			return nil, err
		}
	} else if strings.Count(raw, ":") > 1 {
		// Bare IPv6 literal without a port.
		host = strings.Trim(raw, "[]")
		if net.ParseIP(stripZone(host)) == nil {
			return nil, errors.New("IPv6 地址格式无效")
		}
	}
	if err := validateHost(host); err != nil {
		return nil, err
	}
	return &ServerAddress{Host: host, Port: port}, nil
}

// String returns a host:port form suitable for --quickPlayMultiplayer.
func (s *ServerAddress) String() string {
	host := strings.Trim(s.Host, "[]")
	if strings.Contains(host, ":") {
		return net.JoinHostPort(host, strconv.Itoa(s.Port))
	}
	return host + ":" + strconv.Itoa(s.Port)
}

// HostArg returns the host component for legacy --server arguments.
func (s *ServerAddress) HostArg() string {
	return strings.Trim(s.Host, "[]")
}

func validateHostPort(host, port string) (string, int, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", 0, errors.New("服务器地址缺少主机名")
	}
	value := 25565
	if strings.TrimSpace(port) != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil || parsed < 1 || parsed > 65535 {
			return "", 0, errors.New("服务器端口必须在 1-65535 之间")
		}
		value = parsed
	}
	if err := validateHost(host); err != nil {
		return "", 0, err
	}
	return host, value, nil
}

func validateHost(host string) error {
	host = strings.Trim(host, "[]")
	if host == "" {
		return errors.New("服务器地址缺少主机名")
	}
	if len(host) > 253 {
		return errors.New("服务器主机名过长")
	}
	if net.ParseIP(stripZone(host)) != nil {
		return nil
	}
	if strings.Count(host, ".") > 0 {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || !hostnamePattern.MatchString(label) {
				return fmt.Errorf("服务器主机名无效: %s", host)
			}
		}
		return nil
	}
	if !hostnamePattern.MatchString(host) {
		return fmt.Errorf("服务器主机名无效: %s", host)
	}
	return nil
}

func stripZone(host string) string {
	if idx := strings.Index(host, "%"); idx >= 0 {
		return host[:idx]
	}
	return host
}
