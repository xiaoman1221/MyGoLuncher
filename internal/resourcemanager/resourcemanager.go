// Package resourcemanager lists and toggles mods, resource packs and shader
// packs inside one isolated game instance.
package resourcemanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	KindMods          = "mods"
	KindResourcePacks = "resourcepacks"
	KindShaders       = "shaderpacks"
	disabledSuffix    = ".disabled"
)

// Item is one resource entry in a game instance.
type Item struct {
	Kind       string `json:"kind"`
	FileName   string `json:"fileName"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	Directory  bool   `json:"directory"`
	Size       int64  `json:"size"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

// Inventory groups all configurable resources for an instance.
type Inventory struct {
	Mods          []Item `json:"mods"`
	ResourcePacks []Item `json:"resourcePacks"`
	Shaders       []Item `json:"shaders"`
}

func folder(kind string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case KindMods:
		return KindMods, true
	case KindResourcePacks:
		return KindResourcePacks, true
	case KindShaders:
		return KindShaders, true
	default:
		return "", false
	}
}

// List scans the standard resource folders of gameDir. Missing directories
// produce empty lists rather than an error.
func List(gameDir string) Inventory {
	gameDir = strings.TrimSpace(gameDir)
	inventory := Inventory{Mods: []Item{}, ResourcePacks: []Item{}, Shaders: []Item{}}
	if gameDir == "" {
		return inventory
	}
	inventory.Mods = listDir(filepath.Join(gameDir, KindMods), KindMods)
	inventory.ResourcePacks = listDir(filepath.Join(gameDir, KindResourcePacks), KindResourcePacks)
	inventory.Shaders = listDir(filepath.Join(gameDir, KindShaders), KindShaders)
	return inventory
}

func listDir(dir, kind string) []Item {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Item{}
	}
	items := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		enabled := !strings.HasSuffix(strings.ToLower(name), disabledSuffix)
		display := name
		if !enabled {
			display = name[:len(name)-len(disabledSuffix)]
		}
		if display == "" {
			continue
		}
		lower := strings.ToLower(display)
		if kind == KindMods {
			if !strings.HasSuffix(lower, ".jar") {
				continue
			}
		} else if !entry.IsDir() && !strings.HasSuffix(lower, ".zip") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		item := Item{
			Kind:      kind,
			FileName:  name,
			Name:      display,
			Enabled:   enabled,
			Directory: entry.IsDir(),
			Size:      info.Size(),
		}
		if !info.ModTime().IsZero() {
			item.ModifiedAt = info.ModTime().Format(time.RFC3339)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		left, right := strings.ToLower(items[i].Name), strings.ToLower(items[j].Name)
		if left == right {
			return items[i].FileName < items[j].FileName
		}
		return left < right
	})
	return items
}

// SetEnabled renames one resource so Minecraft will include or ignore it.
// Disabled resources use a .disabled suffix, which is ignored by Fabric,
// Forge, Iris and the vanilla resource pack picker.
func SetEnabled(gameDir, kind, fileName string, enabled bool) error {
	gameDir = strings.TrimSpace(gameDir)
	if gameDir == "" {
		return errors.New("游戏目录为空")
	}
	folderName, ok := folder(kind)
	if !ok {
		return errors.New("不支持的资源类型")
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" || fileName != filepath.Base(fileName) || fileName == "." || fileName == ".." {
		return errors.New("资源文件名无效")
	}
	dir := filepath.Join(gameDir, folderName)
	source := filepath.Join(dir, fileName)
	info, err := os.Stat(source)
	if err != nil || (!info.Mode().IsRegular() && !info.IsDir()) {
		return errors.New("资源不存在")
	}
	lower := strings.ToLower(fileName)
	disabled := strings.HasSuffix(lower, disabledSuffix)
	target := fileName
	if enabled {
		if !disabled {
			return nil
		}
		target = fileName[:len(fileName)-len(disabledSuffix)]
	} else {
		if disabled {
			return nil
		}
		target = fileName + disabledSuffix
	}
	if target == "" || target == "." || target == ".." {
		return errors.New("资源文件名无效")
	}
	destination := filepath.Join(dir, target)
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("目标文件已存在: %s", target)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(source, destination)
}

// Counts returns enabled mods, resource packs and shaders.
func Counts(inventory Inventory) (mods, resourcePacks, shaders int) {
	for _, item := range inventory.Mods {
		if item.Enabled {
			mods++
		}
	}
	for _, item := range inventory.ResourcePacks {
		if item.Enabled {
			resourcePacks++
		}
	}
	for _, item := range inventory.Shaders {
		if item.Enabled {
			shaders++
		}
	}
	return mods, resourcePacks, shaders
}
