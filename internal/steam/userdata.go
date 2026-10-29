package steam

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// PlayStat 是 Steam 本地记的游玩统计与分类。
type PlayStat struct {
	PlaytimeMinutes int
	LastPlayed      time.Time
	Tags            []string
}

// LoadPlayStats 读 userdata 下最近活跃那个用户的 localconfig.vdf，
// 返回 appid → 游玩统计。读不到就返回空 map：它不是关键路径，不该让扫描失败。
//
// 结构：UserLocalConfigStore → Software → Valve → Steam → Apps → <appid>
// 其中 Playtime 是累计分钟数，tags 是 Steam 收藏夹分类名。
func LoadPlayStats(root string) map[string]PlayStat {
	stats := make(map[string]PlayStat)
	path := latestLocalConfig(root)
	if path == "" {
		return stats
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return stats
	}
	obj, err := Parse(data)
	if err != nil {
		return stats
	}

	apps := obj.Obj("UserLocalConfigStore").
		Obj("Software").
		Obj("Valve").
		Obj("Steam").
		Obj("Apps")
	if apps == nil {
		return stats
	}

	for appID, value := range apps {
		app, ok := value.(Object)
		if !ok {
			continue
		}
		stat := PlayStat{
			PlaytimeMinutes: atoi(app.Str("Playtime")),
			LastPlayed:      unix(app.Str("LastPlayed")),
			Tags:            tagsOf(app.Obj("tags")),
		}
		if stat.PlaytimeMinutes == 0 && stat.LastPlayed.IsZero() && len(stat.Tags) == 0 {
			continue
		}
		stats[appID] = stat
	}
	return stats
}

// latestLocalConfig 在 userdata 下挑出最近改过的 localconfig.vdf：
// 一台机器可能有多个账号，用当前正在用的那个。
func latestLocalConfig(root string) string {
	pattern := filepath.Join(root, "userdata", "*", "config", "localconfig.vdf")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	best := ""
	var bestTime time.Time
	for _, path := range matches {
		st, err := os.Stat(path)
		if err != nil {
			continue
		}
		if best == "" || st.ModTime().After(bestTime) {
			best, bestTime = path, st.ModTime()
		}
	}
	return best
}

// tagsOf 把 tags 对象转成分类名列表。
// 老版本 Steam 用数字（对应内置分类），新版本直接存名字。
//
// 排序是必须的：tags 是个 map，Go 的遍历顺序随机，
// 不排的话两次打开同一部游戏，分类顺序会变来变去。
func tagsOf(tags Object) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, 0, len(tags))
	for _, value := range tags {
		if name, ok := value.(string); ok && name != "" {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	sort.Strings(out)
	return out
}
