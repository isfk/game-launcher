//go:build darwin

package epic

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 macOS 上 Epic 的 Manifests 目录。
// Epic 在 Mac 上只发了少数游戏，但目录结构与 Windows 一致。
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Epic", "EpicGamesLauncher", "Data", "Manifests"),
	}
}
