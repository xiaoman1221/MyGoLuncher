package mcsm

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path"
	"strings"
	"time"
)

const backupRootName = ".mgl-backups"

// CreateBackup snapshots the selected remote directories on the MCSM host. The
// archives stay on the server, so rollback does not require downloading the
// files to the launcher first.
func (c *Client) CreateBackup(ctx context.Context, daemonID, instanceUUID, remoteRoot, scope string, dirs []string, reason string, progress ProgressFunc) (Backup, error) {
	var result Backup
	root, err := normalizeRemoteRoot(remoteRoot)
	if err != nil {
		return result, err
	}
	dirs, err = normalizeSyncDirs(dirs)
	if err != nil {
		return result, err
	}
	scope = sanitizeBackupScope(scope)
	backupID := newBackupID()
	backupDir := backupPath(root, scope, backupID)
	result = Backup{
		ID: backupID, CreatedAt: Now(), Reason: reason,
		Dirs: append([]string(nil), dirs...),
	}
	report := func(stage, message string, current, total int64) {
		if progress != nil {
			progress(stage, message, current, total)
		}
	}
	if err := c.Mkdir(ctx, daemonID, instanceUUID, backupDir); err != nil {
		return result, fmt.Errorf("创建远端快照目录失败: %w", err)
	}

	rootItems, err := c.ListDir(ctx, daemonID, instanceUUID, root)
	if err != nil {
		return result, fmt.Errorf("读取远端根目录失败: %w", err)
	}
	remoteDirs := map[string]bool{}
	for _, item := range rootItems {
		if item.Type == 0 {
			remoteDirs[strings.TrimSpace(item.Name)] = true
		}
	}

	for idx, dir := range dirs {
		report("backup", fmt.Sprintf("正在创建回滚快照 %s（%d/%d）", dir, idx+1, len(dirs)), int64(idx), int64(len(dirs)))
		if !remoteDirs[dir] {
			result.MissingDirs = append(result.MissingDirs, dir)
			continue
		}
		remoteDir := path.Join(root, dir)
		items, listErr := c.ListDir(ctx, daemonID, instanceUUID, remoteDir)
		if listErr != nil {
			return result, fmt.Errorf("读取远端目录 %s 失败: %w", dir, listErr)
		}
		targets := make([]string, 0, len(items))
		for _, item := range items {
			name := strings.TrimSpace(item.Name)
			if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") {
				continue
			}
			targets = append(targets, path.Join(remoteDir, name))
		}
		if len(targets) == 0 {
			result.EmptyDirs = append(result.EmptyDirs, dir)
			continue
		}
		tempZip := path.Join(remoteDir, ".mgl-snapshot-"+backupID+".zip")
		if err := c.Compress(ctx, daemonID, instanceUUID, tempZip, targets, 1); err != nil {
			_ = c.DeleteTargets(ctx, daemonID, instanceUUID, []string{tempZip})
			return result, fmt.Errorf("压缩远端目录 %s 失败: %w", dir, err)
		}
		// MCSM acknowledges /file/compress before the archive is written.
		// Wait until the file exists and its size is stable before moving it.
		if err := c.waitForRemoteFile(ctx, daemonID, instanceUUID, tempZip); err != nil {
			_ = c.DeleteTargets(ctx, daemonID, instanceUUID, []string{tempZip})
			return result, fmt.Errorf("等待远端快照 %s 超时: %w", dir, err)
		}
		archivePath := path.Join(backupDir, dir+".zip")
		if err := c.MoveTargets(ctx, daemonID, instanceUUID, [][2]string{{tempZip, archivePath}}); err != nil {
			_ = c.DeleteTargets(ctx, daemonID, instanceUUID, []string{tempZip})
			return result, fmt.Errorf("保存远端快照 %s 失败: %w", dir, err)
		}
	}
	report("backup", "回滚快照已创建", int64(len(dirs)), int64(len(dirs)))
	return result, nil
}

// RestoreBackup deletes the synchronized remote directories and restores them
// from a snapshot archive. The caller must stop the server first.
func (c *Client) RestoreBackup(ctx context.Context, daemonID, instanceUUID, remoteRoot, scope string, backup Backup, progress ProgressFunc) error {
	root, err := normalizeRemoteRoot(remoteRoot)
	if err != nil {
		return err
	}
	scope = sanitizeBackupScope(scope)
	if !validBackupID(backup.ID) {
		return fmt.Errorf("回滚快照 ID 无效")
	}
	dirs, err := normalizeSyncDirs(backup.Dirs)
	if err != nil {
		return err
	}
	backup.Dirs = dirs
	backupDir := backupPath(root, scope, backup.ID)
	archives, err := c.ListDir(ctx, daemonID, instanceUUID, backupDir)
	if err != nil {
		return fmt.Errorf("读取回滚快照失败: %w", err)
	}
	available := map[string]bool{}
	for _, item := range archives {
		if item.Type == 1 {
			available[strings.TrimSpace(item.Name)] = true
		}
	}
	missing := map[string]bool{}
	for _, dir := range backup.MissingDirs {
		missing[dir] = true
	}
	empty := map[string]bool{}
	for _, dir := range backup.EmptyDirs {
		empty[dir] = true
	}
	report := func(stage, message string, current, total int64) {
		if progress != nil {
			progress(stage, message, current, total)
		}
	}
	// Verify every required archive before deleting anything. A missing
	// snapshot must abort the rollback, not leave the server directory empty.
	for _, dir := range backup.Dirs {
		if missing[dir] || empty[dir] {
			continue
		}
		if !available[dir+".zip"] {
			return fmt.Errorf("回滚快照缺少 %s，已取消回滚", dir+".zip")
		}
	}
	for idx, dir := range backup.Dirs {
		report("rollback", fmt.Sprintf("正在恢复 %s（%d/%d）", dir, idx+1, len(backup.Dirs)), int64(idx), int64(len(backup.Dirs)))
		remoteDir := path.Join(root, dir)
		if err := c.DeleteTargets(ctx, daemonID, instanceUUID, []string{remoteDir}); err != nil {
			return fmt.Errorf("删除远端目录 %s 失败: %w", dir, err)
		}
		if missing[dir] {
			continue
		}
		if err := c.Mkdir(ctx, daemonID, instanceUUID, remoteDir); err != nil {
			return fmt.Errorf("创建远端目录 %s 失败: %w", dir, err)
		}
		if empty[dir] {
			continue
		}
		archiveName := dir + ".zip"
		if err := c.Compress(ctx, daemonID, instanceUUID,
			path.Join(backupDir, archiveName), []string{remoteDir}, 0); err != nil {
			return fmt.Errorf("恢复远端目录 %s 失败: %w", dir, err)
		}
		// Extraction is asynchronous on the daemon; wait for the directory
		// content to settle before reporting success.
		if err := c.waitForRemoteDir(ctx, daemonID, instanceUUID, remoteDir); err != nil {
			return fmt.Errorf("等待远端目录 %s 恢复超时: %w", dir, err)
		}
	}
	report("rollback", "回滚完成", int64(len(backup.Dirs)), int64(len(backup.Dirs)))
	return nil
}

// waitForRemoteFile waits until target exists and its size has been stable for
// two consecutive polls. MCSM's compress endpoint returns before the archive
// is complete.
func (c *Client) waitForRemoteFile(ctx context.Context, daemonID, instanceUUID, target string) error {
	deadline := time.Now().Add(5 * time.Minute)
	lastSize := int64(-1)
	for {
		items, err := c.ListDir(ctx, daemonID, instanceUUID, path.Dir(target))
		if err != nil {
			return err
		}
		name := path.Base(target)
		size := int64(-1)
		for _, item := range items {
			if strings.TrimSpace(item.Name) == name && item.Type == 1 {
				size = item.Size
				break
			}
		}
		if size > 0 && size == lastSize {
			return nil
		}
		lastSize = size
		if time.Now().After(deadline) {
			return fmt.Errorf("超过等待时间")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// waitForRemoteDir waits until a restored directory has non-empty, stable
// contents. An existing archive always represents at least one entry because
// empty directories are recorded separately.
func (c *Client) waitForRemoteDir(ctx context.Context, daemonID, instanceUUID, dir string) error {
	deadline := time.Now().Add(5 * time.Minute)
	lastCount := -1
	for {
		items, err := c.ListDir(ctx, daemonID, instanceUUID, dir)
		if err != nil {
			return err
		}
		count := len(items)
		if count > 0 && count == lastCount {
			return nil
		}
		lastCount = count
		if time.Now().After(deadline) {
			return fmt.Errorf("超过等待时间")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// DeleteBackup removes one snapshot directory from the remote host.
func (c *Client) DeleteBackup(ctx context.Context, daemonID, instanceUUID, remoteRoot, scope string, backup Backup) error {
	root, err := normalizeRemoteRoot(remoteRoot)
	if err != nil {
		return err
	}
	if !validBackupID(backup.ID) {
		return fmt.Errorf("回滚快照 ID 无效")
	}
	return c.DeleteTargets(ctx, daemonID, instanceUUID, []string{backupPath(root, sanitizeBackupScope(scope), backup.ID)})
}

func validBackupID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 64 {
		return false
	}
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func backupPath(remoteRoot, scope, backupID string) string {
	return path.Join(remoteRoot, backupRootName, scope, backupID)
}

func sanitizeBackupScope(scope string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(scope)) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		}
	}
	if builder.Len() == 0 {
		return "default"
	}
	return builder.String()
}

func newBackupID() string {
	var suffix [3]byte
	_, _ = rand.Read(suffix[:])
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(suffix[:])
}

// IsRemoteRunning reports whether a status blocks file restoration.
func IsRemoteRunning(status int) bool {
	return status == -1 || status == 1 || status == 2 || status == 3
}
