// Package wegame 接入腾讯 WeGame 的本机游戏。
//
// WeGame 没有公开的清单格式（不像 Steam 的 appmanifest、Epic 的 .item），
// 它把游戏直接铺在 rail_apps 目录下：一个子目录 = 一款游戏。
// 所以这里只能靠「目录结构 + 可执行文件」推断，规则都写在 pickExec 里。
//
// 启动也走最实在的那条路：直接跑找到的 exe。WeGame 的 tencent:// 协议
// 需要它内部的游戏 id，那玩意儿没有稳定来源，拼错了只会静默失败。
package wegame

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"game-launcher/internal/launch"
	"game-launcher/internal/library"
)

// RootFromEnv 允许用 LAUNCHER_WEGAME_DIR 指定 rail_apps 目录，
// 开发机上没有 WeGame 时用它造夹具。
func RootFromEnv() string {
	return strings.TrimSpace(os.Getenv("LAUNCHER_WEGAME_DIR"))
}

// noiseDirs 是 rail_apps 里那些不是游戏的东西。
// 这些名字都是 WeGame 自己的组件目录，扫进来会冒出一堆莫名其妙的「游戏」。
var noiseDirs = map[string]bool{
	"wegame":       true,
	"tgp":          true,
	"tenio":        true,
	"plugins":      true,
	"plugin":       true,
	"download":     true,
	"downloads":    true,
	"redist":       true,
	"redists":      true,
	"directx":      true,
	"vcredist":     true,
	"common":       true,
	"temp":         true,
	"tmp":          true,
	"cache":        true,
	"update":       true,
	"updates":      true,
	"logs":         true,
	"resources":    true,
	"$recycle.bin": true,
}

// noiseExes 是游戏目录里那些不该被当成「主程序」的可执行文件：
// 卸载器、运行库安装包、崩溃上报器之类。
var noiseExes = []string{
	"unins", "uninstall", "setup", "install", "vcredist", "dxsetup",
	"dxwebsetup", "crash", "report", "update", "repair",
}

// skipDir 判断 rail_apps 下的某个子目录是不是游戏。
func skipDir(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || strings.HasPrefix(n, "$") || strings.HasPrefix(n, ".") {
		return true
	}
	return noiseDirs[n]
}

// pickExec 在一个游戏目录里挑主程序。
//
// 规则（宁可挑错也不挑到卸载器）：
//  1. 只在目录树里找 .exe，深度不超过 3 层（够覆盖 Binaries/Win64 这种）
//  2. 跳过疑似卸载/安装/上报的程序
//  3. 名字和目录名一样的优先（LOL\LOL.exe 这种最常见）
//  4. 其次取路径最浅的，最后取体积最大的
func pickExec(root, dirName string) string {
	type candidate struct {
		path  string
		depth int
		size  int64
	}
	var found []candidate

	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // 没权限的子目录就跳过
		}
		depth := strings.Count(filepath.Clean(path), string(os.PathSeparator)) - rootDepth
		if d.IsDir() {
			if depth >= 3 {
				return filepath.SkipDir // 再深基本是资源/日志
			}
			// 明显的运行库、缓存目录不必进
			if depth > 0 && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		for _, noise := range noiseExes {
			if strings.Contains(base, noise) {
				return nil
			}
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		found = append(found, candidate{path: path, depth: depth, size: info.Size()})
		return nil
	})

	if len(found) == 0 {
		return ""
	}

	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		aNamed := strings.EqualFold(strings.TrimSuffix(filepath.Base(a.path), ".exe"), dirName)
		bNamed := strings.EqualFold(strings.TrimSuffix(filepath.Base(b.path), ".exe"), dirName)
		if aNamed != bNamed {
			return aNamed // 同名的排前面
		}
		if a.depth != b.depth {
			return a.depth < b.depth // 越浅越像主程序
		}
		return a.size > b.size
	})
	return found[0].path
}

// Scan 读一个 rail_apps 目录。
func Scan(ctx context.Context, root string) ([]library.Game, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("读 WeGame 游戏目录失败: %w", err)
	}

	games := make([]library.Game, 0, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !e.IsDir() || skipDir(e.Name()) {
			continue
		}
		dir := filepath.Join(root, e.Name())
		exec := pickExec(dir, e.Name())
		if exec == "" {
			continue // 没有可执行文件：不是游戏，或者还没下载完
		}
		games = append(games, library.Game{
			Key:        "wegame:" + e.Name(),
			Title:      e.Name(),
			Provider:   "wegame",
			AppID:      e.Name(),
			InstallDir: dir,
			Exec:       exec,
			Installed:  true,
		})
	}

	// WeGame 目录名就是游戏名，没有「上次游玩」可排，按名字给个稳定顺序
	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Title) < strings.ToLower(games[j].Title)
	})
	return games, nil
}

// Provider 把 WeGame 接成 library.Provider。
type Provider struct{}

func (Provider) ID() string { return "wegame" }

func (Provider) Scan(ctx context.Context) ([]library.Game, error) {
	if runtime.GOOS != "windows" {
		// WeGame 只有 Windows 客户端；其他平台直接当作没装
		if RootFromEnv() == "" {
			return nil, nil
		}
	}
	roots := []string{}
	if root := RootFromEnv(); root != "" {
		roots = []string{root}
	} else {
		roots = DefaultRoots()
	}

	var all []library.Game
	seen := make(map[string]bool)
	var firstErr error
	for _, root := range roots {
		games, err := Scan(ctx, root)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, g := range games {
			if seen[g.Key] {
				continue
			}
			seen[g.Key] = true
			all = append(all, g)
		}
	}
	if len(all) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return all, nil
}

// Launch 直接跑找到的主程序。
func (Provider) Launch(ctx context.Context, g library.Game) error {
	if g.Exec == "" {
		return fmt.Errorf("「%s」没找到可执行文件，请在 WeGame 里启动一次试试", g.Title)
	}
	return launch.OpenURL(ctx, g.Exec)
}

// Signature 给 rai_apps 拍个指纹，用来发现「刚装了一款游戏」。
func Signature(roots []string) string {
	var b strings.Builder
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || skipDir(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s|%d;", e.Name(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}

// SignatureNow 用默认（或环境变量指定的）目录拍一次指纹。
func SignatureNow() string {
	if root := RootFromEnv(); root != "" {
		return Signature([]string{root})
	}
	return Signature(DefaultRoots())
}
