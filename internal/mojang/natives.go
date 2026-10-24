package mojang

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"MyMinecraftGoLuncher/internal/util"
)

// ExtractNativeJar unpacks a native library archive, skipping metadata and
// paths that would escape dest.
func ExtractNativeJar(jarPath, dest string, excludes []string) error {
	r, err := zip.OpenReader(jarPath)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if strings.HasPrefix(strings.ToUpper(name), "META-INF/") {
			continue
		}
		excluded := false
		for _, ex := range excludes {
			ex = strings.ReplaceAll(ex, "\\", "/")
			if ex != "" && strings.HasPrefix(name, ex) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 || !mode.IsRegular() {
			continue
		}
		target, err := util.SafeJoin(dest, name)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, 1<<30))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}
