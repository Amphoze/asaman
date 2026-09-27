//go:build !windows

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
