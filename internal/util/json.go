package util

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// ReadJSON decodes a JSON file into v.
func ReadJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(v)
}

// WriteJSON writes v atomically so a crash cannot leave a half-written config.
func WriteJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// ReplaceFile moves tempPath over dest. On platforms where Rename cannot
// replace an existing file it falls back to a backup-and-restore sequence so
// a failed replacement never loses the previous destination.
func ReplaceFile(tempPath, dest string) error {
	if err := os.Rename(tempPath, dest); err == nil {
		return nil
	}
	backup := dest + ".bak-replace"
	for index := 1; ; index++ {
		if _, err := os.Stat(backup); os.IsNotExist(err) {
			break
		}
		backup = fmt.Sprintf("%s.bak-replace.%d", dest, index)
	}
	if err := os.Rename(dest, backup); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return os.Rename(tempPath, dest)
		}
		return err
	}
	if err := os.Rename(tempPath, dest); err != nil {
		_ = os.Rename(backup, dest)
		return err
	}
	_ = os.Remove(backup)
	return nil
}

// Exists reports whether path exists and is not a directory.
func Exists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// NonEmptyFile reports whether path exists and has content.
func NonEmptyFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// CopyFile copies src to dst, creating parent directories.
func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func ErrEmpty(value, name string) error {
	if value == "" {
		return errors.New(name + " 不能为空")
	}
	return nil
}
