//go:build linux

package steam

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 Linux 上 Steam 的根目录。
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return dedupe([]string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".local", "share", "Steam"),
	})
}
