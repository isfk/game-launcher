// Package manual 把「手动添加的游戏」变成启动器认识的 Game。
//
// 手动添加的游戏没有平台、没有 appmanifest、没有 CDN 图，只有一条路径：
// 所以这里的全部工作就是「路径 → 一个像样的 Game」，外加几个防手滑的校验。
package manual

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"game-launcher/internal/config"
	"game-launcher/internal/library"
)

// 允许直接启动的扩展名。Windows 上不乱认，免得把 .txt 也当游戏；
// macOS 的 .app 是个目录，单独处理。
var windowsExts = map[string]bool{".exe": true, ".bat": true, ".cmd": true, ".lnk": true}

// IDFor 由路径算出稳定的 key：同一个文件重复添加不会变成两条。
func IDFor(path string) string {
	sum := sha256.Sum256([]byte(path))
	return "manual:" + hex.EncodeToString(sum[:])[:10]
}

// TitleFor 从路径猜一个显示名：去掉扩展名，.app 包也去掉后缀。
//
// 不去猜「_」还是「-」该换成空格 —— 游戏名里的标点是有意义的
// （比如 "NieR_Automata" 就是人家自己的写法）。
func TitleFor(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if runtime.GOOS == "darwin" && strings.EqualFold(ext, ".app") {
		return strings.TrimSuffix(base, ext)
	}
	if windowsExts[strings.ToLower(ext)] {
		return strings.TrimSuffix(base, ext)
	}
	// 脚本类（.sh / .command）也去掉后缀
	if strings.EqualFold(ext, ".sh") || strings.EqualFold(ext, ".command") {
		return strings.TrimSuffix(base, ext)
	}
	if ext == "" {
		return base
	}
	return base
}

// ValidPath 检查这条路径能不能当游戏启动。返回的错误是给人看的。
func ValidPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("没有选择文件")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("找不到这个文件：%w", err)
	}

	if info.IsDir() {
		if runtime.GOOS == "darwin" && strings.EqualFold(filepath.Ext(path), ".app") {
			return nil
		}
		return fmt.Errorf("选中的是文件夹，不是可执行文件")
	}

	switch runtime.GOOS {
	case "windows":
		if !windowsExts[strings.ToLower(filepath.Ext(path))] {
			return fmt.Errorf("只能添加 .exe / .bat / .cmd / .lnk")
		}
	default:
		// 有没有执行位是最实在的判断；不强制，因为解压出来的程序常常丢执行位
		if info.Mode()&0o111 == 0 {
			return fmt.Errorf("这个文件没有执行权限（chmod +x 之后再添加）")
		}
	}
	return nil
}

// Entry 由路径造一条待落盘的记录。
func Entry(path, title string) config.ManualGame {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if title == "" {
		title = TitleFor(abs)
	}
	return config.ManualGame{
		ID:    IDFor(abs),
		Title: title,
		Path:  abs,
		Added: time.Now().UTC(),
	}
}

// Games 把落盘的记录转成界面认的 Game。
//
// InstallDir 指向可执行文件所在目录：判断「游戏还在跑吗」靠进程的 cwd /
// 命令行里有没有这个目录（见 internal/proc）。
func Games(entries []config.ManualGame) []library.Game {
	out := make([]library.Game, 0, len(entries))
	for _, e := range entries {
		if e.Path == "" {
			continue
		}
		installDir := filepath.Dir(e.Path)
		out = append(out, library.Game{
			Key:        IDFor(e.Path),
			Title:      e.Title,
			Provider:   "manual",
			InstallDir: installDir,
			Installed:  true,
		})
	}
	return out
}
