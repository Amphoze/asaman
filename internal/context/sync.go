// Package context keeps the canonical always-loaded file's index block fresh
// without ever breaking the CLAUDE.md → AGENTS.md symlink.
package context

import (
	"fmt"
	"os"
	"strings"

	"github.com/amphoze/asaman/internal/core"
)

// SyncIndex replaces the marked index span in the file that contextFile points
// to. contextFile may be a symlink (e.g. CLAUDE.md → AGENTS.md); the write lands
// on the resolved real target, so the symlink is preserved. Serialized by lock.
func SyncIndex(contextFile, block string, markers [2]string, lockPath string) error {
	return core.WithLock(lockPath, func() error {
		real, err := resolve(contextFile)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(real)
		if err != nil {
			return err
		}
		next, err := spliceBlock(string(data), block, markers)
		if err != nil {
			return err
		}
		if next == string(data) {
			return nil
		}
		return core.AtomicWrite(real, []byte(next))
	})
}

// resolve follows a symlink (if any) to the real file path.
func resolve(p string) (string, error) {
	fi, err := os.Lstat(p)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(p)
		if err != nil {
			return "", err
		}
		if !strings.HasPrefix(target, "/") {
			// relative symlink: resolve against link's directory
			target = dirJoin(p, target)
		}
		return target, nil
	}
	return p, nil
}

func dirJoin(link, target string) string {
	i := strings.LastIndex(link, "/")
	if i < 0 {
		return target
	}
	return link[:i+1] + target
}

// spliceBlock replaces the content between markers (inclusive of the marker
// lines' interior) with block. If the markers are absent, it appends a fresh
// marked section at the end of the file.
func spliceBlock(content, block string, markers [2]string) (string, error) {
	start := strings.Index(content, markers[0])
	end := strings.Index(content, markers[1])
	newSection := markers[0] + "\n" + strings.TrimRight(block, "\n") + "\n" + markers[1]
	if start < 0 && end < 0 {
		sep := "\n"
		if strings.HasSuffix(content, "\n") {
			sep = "\n"
		}
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + sep + newSection + "\n", nil
	}
	if start < 0 || end < 0 || end < start {
		return "", fmt.Errorf("context: malformed index markers")
	}
	before := content[:start]
	after := content[end+len(markers[1]):]
	return before + newSection + after, nil
}
