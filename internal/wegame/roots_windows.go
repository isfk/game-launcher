//go:build windows

package wegame

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 WeGame 放游戏的目录（rail_apps）。
//
// WeGame 通常装在 Program Files (x86)，但也可以装到别的盘；
// 这里把两个 Program Files 都试一遍，剩下的靠 LAUNCHER_WEGAME_DIR 兜。
func DefaultRoots() []string {
	var roots []string
	seen := map[string]bool{}
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles"} {
		dir := os.Getenv(env)
		if dir == "" {
			continue
		}
		for _, sub := range []string{"WeGame", filepath.Join("Tencent", "WeGame")} {
			root := filepath.Join(dir, sub, "rail_apps")
			if seen[root] {
				continue
			}
			seen[root] = true
			roots = append(roots, root)
		}
	}
	return roots
}
