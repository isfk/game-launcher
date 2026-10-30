package epic

// 这些用例都在临时目录里造假清单跑，不需要真的装 Epic：
// 扫描逻辑全在 user 态的 JSON 上，Windows 上的真实目录只影响 DefaultRoots。

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"game-launcher/internal/library"
)

// writeManifest 在临时目录里造一份 .item。name 为空时用 AppName 当文件名。
func writeManifest(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// manifestJSON 拼一份最常见的「装好的游戏」清单。
func manifestJSON(appName, displayName string, extra string) string {
	return `{
  "AppName": "` + appName + `",
  "DisplayName": "` + displayName + `",
  "AppCategories": ["public", "games", "applications"],
  "InstallLocation": "D:\\Epic\\` + displayName + `",
  "LaunchExecutable": "` + appName + `.exe",
  "InstallSize": 12345678,
  "CatalogNamespace": "ns",
  "AppVersionString": "1.0.0"` + extra + `
}`
}

// TestScanOnlyKeepsGames 是这包最要紧的一条：Manifests 目录里躺着的不只有游戏。
func TestScanOnlyKeepsGames(t *testing.T) {
	dir := t.TempDir()

	// 正常的游戏：要
	writeManifest(t, dir, "Fortnite.item", manifestJSON("Fortnite", "Fortnite", ""))
	// DLC / 附加包：不要（MainGameAppName 指向主游戏）
	writeManifest(t, dir, "FortniteDLC.item",
		manifestJSON("FortniteDLC", "Fortnite 附加内容", `, "MainGameAppName": "Fortnite"`))
	// 不是 games 类目（引擎、插件）：不要
	writeManifest(t, dir, "UnrealEngine.item",
		`{"AppName":"UnrealEngine","DisplayName":"Unreal Engine","AppCategories":["public","engines"],"InstallLocation":"D:\\UE"}`)
	// 开发版：不要
	writeManifest(t, dir, "Internal.item",
		manifestJSON("Internal", "内部构建", `, "IsInDevelopment": true`))
	// 没装完：不要
	writeManifest(t, dir, "Broken.item",
		manifestJSON("Broken", "下到一半", `, "bIsIncompleteInstall": true`))
	// 写坏的清单：不要，也不能让整个扫描失败
	writeManifest(t, dir, "Bad.item", `{not json`)
	// 非 .item 文件：直接忽略
	writeManifest(t, dir, "notes.txt", `随便什么`)

	games, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("只该留下 Fortnite，实际 %d 部: %+v", len(games), games)
	}
	g := games[0]
	if g.Key != "epic:Fortnite" {
		t.Errorf("Key = %q", g.Key)
	}
	if g.Title != "Fortnite" || g.Provider != "epic" || g.AppID != "Fortnite" {
		t.Errorf("字段不对: %+v", g)
	}
	if g.SizeBytes != 12345678 {
		t.Errorf("SizeBytes = %d", g.SizeBytes)
	}
	if !g.Installed {
		t.Error("扫出来的应该是已安装")
	}
}

// TestUsableRejectsMissingFields 覆盖缺字段的清单：宁可少一格，不能显示成空白卡。
func TestUsableRejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		m    Manifest
	}{
		{"没有 AppName", Manifest{DisplayName: "X", InstallLocation: "/x", AppCategories: []string{"games"}}},
		{"没有 DisplayName", Manifest{AppName: "x", InstallLocation: "/x", AppCategories: []string{"games"}}},
		{"没有安装目录", Manifest{AppName: "x", DisplayName: "X", AppCategories: []string{"games"}}},
		{"类目里没有 games", Manifest{AppName: "x", DisplayName: "X", InstallLocation: "/x", AppCategories: []string{"public"}}},
	}
	for _, c := range cases {
		if usable(c.m) {
			t.Errorf("%s 应该被跳过", c.name)
		}
	}

	ok := Manifest{AppName: "x", DisplayName: "X", InstallLocation: "/x", AppCategories: []string{"games"}}
	if !usable(ok) {
		t.Error("字段齐备的清单应该通过")
	}
}

// TestScanSortedByTitle 确认顺序稳定：Epic 清单没有「上次游玩」，
// 按标题排至少两次打开顺序一致。
func TestScanSortedByTitle(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "c.item", manifestJSON("c", "Zelda-ish", ""))
	writeManifest(t, dir, "a.item", manifestJSON("a", "alpha", ""))
	writeManifest(t, dir, "b.item", manifestJSON("b", "Beta", ""))

	games, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	want := []string{"alpha", "Beta", "Zelda-ish"}
	if len(games) != len(want) {
		t.Fatalf("数量 = %d", len(games))
	}
	for i := range want {
		if games[i].Title != want[i] {
			t.Errorf("第 %d 个 = %q，期望 %q", i, games[i].Title, want[i])
		}
	}
}

// TestScanMissingDir 确认目录不存在时返回错误（Provider 会容忍它，扫描不会崩）。
func TestScanMissingDir(t *testing.T) {
	if _, err := Scan(context.Background(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Error("目录不存在应该报错")
	}
}

// TestProviderScanUsesEnv 确认 LAUNCHER_EPIC_DIR 能接管目录（开发和测试用）。
func TestProviderScanUsesEnv(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "x.item", manifestJSON("x", "某游戏", ""))
	t.Setenv("LAUNCHER_EPIC_DIR", dir)

	games, err := Provider{}.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 1 || games[0].Title != "某游戏" {
		t.Errorf("没读到环境变量指定的清单: %+v", games)
	}

	// 环境变量指向空目录：不报错，返回空列表（这台机器没装 Epic 的样子）
	t.Setenv("LAUNCHER_EPIC_DIR", t.TempDir())
	games, err = Provider{}.Scan(context.Background())
	if err != nil {
		t.Errorf("空目录不该报错: %v", err)
	}
	if len(games) != 0 {
		t.Errorf("空目录应该得到空列表，实际 %d 部", len(games))
	}
}

// TestURLs 覆盖两个协议：启动与卸载。
func TestURLs(t *testing.T) {
	if got, want := LaunchURL("Fortnite"),
		"com.epicgames.launcher://apps/Fortnite?action=launch&silent=true"; got != want {
		t.Errorf("LaunchURL = %q，期望 %q", got, want)
	}
	if got, want := UninstallURL("Fortnite"),
		"com.epicgames.launcher://apps/Fortnite?action=uninstall&silent=true"; got != want {
		t.Errorf("UninstallURL = %q，期望 %q", got, want)
	}

	// 没有 AppName 的时候要报错，不能拼一个空协议出去
	noAppID := library.Game{Key: "epic:x", Title: "某游戏", Provider: "epic"}
	if err := (Provider{}).Launch(context.Background(), noAppID); err == nil {
		t.Error("缺 AppID 时应该报错")
	}
}

// TestSignatureChangesWithManifest 确认库指纹会跟着清单变化。
// app.go 每 5 秒拍一次，靠它发现「Epic 里刚装完一部游戏」。
func TestSignatureChangesWithManifest(t *testing.T) {
	dir := t.TempDir()
	before := Signature([]string{dir})

	writeManifest(t, dir, "new.item", manifestJSON("new", "新游戏", ""))
	after := Signature([]string{dir})

	if before == after {
		t.Error("加了清单之后指纹应该变")
	}
	if again := Signature([]string{dir}); again != after {
		t.Error("没有变化时指纹应该稳定")
	}
}
