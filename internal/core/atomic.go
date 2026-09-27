package core

import (
	"os"
	"path/filepath"
)

// AtomicWrite writes data to realPath via a temp file in the same directory,
// fsync, then rename. realPath must be the resolved real file (never a symlink
// path) so the rename replaces content, not the symlink.
func AtomicWrite(realPath string, data []byte) error {
	dir := filepath.Dir(realPath)
	tmp, err := os.CreateTemp(dir, ".asaman-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename
	if _, err := tmp.Write(data); err != nil {
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
	// Preserve the mode of the existing target if present.
	if fi, err := os.Stat(realPath); err == nil {
		os.Chmod(tmpName, fi.Mode())
	}
	if err := os.Rename(tmpName, realPath); err != nil {
		return err
	}
	// fsync the parent directory so the rename itself is durable across a crash
	// (rename gives atomicity/consistency, dir fsync gives durability).
	return fsyncDir(dir)
}

// WithLock and fsyncDir are platform-specific (see atomic_unix.go /
// atomic_windows.go): POSIX flock + directory fsync on Unix, LockFileEx on
// Windows where a directory handle cannot be fsync'd.
