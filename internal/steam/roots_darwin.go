//go:build darwin

package steam

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 macOS 上 Steam 的根目录。
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return dedupe([]string{
		filepath.Join(home, "Library", "Application Support", "Steam"),
	})
}
