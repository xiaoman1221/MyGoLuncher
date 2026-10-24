package mcsm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"MyMinecraftGoLuncher/internal/modfilter"
)

type localFile struct {
	path string
	size int64
}

// ScanFiles lists the files that a full sync would consider. Client-only mods
// are included in the result with ClientOnly=true so the UI can explain why
// they are not selected; Sync skips them automatically.
func ScanFiles(localDir string, dirs []string) ([]SyncFile, error) {
	localDir = strings.TrimSpace(localDir)
	if localDir == "" {
		return nil, errors.New("本地游戏目录为空")
	}
	info, err := os.Stat(localDir)
	if err != nil || !info.IsDir() {
		return nil, errors.New("本地游戏目录不存在")
	}
	dirs, err = normalizeSyncDirs(dirs)
	if err != nil {
		return nil, err
	}
	out := make([]SyncFile, 0, 64)
	for _, dir := range dirs {
		root := filepath.Join(localDir, dir)
		if stat, statErr := os.Stat(root); statErr != nil || !stat.IsDir() {
			continue
		}
		walkErr := filepath.WalkDir(root, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || skipSyncName(entry.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(root, filePath)
			if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return nil
			}
			stat, statErr := entry.Info()
			if statErr != nil {
				return statErr
			}
			clientOnly, reason := clientOnlyMod(dir, filePath)
			out = append(out, SyncFile{
				Path:       path.Join(dir, filepath.ToSlash(rel)),
				Dir:        dir,
				Size:       stat.Size(),
				ClientOnly: clientOnly,
				Reason:     reason,
			})
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("扫描 %s 失败: %w", dir, walkErr)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func clientOnlyMod(dir, filePath string) (bool, string) {
	if dir != DirMods || !modfilter.IsModJar(filePath) {
		return false, ""
	}
	return modfilter.IsClientOnly(filePath)
}

func collectSyncFiles(opts SyncOptions, dirs []string) (map[string]localFile, []string, int, error) {
	localFiles := map[string]localFile{}
	presentDirs := make([]string, 0, len(dirs))
	filtered := 0
	if len(opts.Files) > 0 {
		dirSet := map[string]bool{}
		for _, dir := range dirs {
			dirSet[dir] = true
		}
		for _, raw := range opts.Files {
			rel := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
			rel = path.Clean(rel)
			if rel == "" || rel == "." || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "/") {
				return nil, nil, 0, fmt.Errorf("文件路径无效: %s", raw)
			}
			parts := strings.SplitN(rel, "/", 2)
			dir := parts[0]
			if !dirSet[dir] {
				return nil, nil, 0, fmt.Errorf("文件不在所选同步目录中: %s", raw)
			}
			fullPath := filepath.Join(opts.LocalDir, filepath.FromSlash(rel))
			info, err := os.Lstat(fullPath)
			if err != nil || !info.Mode().IsRegular() {
				return nil, nil, 0, fmt.Errorf("本地文件不存在: %s", raw)
			}
			if skipSyncName(info.Name()) {
				continue
			}
			if clientOnly, _ := clientOnlyMod(dir, fullPath); clientOnly {
				filtered++
				continue
			}
			remotePath := path.Join(opts.RemoteRoot, rel)
			localFiles[remotePath] = localFile{path: fullPath, size: info.Size()}
			dirSet[dir] = true
		}
		for _, dir := range dirs {
			if dirSet[dir] {
				presentDirs = append(presentDirs, dir)
			}
		}
		if len(localFiles) == 0 {
			return nil, nil, filtered, errors.New("没有可同步的文件（客户端模组会被自动跳过）")
		}
		return localFiles, presentDirs, filtered, nil
	}

	for _, dir := range dirs {
		localDir := filepath.Join(opts.LocalDir, dir)
		if stat, statErr := os.Stat(localDir); statErr != nil || !stat.IsDir() {
			continue
		}
		presentDirs = append(presentDirs, dir)
		err := filepath.WalkDir(localDir, func(filePath string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() || skipSyncName(entry.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(localDir, filePath)
			if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return nil
			}
			if clientOnly, _ := clientOnlyMod(dir, filePath); clientOnly {
				filtered++
				return nil
			}
			stat, statErr := entry.Info()
			if statErr != nil {
				return statErr
			}
			remotePath := path.Join(opts.RemoteRoot, dir, filepath.ToSlash(rel))
			localFiles[remotePath] = localFile{path: filePath, size: stat.Size()}
			return nil
		})
		if err != nil {
			return nil, nil, filtered, fmt.Errorf("扫描 %s 失败: %w", dir, err)
		}
	}
	return localFiles, presentDirs, filtered, nil
}

// Sync incrementally uploads selected local directories into a remote MCSM
// instance. Files are compared by size; matching files are skipped. Client-only
// mods are never uploaded. When opts.Files is set, unselected files are left
// untouched and DeleteExtras is intentionally ignored.
func (c *Client) Sync(ctx context.Context, daemonID, instanceUUID string, opts SyncOptions) (SyncResult, error) {
	var result SyncResult
	if strings.TrimSpace(opts.LocalDir) == "" {
		return result, errors.New("本地游戏目录为空")
	}
	info, err := os.Stat(opts.LocalDir)
	if err != nil || !info.IsDir() {
		return result, errors.New("本地游戏目录不存在")
	}
	remoteRoot, err := normalizeRemoteRoot(opts.RemoteRoot)
	if err != nil {
		return result, err
	}
	opts.RemoteRoot = remoteRoot
	dirs, err := normalizeSyncDirs(opts.Dirs)
	if err != nil {
		return result, err
	}
	progress := func(stage, message string, current, total int64) {
		if opts.Progress != nil {
			opts.Progress(stage, message, current, total)
		}
	}

	progress("scan", "扫描本地文件…", 0, 0)
	localFiles, presentDirs, filtered, err := collectSyncFiles(opts, dirs)
	if err != nil {
		return result, err
	}
	result.Filtered = filtered
	progress("scan", fmt.Sprintf("找到 %d 个本地文件", len(localFiles)), int64(len(localFiles)), int64(len(localFiles)))
	if filtered > 0 {
		progress("filter", fmt.Sprintf("已跳过 %d 个仅客户端模组", filtered), int64(filtered), int64(filtered))
	}

	// Make sure the selected remote directories exist before listing them.
	for _, dir := range presentDirs {
		if err := c.Mkdir(ctx, daemonID, instanceUUID, path.Join(remoteRoot, dir)); err != nil {
			return result, fmt.Errorf("创建远端目录 %s 失败: %w", dir, err)
		}
	}

	remoteFiles := map[string]RemoteFile{}
	for _, dir := range presentDirs {
		if err := c.walkRemote(ctx, daemonID, instanceUUID, path.Join(remoteRoot, dir), remoteFiles, 0); err != nil {
			return result, fmt.Errorf("读取远端目录 %s 失败: %w", dir, err)
		}
	}
	progress("scan", "对比文件差异…", 0, 0)

	uploadPaths := make([]string, 0, len(localFiles))
	var uploadBytes int64
	for remotePath, file := range localFiles {
		if remote, ok := remoteFiles[remotePath]; ok && remote.Size == file.size {
			result.Skipped++
			continue
		}
		uploadPaths = append(uploadPaths, remotePath)
		uploadBytes += file.size
	}
	sort.Strings(uploadPaths)

	createdDirs := map[string]bool{}
	var uploadedBytes int64
	for idx, remotePath := range uploadPaths {
		file := localFiles[remotePath]
		parent := path.Dir(remotePath)
		if !createdDirs[parent] {
			if err := c.Mkdir(ctx, daemonID, instanceUUID, parent); err != nil {
				return result, fmt.Errorf("创建远端目录 %s 失败: %w", parent, err)
			}
			createdDirs[parent] = true
		}
		message := fmt.Sprintf("上传 %s（%d/%d）", path.Base(remotePath), idx+1, len(uploadPaths))
		progress("upload", message, uploadedBytes, uploadBytes)
		err := c.UploadFile(ctx, daemonID, instanceUUID, parent, file.path, func(sent, total int64) {
			progress("upload", message, uploadedBytes+sent, uploadBytes)
		})
		if err != nil {
			return result, fmt.Errorf("上传 %s 失败: %w", remotePath, err)
		}
		uploadedBytes += file.size
		result.Uploaded++
		result.Bytes += file.size
		progress("upload", message, uploadedBytes, uploadBytes)
	}

	// A manual file selection must never delete files the user did not select.
	if opts.DeleteExtras && len(opts.Files) == 0 {
		extras := make([]string, 0)
		for remotePath := range remoteFiles {
			if _, ok := localFiles[remotePath]; !ok {
				extras = append(extras, remotePath)
			}
		}
		sort.Strings(extras)
		for start := 0; start < len(extras); start += 50 {
			end := start + 50
			if end > len(extras) {
				end = len(extras)
			}
			batch := extras[start:end]
			progress("delete", fmt.Sprintf("删除远端文件（%d/%d）", end, len(extras)), int64(end), int64(len(extras)))
			if err := c.DeleteTargets(ctx, daemonID, instanceUUID, batch); err != nil {
				return result, fmt.Errorf("删除远端文件失败: %w", err)
			}
			result.Deleted += len(batch)
		}
	}

	message := fmt.Sprintf("同步完成：上传 %d，跳过 %d，删除 %d", result.Uploaded, result.Skipped, result.Deleted)
	if result.Filtered > 0 {
		message += fmt.Sprintf("，过滤客户端模组 %d", result.Filtered)
	}
	progress("done", message, 1, 1)
	return result, nil
}

func (c *Client) walkRemote(ctx context.Context, daemonID, instanceUUID, dir string, out map[string]RemoteFile, depth int) error {
	if depth > 20 {
		return errors.New("远端目录层级过深")
	}
	items, err := c.ListDir(ctx, daemonID, instanceUUID, dir)
	if err != nil {
		return err
	}
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
			continue
		}
		full := path.Join(dir, name)
		if item.Type == 0 {
			if err := c.walkRemote(ctx, daemonID, instanceUUID, full, out, depth+1); err != nil {
				return err
			}
			continue
		}
		out[full] = item
	}
	return nil
}

func skipSyncName(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case ".ds_store", "thumbs.db", "desktop.ini", ".gitkeep":
		return true
	}
	return strings.HasSuffix(lower, ".tmp") ||
		strings.HasSuffix(lower, ".old") || strings.HasSuffix(lower, ".dis") ||
		strings.Contains(lower, ".old.") || strings.Contains(lower, ".dis.")
}
