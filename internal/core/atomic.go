package core

import (
	"os"
	"path/filepath"
	"syscall"
)

// WithLock runs fn while holding an exclusive advisory lock on lockPath.
// CLI, UI, and import share the same lock path to serialize writers.
func WithLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

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

// fsyncDir flushes a directory entry to disk. A failure to open/sync the
// directory is non-fatal for correctness (the file content is already synced),
// so callers that only need consistency may ignore it; AtomicWrite surfaces it.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return err
	}
	return nil
}
