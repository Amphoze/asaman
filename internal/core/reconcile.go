package core

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileState is a content fingerprint of one file.
type FileState struct {
	SHA256 string
	Size   int64
	MTime  float64
}

// Inventory maps absolute path → FileState.
type Inventory map[string]FileState

// Changes is the result of diffing two inventories.
type Changes struct {
	Added   []string
	Deleted []string
	Edited  []string
}

// Any reports whether the diff carries any change.
func (c Changes) Any() bool {
	return len(c.Added)+len(c.Deleted)+len(c.Edited) > 0
}

// ExpandGlob resolves a glob that may contain a `**` (recursive) segment.
func ExpandGlob(pattern string) []string {
	if !strings.Contains(pattern, "**") {
		m, _ := filepath.Glob(pattern)
		return m
	}
	i := strings.Index(pattern, "**")
	base := strings.TrimRight(pattern[:i], "/")
	if base == "" {
		base = "."
	}
	suffix := strings.TrimPrefix(pattern[i+2:], "/")
	var out []string
	filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if suffix == "" {
			out = append(out, p)
			return nil
		}
		if ok, _ := filepath.Match(suffix, filepath.Base(p)); ok {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// FileSHA returns the hex sha256 of a file's contents.
func FileSHA(path string) (string, error) { return fileSHA(path) }

func fileSHA(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ScanInventory fingerprints every file matched by the globs.
func ScanInventory(globs []string) (Inventory, error) {
	inv := Inventory{}
	for _, g := range globs {
		for _, p := range ExpandGlob(g) {
			fi, err := os.Stat(p)
			if err != nil || fi.IsDir() {
				continue
			}
			sha, err := fileSHA(p)
			if err != nil {
				continue
			}
			inv[p] = FileState{SHA256: sha, Size: fi.Size(),
				MTime: float64(fi.ModTime().UnixNano()) / 1e9}
		}
	}
	return inv, nil
}

// Diff compares two inventories by content hash — never by mtime, so an added
// file with an older timestamp and an edit that preserves size are both caught.
func Diff(old, cur Inventory) Changes {
	var ch Changes
	for p, cs := range cur {
		os_, ok := old[p]
		if !ok {
			ch.Added = append(ch.Added, p)
		} else if os_.SHA256 != cs.SHA256 {
			ch.Edited = append(ch.Edited, p)
		}
	}
	for p := range old {
		if _, ok := cur[p]; !ok {
			ch.Deleted = append(ch.Deleted, p)
		}
	}
	return ch
}
