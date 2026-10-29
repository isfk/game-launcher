package steam

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"game-launcher/internal/launch"
	"game-launcher/internal/library"
)

// Scan 扫描给定的 Steam 根目录（传空则用 DefaultRoots），
// 返回装好的游戏，按上次运行时间从新到旧。
//
// 根目录 = Steam 安装目录，里面有 steamapps/ 的那个。
// ctx 取消时尽快返回，并把已经扫到的部分结果一起交出去。
func Scan(ctx context.Context, roots ...string) ([]library.Game, error) {
	if len(roots) == 0 {
		roots = DefaultRoots()
	}

	seen := map[string]bool{}
	var games []library.Game

	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return games, err
		}
		if root == "" || seen["root:"+root] {
			continue
		}
		seen["root:"+root] = true

		// 没有 steamapps 的目录不是错误：这台机器只是没装 Steam，
		// 空库是合法状态，界面会退回演示数据。
		if _, err := os.Stat(filepath.Join(root, "steamapps")); err != nil {
			continue
		}

		// 一个根目录下可能有多个库（多个盘）。
		for _, lib := range libraries(root) {
			for _, g := range gamesIn(lib, root) {
				if seen[g.Key] {
					continue
				}
				seen[g.Key] = true
				games = append(games, g)
			}
		}
	}

	// 合并 Steam 本地记的游玩时长、最后运行时间与分类标签。
	// 用根目录里最近活跃的那个账号的数据（多账号时以正在用的为准）。
	for _, root := range dedupe(roots) {
		stats := LoadPlayStats(root)
		if len(stats) == 0 {
			continue
		}
		for i := range games {
			stat, ok := stats[games[i].AppID]
			if !ok {
				continue
			}
			if stat.PlaytimeMinutes > 0 {
				games[i].PlaytimeMinutes = stat.PlaytimeMinutes
			}
			if !stat.LastPlayed.IsZero() {
				t := stat.LastPlayed
				games[i].LastPlayed = &t
			}
			if len(stat.Tags) > 0 {
				games[i].Tags = stat.Tags
			}
		}
	}

	sort.SliceStable(games, func(i, j int) bool {
		a, b := games[i].LastPlayed, games[j].LastPlayed
		switch {
		case a == nil && b == nil:
			return games[i].Title < games[j].Title
		case a == nil:
			return false
		case b == nil:
			return true
		default:
			return a.After(*b)
		}
	})

	return games, nil
}

// libraries 返回一个根目录下的全部库路径（根目录自己也在内）。
func libraries(root string) []string {
	out := []string{root}
	seen := map[string]bool{root: true}

	data, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf"))
	if err != nil {
		return out
	}
	obj, err := Parse(data)
	if err != nil {
		return out
	}

	folders := obj.Obj("libraryfolders")
	if folders == nil {
		folders = obj
	}
	for key, val := range folders {
		// 新格式： "0" { "path" "..." ... }；老格式： "1" "/path"
		var path string
		switch v := val.(type) {
		case Object:
			path = v.Str("path")
		case string:
			if key == "contentstatsid" || key == "last_install" {
				continue
			}
			path = v
		}
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

// gamesIn 读一个库里的全部 appmanifest，转成 Game。
func gamesIn(lib, root string) []library.Game {
	acfs, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
	if len(acfs) == 0 {
		return nil
	}

	out := make([]library.Game, 0, len(acfs))
	for _, acf := range acfs {
		data, err := os.ReadFile(acf)
		if err != nil {
			continue
		}
		m, err := ParseManifest(data)
		if err != nil || m.AppID == "" || !m.FullyInstalled() || isToolApp(m) {
			continue
		}

		g := library.Game{
			Key:       "steam:" + m.AppID,
			Title:     m.Name,
			Provider:  "steam",
			AppID:     m.AppID,
			SizeBytes: m.SizeOnDisk,
			Installed: true,
			Artwork:   artwork(lib, root, m.AppID),
		}
		if g.Title == "" {
			g.Title = m.InstallDir
		}
		if m.InstallDir != "" {
			g.InstallDir = filepath.Join(lib, "steamapps", "common", m.InstallDir)
		}
		if !m.LastPlayed.IsZero() {
			t := m.LastPlayed
			g.LastPlayed = &t
		}
		out = append(out, g)
	}
	return out
}

// artwork 找这部游戏的美术资源。
//
// 先用本地缓存（Steam 把封面放在根目录的 appcache/librarycache 里，
// 某些盘上的库会放在库自己的 appcache 下），本地没有再退回 Steam CDN。
//
// 为什么要退回 CDN：Steam 只缓存它自己库页面加载过的资源，
// 一堆游戏在本机可能只有封面没 hero、或者什么都没有。
func artwork(lib, root, appid string) library.Artwork {
	dirs := []string{
		filepath.Join(root, "appcache", "librarycache", appid),
		filepath.Join(lib, "appcache", "librarycache", appid),
	}

	pick := func(names ...string) string {
		for _, dir := range dirs {
			for _, name := range names {
				p := filepath.Join(dir, name)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
		// 本地没有：给 CDN 地址，页面直接 <img src> 就能拿到
		if len(names) > 0 && cdnAppExists {
			return cdnURL(appid, names[0])
		}
		return ""
	}

	art := library.Artwork{
		Cover: pick("library_600x900.jpg", "library_600x900_2x.jpg", "library_capsule.jpg", "capsule_616x353.jpg", "header.jpg"),
		Hero:  pick("library_hero.jpg", "library_hero_blur.jpg"),
		Logo:  pick("logo.png", "logo_2x.png"),
		Icon:  pick("icon.png"),
	}
	// Icon 不值得走 CDN，本地没有就空着
	if art.Icon == "" {
		art.Icon = ""
	}
	return art
}

// cdnAppExists 留作以后离线开关；现在总是允许 CDN 兜底。
const cdnAppExists = true

// cdnURL 拼 Steam 的公开 CDN 地址（不需要 key，也不需要登录）。
func cdnURL(appid, file string) string {
	return "https://cdn.cloudflare.steamstatic.com/steam/apps/" + appid + "/" + file
}

// toolAppIDs 是每个库里都有、但根本不是游戏的东西（运行库、兼容层）。
var toolAppIDs = map[string]bool{
	"228980":  true, // Steamworks Common Redistributables
	"1070560": true, // Steam Linux Runtime - Scout
	"1391110": true, // Steam Linux Runtime - Soldier
	"1628350": true, // Steam Linux Runtime - Sniper
	"2180100": true, // Proton Hotfix
}

// isToolApp 判断一条清单是不是运行时/工具而不是游戏。
// 启动器里出现「Steamworks Common Redistributables」这种条目很破坏体验。
func isToolApp(m Manifest) bool {
	if toolAppIDs[m.AppID] {
		return true
	}
	name := strings.ToLower(m.Name)
	switch {
	case strings.Contains(name, "steamworks common redistributables"),
		strings.Contains(name, "steam linux runtime"),
		strings.HasPrefix(name, "proton"):
		return true
	}
	return false
}

// RootFromEnv 允许用 LAUNCHER_STEAM_DIR 指定根目录，
// 开发机上没有 Steam、或者库在非默认位置时用。
func RootFromEnv() string {
	return strings.TrimSpace(os.Getenv("LAUNCHER_STEAM_DIR"))
}

// Provider 把 Steam 接成 library.Provider。
type Provider struct{}

func (Provider) ID() string { return "steam" }

func (Provider) Scan(ctx context.Context) ([]library.Game, error) {
	if root := RootFromEnv(); root != "" {
		return Scan(ctx, root)
	}
	return Scan(ctx)
}

func (Provider) Launch(ctx context.Context, g library.Game) error {
	// Steam 的游戏一律交给 Steam 客户端启动：
	// DRM、overlay、云存档、反作弊都靠它接管，直接跑 exe 会出事。
	return launch.Steam(ctx, g.AppID)
}

// dedupe 去掉空串和重复项，保持原有顺序。
func dedupe(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
