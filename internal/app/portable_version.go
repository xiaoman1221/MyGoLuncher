package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"MyMinecraftGoLuncher/internal/mojang"
	"MyMinecraftGoLuncher/internal/store"
	"MyMinecraftGoLuncher/internal/util"
)

// ensurePortableVersion writes a standard custom version profile into
// .minecraft/versions/<versionId>. The profile is a fully merged version JSON,
// not a launcher-private shortcut, so PCL/HMCL/Prism/MultiMC and the official
// launcher can launch it directly. The client jar is hard-linked when possible
// and copied otherwise.
func (a *App) ensurePortableVersion(inst store.Instance) error {
	if a.store == nil || a.mojang == nil || !a.store.Settings().VersionIsolation {
		return nil
	}
	versionID := strings.TrimSpace(inst.VersionID)
	if versionID == "" {
		versionID = strings.TrimSpace(inst.ID)
	}
	launchID := strings.TrimSpace(inst.LaunchVersionID)
	if launchID == "" {
		launchID = strings.TrimSpace(inst.MCVersion)
	}
	if versionID == "" || launchID == "" {
		return nil
	}
	chain, err := a.mojang.Chain(launchID)
	if err != nil {
		return err
	}
	merged := mojang.Merge(chain)
	if merged.MainClass == "" {
		return fmt.Errorf("版本 %s 缺少主类，无法生成通用版本档案", launchID)
	}
	merged.ID = versionID
	merged.InheritsFrom = ""
	if strings.TrimSpace(merged.Type) == "" {
		merged.Type = "release"
	}
	if merged.Time.IsZero() {
		merged.Time = mojang.FlexTime{Time: time.Now().UTC()}
	}
	if merged.ReleaseTime.IsZero() {
		merged.ReleaseTime = merged.Time
	}

	dir := filepath.Join(a.paths.Versions, versionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := util.WriteJSON(filepath.Join(dir, versionID+".json"), merged); err != nil {
		return fmt.Errorf("写入通用版本档案失败: %w", err)
	}

	sourceJar := ""
	for _, version := range chain {
		if _, ok := version.Downloads["client"]; !ok {
			continue
		}
		candidate := a.mojang.ClientJar(version.ID)
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			sourceJar = candidate
			break
		}
	}
	if sourceJar == "" {
		return nil
	}
	targetJar := filepath.Join(dir, versionID+".jar")
	if info, statErr := os.Stat(targetJar); statErr == nil {
		if sourceInfo, sourceErr := os.Stat(sourceJar); sourceErr == nil && info.Size() == sourceInfo.Size() {
			return nil
		}
		if err := os.Remove(targetJar); err != nil {
			return err
		}
	}
	if err := os.Link(sourceJar, targetJar); err == nil {
		return nil
	}
	if err := util.CopyFile(sourceJar, targetJar); err != nil {
		return fmt.Errorf("复制客户端主文件失败: %w", err)
	}
	return nil
}

func (a *App) ensurePortableVersions() {
	if a.store == nil || a.mojang == nil || !a.store.Settings().VersionIsolation {
		return
	}
	for _, inst := range a.store.Instances() {
		_ = a.ensurePortableVersion(inst)
	}
}
