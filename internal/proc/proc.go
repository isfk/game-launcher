// Package proc 回答一个问题：还有没有进程在这个目录里跑？
//
// 启动器用它来判断「启动的游戏退出了没有」——Steam 的 steam:// 协议
// 不给我们 PID，只能靠「安装目录里还有没有活进程」来推断。
package proc

import (
	"path/filepath"
	"strings"
)

// within 判断 path 是不是在 dir 里（Windows 上大小写不敏感）。
func within(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(cleanPath(dir), cleanPath(path))
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cleanPath(p string) string {
	cleaned := filepath.Clean(p)
	// macOS 上 /private/var 与 /var、/Volumes 的软链会干扰前缀比较
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return resolved
	}
	return cleaned
}
