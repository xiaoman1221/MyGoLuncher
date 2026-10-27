// Package modfilter detects mods that are only useful on the client and should
// not be uploaded to a dedicated Minecraft server.
package modfilter

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

const maxMetadataSize = 2 << 20

var (
	forgeDisplayTest   = regexp.MustCompile(`(?mi)^\s*displayTest\s*=\s*["'](IGNORE_SERVER_VERSION|IGNORE_ALL_VERSION)["']`)
	forgeSideOnly      = regexp.MustCompile(`(?mi)^\s*clientSideOnly\s*=\s*true`)
	forgeClientSide    = regexp.MustCompile(`(?mi)^\s*side\s*=\s*["']CLIENT["']`)
	manifestSideOnly   = regexp.MustCompile(`(?mi)^\s*(?:modSide|fmlSide|side)\s*:\s*CLIENT\s*$`)
	manifestClientOnly = regexp.MustCompile(`(?mi)^\s*clientSideOnly\s*:\s*true\s*$`)
	mcmodSideOnly      = regexp.MustCompile(`(?i)"clientSideOnly"\s*:\s*true`)
)

// IsClientOnly inspects common mod metadata formats inside a jar. It is
// intentionally conservative: unknown or malformed metadata means the mod is
// kept, so a server pack never silently loses a server-side dependency.
func IsClientOnly(jarPath string) (bool, string) {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return false, ""
	}
	defer reader.Close()
	files := make(map[string]*zip.File, len(reader.File))
	for _, file := range reader.File {
		files[strings.ToLower(filepathToSlash(file.Name))] = file
	}

	// Fabric: "environment": "client", or a client-only entrypoint list.
	if file := files["fabric.mod.json"]; file != nil {
		var meta struct {
			Environment string                     `json:"environment"`
			Entrypoints map[string]json.RawMessage `json:"entrypoints"`
		}
		if readJSON(file, &meta) == nil {
			if strings.EqualFold(strings.TrimSpace(meta.Environment), "client") {
				return true, "fabric.mod.json environment=client"
			}
			if strings.TrimSpace(meta.Environment) == "" && hasEntrypoint(meta.Entrypoints, "client") &&
				!hasEntrypoint(meta.Entrypoints, "main") && !hasEntrypoint(meta.Entrypoints, "server") {
				return true, "fabric.mod.json has only client entrypoints"
			}
		}
	}

	// Quilt: minecraft.environment = "client", or a client-only entrypoint list.
	if file := files["quilt.mod.json"]; file != nil {
		var meta struct {
			Minecraft struct {
				Environment string `json:"environment"`
			} `json:"minecraft"`
			QuiltLoader struct {
				Entrypoints map[string]json.RawMessage `json:"entrypoints"`
			} `json:"quilt_loader"`
		}
		if readJSON(file, &meta) == nil {
			if strings.EqualFold(strings.TrimSpace(meta.Minecraft.Environment), "client") {
				return true, "quilt.mod.json environment=client"
			}
			if strings.TrimSpace(meta.Minecraft.Environment) == "" && hasEntrypoint(meta.QuiltLoader.Entrypoints, "client") &&
				!hasEntrypoint(meta.QuiltLoader.Entrypoints, "main") && !hasEntrypoint(meta.QuiltLoader.Entrypoints, "server") {
				return true, "quilt.mod.json has only client entrypoints"
			}
		}
	}

	// Forge / NeoForge: displayTest=IGNORE_SERVER_VERSION / IGNORE_ALL_VERSION
	// marks a mod that is not required on the server. clientSideOnly appears in
	// a few legacy/manual configurations.
	for _, name := range []string{"meta-inf/mods.toml", "meta-inf/neoforge.mods.toml"} {
		if file := files[name]; file != nil {
			data := readBytes(file)
			if forgeDisplayTest.Match(data) {
				return true, name + " displayTest=IGNORE_*"
			}
			if forgeSideOnly.Match(data) {
				return true, name + " clientSideOnly=true"
			}
			if forgeClientSide.Match(data) {
				return true, name + " side=CLIENT"
			}
		}
	}
	if file := files["meta-inf/manifest.mf"]; file != nil {
		data := readBytes(file)
		if manifestSideOnly.Match(data) {
			return true, "META-INF/MANIFEST.MF side=CLIENT"
		}
		if manifestClientOnly.Match(data) {
			return true, "META-INF/MANIFEST.MF clientSideOnly=true"
		}
	}
	if file := files["mcmod.info"]; file != nil {
		if mcmodSideOnly.Match(readBytes(file)) {
			return true, "mcmod.info clientSideOnly=true"
		}
	}
	return false, ""
}

func hasEntrypoint(entrypoints map[string]json.RawMessage, name string) bool {
	value, ok := entrypoints[name]
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(string(value))
	return trimmed != "" && trimmed != "null" && trimmed != "[]" && trimmed != "{}"
}

func filepathToSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

func readJSON(file *zip.File, out any) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, maxMetadataSize))
	if err != nil {
		return err
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	return json.Unmarshal(data, out)
}

func readBytes(file *zip.File) []byte {
	reader, err := file.Open()
	if err != nil {
		return nil
	}
	defer reader.Close()
	data, _ := io.ReadAll(io.LimitReader(reader, maxMetadataSize))
	return data
}

// IsModJar reports whether path looks like a mod jar.
func IsModJar(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".jar")
}
