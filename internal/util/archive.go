package util

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SafeJoin joins base and name, rejecting paths that escape base.
func SafeJoin(base, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	cleaned := filepath.Clean(filepath.FromSlash(name))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("无效路径: %q", name)
	}
	target := filepath.Join(base, cleaned)
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("压缩包包含非法路径: %q", name)
	}
	return target, nil
}

// ExtractZip extracts a zip safely. If strip is non-empty, only entries below
// that directory are extracted and the prefix is removed.
func ExtractZip(zipPath, dest, strip string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	strip = strings.Trim(strings.ReplaceAll(strip, "\\", "/"), "/")
	for _, f := range r.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if strip != "" {
			if name == strip || name == strip+"/" {
				continue
			}
			if !strings.HasPrefix(name, strip+"/") {
				continue
			}
			name = strings.TrimPrefix(name, strip+"/")
		}
		if name == "" {
			continue
		}
		target, err := SafeJoin(dest, name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		// Refuse links and devices.
		mode := f.Mode()
		if mode&os.ModeSymlink != 0 || !mode.IsRegular() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, 4<<30))
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

// ExtractTarGz extracts a .tar.gz safely.
func ExtractTarGz(archivePath, dest string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if h.Name == "" {
			continue
		}
		// Java archives wrap everything in a single top-level directory.
		name := strings.TrimPrefix(strings.ReplaceAll(h.Name, "\\", "/"), "./")
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 {
			name = parts[1]
		}
		if name == "" {
			continue
		}
		target, err := SafeJoin(dest, name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o755)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, io.LimitReader(tr, 4<<30))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

// CopyDirTree copies the contents of src into dst.
func CopyDirTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return CopyFile(path, target)
	})
}
