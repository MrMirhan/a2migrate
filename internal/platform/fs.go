package platform

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

// ErrPathNotFound is returned when an expected file or directory is missing.
var ErrPathNotFound = errors.New("path not found")

// Exists returns true if path exists on disk.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir returns true if path is an existing directory.
func IsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile returns true if path is an existing regular file.
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// EnsureDir creates dir (and parents) if missing.
func EnsureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// AtomicWriteFile writes data to path via a sibling temp file, then renames
// into place. On failure the temp file is removed.
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".a2migrate-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// CopyFile copies src to dst, preserving permissions. Refuses to overwrite
// unless overwrite is true.
func CopyFile(src, dst string, overwrite bool) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	if !overwrite {
		if _, err := os.Stat(dst); err == nil {
			return os.ErrExist
		}
	}

	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// BackupFile copies path to path+".bak-<timestamp>" and returns the backup path.
// If backupDir is non-empty, the file is placed there instead.
func BackupFile(path, backupDir string) (string, error) {
	if !Exists(path) {
		return "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	stamp := TimeStamp()
	suffix := ".bak-" + stamp
	dst := path + suffix
	if backupDir != "" {
		if err := EnsureDir(backupDir); err != nil {
			return "", err
		}
		base := filepath.Base(path)
		dst = filepath.Join(backupDir, base+suffix)
	}
	if err := AtomicWriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

// TimeStamp returns a sortable filesystem-safe timestamp.
// Format: 20060102-150405.999.
func TimeStamp() string {
	return nowFunc().Format("20060102-150405.000")
}

// nowFunc is overridable in tests.
var nowFunc = defaultNow

func defaultNow() timeLike { return timeNow() }
