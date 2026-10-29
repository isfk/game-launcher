package library

import "time"

// DemoGames 是本机扫不到 Steam 库时给界面看的样例数据。
// 全部 Provider 为 "demo"：界面会标注「演示数据」，点它们不会启动任何东西。
func DemoGames() []Game {
	now := time.Now()
	daysAgo := func(d int) *time.Time {
		t := now.AddDate(0, 0, -d)
		return &t
	}
	return []Game{
		{Key: "demo:1", Title: "艾尔登法环", Provider: "demo", AppID: "1245620", Accent: "#c9a227", LastPlayed: daysAgo(1), PlaytimeMinutes: 4210},
		{Key: "demo:2", Title: "赛博朋克 2077", Provider: "demo", AppID: "1091500", Accent: "#e0c341", LastPlayed: daysAgo(3), PlaytimeMinutes: 1680},
		{Key: "demo:3", Title: "哈迪斯 II", Provider: "demo", AppID: "1145350", Accent: "#c0392b", LastPlayed: daysAgo(6), PlaytimeMinutes: 540},
		{Key: "demo:4", Title: "博德之门 3", Provider: "demo", AppID: "1086940", Accent: "#a5642f", LastPlayed: daysAgo(9), PlaytimeMinutes: 6120},
		{Key: "demo:5", Title: "空洞骑士：丝之歌", Provider: "demo", AppID: "1030300", Accent: "#2f8fb0", LastPlayed: daysAgo(14), PlaytimeMinutes: 300},
		{Key: "demo:6", Title: "极乐迪斯科", Provider: "demo", AppID: "632470", Accent: "#5a6a8a", LastPlayed: daysAgo(30), PlaytimeMinutes: 2400},
		{Key: "demo:7", Title: "星露谷物语", Provider: "demo", AppID: "413150", Accent: "#5aa02f", LastPlayed: daysAgo(45), PlaytimeMinutes: 3300},
	}
}
