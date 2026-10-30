//go:build linux

package epic

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 Linux 上（通过 Heroic / Lutris 之类装的）Epic 清单目录。
// Epic 官方没有 Linux 客户端，这里的路径按社区工具的约定给。
func DefaultRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".local", "share", "Epic", "EpicGamesLauncher", "Data", "Manifests"),
	}
}
