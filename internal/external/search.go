package external

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"MyMinecraftGoLuncher/internal/util"
)

const defaultSearchDepth = 5

// SearchProgress is called for each directory visited during a folder search.
type SearchProgress func(directory string, found int)

// SearchMinecraftFolders walks root (bounded depth) looking for .minecraft or
// versions folders and scans every game root it finds. Heavy or system
// directories are skipped so searching a game drive stays reasonably fast.
func SearchMinecraftFolders(ctx context.Context, root string, progress SearchProgress) ([]ScanResult, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("请选择要搜索的目录")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() {
		return nil, errors.New("搜索目录不存在或不是文件夹")
	}

	seen := map[string]bool{}
	results := make([]ScanResult, 0)
	visited := 0
	const maxResults = 30
	const maxVisited = 100000

	addScan := func(path string) bool {
		key := filepath.Clean(path)
		if seen[strings.ToLower(key)] {
			return false
		}
		scan, err := Scan(path)
		if err != nil || len(scan.Candidates) == 0 {
			return false
		}
		seen[strings.ToLower(key)] = true
		results = append(results, scan)
		return true
	}

	// The selected folder may itself be a game, versions or version folder.
	if addScan(absolute) {
		return results, nil
	}

	var walk func(directory string, depth int) error
	walk = func(directory string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > maxVisited {
			return errors.New("搜索范围过大，请选择更具体的目录")
		}
		if progress != nil {
			progress(directory, len(results))
		}
		if depth >= defaultSearchDepth || len(results) >= maxResults {
			return nil
		}
		entries, err := os.ReadDir(directory)
		if err != nil {
			return nil // Unreadable/system folders are not fatal.
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			child := filepath.Join(directory, entry.Name())
			lower := strings.ToLower(entry.Name())

			// Direct candidates. Scan already understands .minecraft,
			// versions, Prism instances and single version folders.
			direct := lower == ".minecraft" || lower == "versions" || lower == "instances" || lower == "profiles" ||
				util.Exists(filepath.Join(child, "mmc-pack.json")) ||
				util.Exists(filepath.Join(child, "profile.json")) ||
				util.Exists(filepath.Join(child, "minecraftinstance.json")) ||
				util.Exists(filepath.Join(child, "instance.json")) ||
				util.Exists(filepath.Join(child, "versions"))
			if direct {
				if addScan(child) {
					if len(results) >= maxResults {
						return nil
					}
					continue
				}
				// A game root without importable versions is not useful to
				// descend into, but its sibling folders may still matter.
				continue
			}
			if skipSearchDir(lower) {
				continue
			}
			if err := walk(child, depth+1); err != nil {
				return err
			}
			if len(results) >= maxResults {
				return nil
			}
		}
		return nil
	}

	err = walk(absolute, 0)
	if err != nil {
		if len(results) > 0 && errors.Is(err, context.Canceled) {
			return results, nil
		}
		if len(results) > 0 && errors.Is(err, context.DeadlineExceeded) {
			return results, nil
		}
		return results, err
	}
	return results, nil
}

func skipSearchDir(name string) bool {
	switch name {
	case "windows", "program files", "program files (x86)", "programdata",
		"$recycle.bin", "system volume information", "recovery", "perflogs",
		"node_modules", ".git", ".svn", ".hg", "__pycache__", "venv", ".venv",
		"cache", "temp", "tmp", "logs", "libraries", "assets", "runtimes",
		"saves", "mods", "resourcepacks", "shaderpacks", "crash-reports",
		"backups", "packages", "driverstore", "winsxs":
		return true
	}
	// Avoid scanning hundreds of nested system/cache folders.
	return strings.HasPrefix(name, "windows.") || strings.HasPrefix(name, "$")
}
