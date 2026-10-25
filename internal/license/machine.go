// Package license implements LicensingGo activation and machine-code handling
// for MyGoLuncher.
package license

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
)

// Components is the raw, un-normalized set of machine identifiers.
type Components map[string]string

// Normalize applies the machine code convention v1 rules.
func Normalize(raw Components) Components {
	out := make(Components, len(raw))
	for key, value := range raw {
		var normalized string
		switch key {
		case "mac":
			normalized = strings.ToUpper(strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(value))
		case "arch":
			normalized = normalizeArch(value)
		case "cpu":
			normalized = strings.ToLower(strings.Join(strings.Fields(value), " "))
		case "disk":
			normalized = strings.ToLower(strings.NewReplacer(":", "", "-", "", " ", "").Replace(strings.TrimSpace(value)))
		case "host", "os", "board", "machine_guid":
			normalized = strings.ToLower(strings.TrimSpace(value))
		default:
			normalized = strings.TrimSpace(value)
		}
		if normalized != "" {
			out[key] = normalized
		}
	}
	return out
}

func normalizeArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "amd64", "x64":
		return "x86_64"
	case "aarch64", "arm64":
		return "arm64"
	case "i386", "i686", "x86", "386":
		return "386"
	case "armv7l", "armv6l", "arm":
		return "arm"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// Canonical builds the canonical machine-code string (lgmc1).
func Canonical(components Components) string {
	keys := make([]string, 0, len(components))
	for key, value := range components {
		if value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	var builder strings.Builder
	builder.WriteString("lgmc1\n")
	for _, key := range keys {
		builder.WriteString(key)
		builder.WriteByte('=')
		builder.WriteString(components[key])
		builder.WriteByte('\n')
	}
	return builder.String()
}

// MachineCode computes the formatted machine code.
func MachineCode(raw Components) string {
	sum := sha256.Sum256([]byte(Canonical(Normalize(raw))))
	value := strings.ToUpper(hex.EncodeToString(sum[:8]))
	return fmt.Sprintf("%s-%s-%s-%s", value[0:4], value[4:8], value[8:12], value[12:16])
}

// Collect gathers the portable baseline components: host, os, arch and the
// platform machine identifier. All launcher builds must use this same set.
func Collect() Components {
	components := Components{
		"host":         "",
		"os":           normalizeOS(runtime.GOOS),
		"arch":         runtime.GOARCH,
		"machine_guid": platformMachineGUID(),
	}
	if host, err := os.Hostname(); err == nil {
		components["host"] = host
	}
	return components
}

func normalizeOS(goos string) string {
	switch goos {
	case "windows", "linux", "darwin", "freebsd":
		return goos
	default:
		return "other"
	}
}
