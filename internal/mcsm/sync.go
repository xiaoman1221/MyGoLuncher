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
)

type localFile struct {
	path string
	size int64
}

// Sync incrementally uploads selected local directories into a remote MCSM
// instance. Files are compared by size; matching files are skipped.
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
	localFiles := map[string]localFile{}
	presentDirs := make([]string, 0, len(dirs))
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
			if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return nil
			}
			if skipSyncName(entry.Name()) {
				return nil
			}
			rel, relErr := filepath.Rel(localDir, filePath)
			if relErr != nil || rel == "." || strings.HasPrefix(rel, "..") {
				return nil
			}
			stat, statErr := entry.Info()
			if statErr != nil {
				return statErr
			}
			remotePath := path.Join(remoteRoot, dir, filepath.ToSlash(rel))
			localFiles[remotePath] = localFile{path: filePath, size: stat.Size()}
			return nil
		})
		if err != nil {
			return result, fmt.Errorf("扫描 %s 失败: %w", dir, err)
		}
	}
	progress("scan", fmt.Sprintf("找到 %d 个本地文件", len(localFiles)), int64(len(localFiles)), int64(len(localFiles)))

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

	if opts.DeleteExtras {
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

	progress("done", fmt.Sprintf("同步完成：上传 %d，跳过 %d，删除 %d", result.Uploaded, result.Skipped, result.Deleted), 1, 1)
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
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db", "desktop.ini", ".gitkeep":
		return true
	}
	return strings.HasSuffix(strings.ToLower(name), ".tmp")
}
