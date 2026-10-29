package steam

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const localConfig = `"UserLocalConfigStore"
{
	"Software"
	{
		"Valve"
		{
			"Steam"
			{
				"Apps"
				{
					"1245620"
					{
						"LastPlayed"		"1710000000"
						"Playtime"		"4210"
						"tags"
						{
							"0"		"魂类"
							"1"		"想白金"
						}
					}
					"1091500"
					{
						"LastPlayed"		"1700000000"
						"Playtime"		"1680"
					}
				}
			}
		}
	}
}
`

func TestLoadPlayStats(t *testing.T) {
	root := t.TempDir()
	writeLocalConfig(t, root, "12345678", localConfig)

	stats := LoadPlayStats(root)
	if len(stats) != 2 {
		t.Fatalf("应该读到 2 部游戏，实际 %d", len(stats))
	}

	elden, ok := stats["1245620"]
	if !ok {
		t.Fatal("没读到 ELDEN RING 的统计")
	}
	if elden.PlaytimeMinutes != 4210 {
		t.Errorf("Playtime = %d，想要 4210", elden.PlaytimeMinutes)
	}
	if elden.LastPlayed.Unix() != 1710000000 {
		t.Errorf("LastPlayed = %d", elden.LastPlayed.Unix())
	}
	if len(elden.Tags) != 2 {
		t.Fatalf("tags = %v", elden.Tags)
	}
	got := map[string]bool{}
	for _, tag := range elden.Tags {
		got[tag] = true
	}
	if !got["魂类"] || !got["想白金"] {
		t.Errorf("tags = %v", elden.Tags)
	}

	if stats["1091500"].PlaytimeMinutes != 1680 {
		t.Errorf("Cyberpunk 的 Playtime = %d", stats["1091500"].PlaytimeMinutes)
	}
}

func TestLoadPlayStatsPicksMostRecentUser(t *testing.T) {
	root := t.TempDir()
	writeLocalConfig(t, root, "11111111", localConfig)

	// 第二个账号的数据更新一点，应该以它为准
	second := `"UserLocalConfigStore" { "Software" { "Valve" { "Steam" { "Apps" {
		"1245620" { "Playtime" "9" }
	} } } } }`
	writeLocalConfig(t, root, "22222222", second)

	tomorrow := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(root, "userdata", "22222222", "config", "localconfig.vdf"), tomorrow, tomorrow); err != nil {
		t.Fatal(err)
	}

	stats := LoadPlayStats(root)
	if got := stats["1245620"].PlaytimeMinutes; got != 9 {
		t.Fatalf("应该用最近活跃账号的数据（9 分钟），实际 %d", got)
	}
}

func TestLoadPlayStatsMissingIsEmpty(t *testing.T) {
	stats := LoadPlayStats(t.TempDir())
	if len(stats) != 0 {
		t.Fatalf("没有 userdata 时应该返回空 map，实际 %d 条", len(stats))
	}
}

func TestScanMergesPlaytimeAndTags(t *testing.T) {
	root, _ := fixture(t)
	writeLocalConfig(t, root, "12345678", localConfig)

	games, err := Scan(t.Context(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	var elden bool
	for _, g := range games {
		if g.Key != "steam:1245620" {
			continue
		}
		elden = true
		if g.PlaytimeMinutes != 4210 {
			t.Errorf("PlaytimeMinutes = %d，想要 localconfig 里的 4210", g.PlaytimeMinutes)
		}
		if len(g.Tags) != 2 {
			t.Errorf("Tags = %v", g.Tags)
		}
	}
	if !elden {
		t.Fatal("没扫到 ELDEN RING")
	}
}

func writeLocalConfig(t *testing.T, root, userID, content string) {
	t.Helper()
	dir := filepath.Join(root, "userdata", userID, "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "localconfig.vdf"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTagsOfSorted 确认分类名有稳定顺序。
// tags 是 map，遍历顺序随机；不排序会导致同一部游戏两次打开标签顺序不同。
func TestTagsOfSorted(t *testing.T) {
	tags := Object{
		"0": "RPG",
		"1": "开放世界",
		"2": "动作",
		"3": "",
		"4": 42, // 老版本的数字分类，不该当成名字收进来
	}
	got := tagsOf(tags)
	want := []string{"RPG", "动作", "开放世界"}
	if len(got) != len(want) {
		t.Fatalf("tagsOf = %v，期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tagsOf = %v，期望排序后的 %v", got, want)
		}
	}

	// 多跑几次，防的是「排序漏了但这次恰好是顺序的」
	for i := 0; i < 20; i++ {
		again := tagsOf(tags)
		for j := range again {
			if again[j] != got[j] {
				t.Fatalf("第 %d 次遍历顺序变了: %v", i+1, again)
			}
		}
	}
}
