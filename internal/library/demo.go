package library

import "time"

// demoCoverBase 是演示数据借用的封面源：Steam CDN 的库封面。
//
// 为什么借 Steam 的地址：演示数据的目的是让人看清版式和真实库是什么样，
// 一排渐变色块（没有封面）反而像界面坏了。挑的都是 Steam 上有同一部游戏的
// 作品，封面内容对得上游戏本身，不误导。
const demoCoverBase = "https://cdn.cloudflare.steamstatic.com/steam/apps/"

// DemoGames 是本机扫不到任何平台时给界面看的样例数据。
//
// 两条规矩：
//  1. Provider 用**真实平台名**（steam / epic / wegame），不是 "demo" ——
//     这样顶栏统计、平台筛选、图标位置都按真库的样子走，能看出真实版式。
//  2. 每一部都带 Demo = true。名字是假的、库是假的，所以启动 / 卸载 / 打开目录
//     全都会被拒绝（见 app.go 的 Launch / Uninstall / OpenFolder）。
func DemoGames() []Game {
	now := time.Now()
	daysAgo := func(d int) *time.Time {
		t := now.AddDate(0, 0, -d)
		return &t
	}

	// demo 造一部演示游戏。artID 是 Steam appid，只用来拼封面地址；
	// 非 Steam 平台不填 AppID（Epic 那边是 catalog id，两套编号不能混）。
	demo := func(key, title, provider, artID, accent string, days, minutes int, sizeGB float64, tags ...string) Game {
		return Game{
			Key:      key,
			Title:    title,
			Provider: provider,
			AppID:    steamAppID(provider, artID),
			Artwork: Artwork{
				Cover: demoCoverBase + artID + "/library_600x900.jpg",
				Hero:  demoCoverBase + artID + "/library_hero.jpg",
			},
			Accent:          accent,
			LastPlayed:      daysAgo(days),
			PlaytimeMinutes: minutes,
			SizeBytes:       int64(sizeGB * (1 << 30)),
			Tags:            tags,
			Installed:       true,
			Demo:            true,
		}
	}

	return []Game{
		demo("demo:steam:1245620", "艾尔登法环", "steam", "1245620", "#c9a227", 1, 4210, 60.2, "魂系"),
		demo("demo:steam:1086940", "博德之门 3", "steam", "1086940", "#a5642f", 3, 6120, 122.4, "RPG"),
		demo("demo:steam:1091500", "赛博朋克 2077", "steam", "1091500", "#e0c341", 6, 1680, 70.8),
		demo("demo:steam:1145350", "哈迪斯 II", "steam", "1145350", "#c0392b", 14, 540, 8.6),

		demo("demo:epic:1087100", "心灵杀手 2", "epic", "1087100", "#6c7f9a", 9, 960, 90.0, "Epic 独占"),
		demo("demo:epic:1850570", "死亡搁浅 导演剪辑版", "epic", "1850570", "#4f7f8a", 30, 3300, 80.1),

		demo("demo:wegame:2358720", "黑神话：悟空", "wegame", "2358720", "#b8863b", 21, 2100, 130.0, "国产"),
		demo("demo:wegame:838350", "太吾绘卷", "wegame", "838350", "#7a8f4f", 45, 1500, 6.2, "国产"),
	}
}

// steamAppID 只给 Steam 那一批填 AppID：其他平台的编号体系和 Steam 不同，
// 混填会让「用商店图兜底」查到一部完全不相干的游戏。
func steamAppID(provider, artID string) string {
	if provider == "steam" {
		return artID
	}
	return ""
}
