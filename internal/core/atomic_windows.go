//go:build windows

package core

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// WithLock runs fn while holding an exclusive lock on lockPath. On Windows this
// uses LockFileEx over the first byte of the dedicated lock file (the same
// convention gofrs/flock uses); all writers lock the identical range, so it
// serializes CLI, UI, and import exactly as POSIX flock does on Unix.
func WithLock(lockPath string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	h := windows.Handle(f.Fd())
	ol := new(windows.Overlapped)
	// Blocking exclusive lock on 1 byte at offset 0.
	if err := windows.LockFileEx(h, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		return err
	}
	defer windows.UnlockFileEx(h, 0, 1, 0, ol)
	return fn()
}

// fsyncDir is a no-op on Windows: a directory handle cannot be flushed the way
// POSIX allows, and NTFS metadata durability does not depend on it. AtomicWrite
// still fsyncs the file itself before rename, so content durability holds.
func fsyncDir(dir string) error { return nil }
