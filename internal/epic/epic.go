// Package epic 接入 Epic Games 的本机游戏。
//
// 和 Steam 一样，只读本地清单、不登录、不调 Web API：
// 每个装好的游戏在 Manifests 目录里有一份 <AppName>.item（JSON），
// 里面就有显示名、安装目录、可执行文件。启动和卸载一律交给
// Epic 客户端自己的 com.epicgames.launcher:// 协议。
package epic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"game-launcher/internal/launch"
	"game-launcher/internal/library"
)

// Manifest 是 .item 文件里我们关心的那些字段。
// Epic 自己会往里加字段，所以这里只声明用得到的，其余忽略。
type Manifest struct {
	AppName          string   `json:"AppName"`
	DisplayName      string   `json:"DisplayName"`
	AppCategories    []string `json:"AppCategories"`
	InstallLocation  string   `json:"InstallLocation"`
	LaunchExecutable string   `json:"LaunchExecutable"`
	// MainGameAppName 非空表示这是某个游戏的 DLC / 附加内容。
	MainGameAppName string `json:"MainGameAppName"`
	// IsInDevelopment 是 Epic 内部开发版标记，正常玩家不会遇到，跳过更安全。
	IsInDevelopment     bool   `json:"IsInDevelopment"`
	IsIncompleteInstall bool   `json:"bIsIncompleteInstall"`
	InstallSize         int64  `json:"InstallSize"`
	AppVersionString    string `json:"AppVersionString"`
	CatalogNamespace    string `json:"CatalogNamespace"`
}

// RootFromEnv 允许用 LAUNCHER_EPIC_DIR 指定 Manifests 目录，
// 开发机上没有 Epic、或者想用假清单调界面时用。
func RootFromEnv() string {
	return strings.TrimSpace(os.Getenv("LAUNCHER_EPIC_DIR"))
}

// usable 判断一份清单该不该进游戏库。
//
// 这个字段组合是踩过坑的：Manifests 目录里躺着的不只是游戏，
// 还有插件、引擎本体、DLC、以及装在开发机上的内部版本。
func usable(m Manifest) bool {
	if strings.TrimSpace(m.AppName) == "" || strings.TrimSpace(m.DisplayName) == "" {
		return false
	}
	if strings.TrimSpace(m.InstallLocation) == "" {
		return false
	}
	// 只收类目里明说自己是游戏的
	if !hasCategory(m.AppCategories, "games") {
		return false
	}
	// DLC / 附加包不算独立一格
	if strings.TrimSpace(m.MainGameAppName) != "" {
		return false
	}
	if m.IsInDevelopment || m.IsIncompleteInstall {
		return false
	}
	return true
}

func hasCategory(categories []string, want string) bool {
	for _, c := range categories {
		if strings.EqualFold(c, want) {
			return true
		}
	}
	return false
}

// Game 把一份清单变成界面认的 Game。
func gameOf(m Manifest) library.Game {
	return library.Game{
		Key:        "epic:" + m.AppName,
		Title:      m.DisplayName,
		Provider:   "epic",
		AppID:      m.AppName, // 协议里用的就是 AppName
		InstallDir: m.InstallLocation,
		SizeBytes:  m.InstallSize,
		Installed:  true,
		// Epic 本地没有库封面（图和账号绑定，存在它自己的缓存里），
		// 这里就留空，交给界面的渐变占位兜底。
	}
}

// Scan 读一个 Manifests 目录里的所有 .item。
func Scan(ctx context.Context, dir string) ([]library.Game, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读 Epic 清单目录失败: %w", err)
	}

	games := make([]library.Game, 0, len(entries))
	for _, e := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".item") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue // 单个清单读不到不影响别的
		}
		var m Manifest
		if err := json.Unmarshal(raw, &m); err != nil {
			continue // 清单写坏了就跳过这一份
		}
		if !usable(m) {
			continue
		}
		games = append(games, gameOf(m))
	}

	// 按名字排：Epic 清单里没有「上次游玩」，顺序只能是稳定的字典序
	sort.Slice(games, func(i, j int) bool {
		return strings.ToLower(games[i].Title) < strings.ToLower(games[j].Title)
	})
	return games, nil
}

// Provider 把 Epic 接成 library.Provider。
type Provider struct{}

func (Provider) ID() string { return "epic" }

func (Provider) Scan(ctx context.Context) ([]library.Game, error) {
	dirs := []string{}
	if root := RootFromEnv(); root != "" {
		dirs = []string{root}
	} else {
		dirs = DefaultRoots()
	}

	var all []library.Game
	seen := make(map[string]bool)
	var firstErr error
	for _, dir := range dirs {
		games, err := Scan(ctx, dir)
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

func (Provider) Launch(ctx context.Context, g library.Game) error {
	if g.AppID == "" {
		return fmt.Errorf("「%s」缺少 Epic 的 AppName", g.Title)
	}
	return launch.OpenURL(ctx, LaunchURL(g.AppID))
}

// LaunchURL 是 Epic 客户端的启动协议。
// silent=true 表示「别弹中间那个确认窗口，直接开游戏」。
func LaunchURL(appName string) string {
	return fmt.Sprintf("com.epicgames.launcher://apps/%s?action=launch&silent=true", appName)
}

// UninstallURL 交给 Epic 客户端卸载：它要清自己的清单、注册表、
// 云存档映射，我们自己删目录会把客户端搞成半损状态。
func UninstallURL(appName string) string {
	return fmt.Sprintf("com.epicgames.launcher://apps/%s?action=uninstall&silent=true", appName)
}

// SignatureNow 用默认（或环境变量指定的）目录拍一次指纹。
func SignatureNow() string {
	if root := RootFromEnv(); root != "" {
		return Signature([]string{root})
	}
	return Signature(DefaultRoots())
}

// Signature 给「库有没有变化」拍个指纹（app.go 的轮询用）。
func Signature(dirs []string) string {
	var b strings.Builder
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".item") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			fmt.Fprintf(&b, "%s|%d|%d;", e.Name(), info.Size(), info.ModTime().UnixNano())
		}
	}
	return b.String()
}
