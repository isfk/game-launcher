package steam

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture 造一棵假的 Steam 目录树：两个库、三部游戏（其中一部没装完）。
// 返回根目录与第二个库的路径。
func fixture(t *testing.T) (root string, lib2 string) {
	t.Helper()
	dir := t.TempDir()
	root = filepath.Join(dir, "Steam")
	lib2 = filepath.Join(dir, "SteamLibrary")
	mustDir(t, filepath.Join(root, "steamapps"))
	mustDir(t, filepath.Join(lib2, "steamapps"))

	// 库清单：根目录 + 第二个库
	mustWrite(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders"
{
	"0" { "path" "`+root+`" }
	"1" { "path" "`+lib2+`" }
}
`)

	// 根目录里两部：装上的一部、下载中的一部
	mustWrite(t, filepath.Join(root, "steamapps", "appmanifest_1245620.acf"), manifestACF(
		"1245620", "ELDEN RING", "ELDEN RING", 4, 1710000000))
	mustWrite(t, filepath.Join(root, "steamapps", "appmanifest_999.acf"), manifestACF(
		"999", "Downloading", "downloading", 2, 1700000000))

	// 每个库里都有、但不是游戏的东西
	mustWrite(t, filepath.Join(root, "steamapps", "appmanifest_228980.acf"), manifestACF(
		"228980", "Steamworks Common Redistributables", "Steamworks Shared", 4, 1700000000))

	// 第二个库里一部
	mustWrite(t, filepath.Join(lib2, "steamapps", "appmanifest_1091500.acf"), manifestACF(
		"1091500", "Cyberpunk 2077", "Cyberpunk 2077", 4, 1600000000))

	// 封面缓存放在根目录的 appcache 里
	artDir := filepath.Join(root, "appcache", "librarycache", "1245620")
	mustDir(t, artDir)
	mustWrite(t, filepath.Join(artDir, "library_600x900.jpg"), "cover")
	mustWrite(t, filepath.Join(artDir, "library_hero.jpg"), "hero")

	return root, lib2
}

func TestScanFindsInstalledGamesAcrossLibraries(t *testing.T) {
	root, _ := fixture(t)

	games, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("应该扫到 2 部游戏（运行库要被过滤），实际 %d 部: %+v", len(games), games)
	}

	// 按上次游玩时间从新到旧
	if games[0].Key != "steam:1245620" {
		t.Errorf("第一部应该是最近玩过的 ELDEN RING，实际 %q", games[0].Key)
	}
	if games[1].Key != "steam:1091500" {
		t.Errorf("第二部应该是 Cyberpunk 2077，实际 %q", games[1].Key)
	}

	elden := games[0]
	if elden.Title != "ELDEN RING" {
		t.Errorf("Title = %q", elden.Title)
	}
	if elden.Provider != "steam" || !elden.Installed {
		t.Errorf("Provider/Installed = %q/%v", elden.Provider, elden.Installed)
	}
	if elden.InstallDir == "" || filepath.Base(elden.InstallDir) != "ELDEN RING" {
		t.Errorf("InstallDir = %q", elden.InstallDir)
	}
	if elden.Artwork.Cover == "" || elden.Artwork.Hero == "" {
		t.Errorf("封面/背景没找到: %+v", elden.Artwork)
	}
	if !strings.HasPrefix(elden.Artwork.Logo, "https://cdn.cloudflare.steamstatic.com/steam/apps/1245620/") {
		t.Errorf("本地没有 logo 时应该退回 CDN: %q", elden.Artwork.Logo)
	}
	if elden.LastPlayed == nil || elden.LastPlayed.Unix() != 1710000000 {
		t.Errorf("LastPlayed = %v", elden.LastPlayed)
	}

	// 第二部在另一个库里、本机没有缓存 → 走 CDN
	if !strings.HasPrefix(games[1].Artwork.Cover, "https://cdn.cloudflare.steamstatic.com/steam/apps/1091500/") {
		t.Errorf("本机没缓存时应该退回 CDN: %q", games[1].Artwork.Cover)
	}
	if !strings.HasPrefix(games[1].Artwork.Hero, "https://") {
		t.Errorf("hero 也该退回 CDN: %q", games[1].Artwork.Hero)
	}
}

func TestScanEmptyRootIsNotAnError(t *testing.T) {
	games, err := Scan(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("空目录不该报错: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("空目录应该是空库，实际 %d 部", len(games))
	}
}

func TestScanHonorsContextCancel(t *testing.T) {
	root, _ := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Scan(ctx, root); err == nil {
		t.Fatal("已经取消的 ctx 应该返回错误")
	}
}

func TestProviderUsesEnvRoot(t *testing.T) {
	root, _ := fixture(t)
	t.Setenv("LAUNCHER_STEAM_DIR", root)

	games, err := (Provider{}).Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("环境变量指定的库应该有 2 部，实际 %d", len(games))
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"", "/a", "/a", "/b", ""})
	want := []string{"/a", "/b"}
	if len(got) != len(want) {
		t.Fatalf("dedupe = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("dedupe = %v", got)
		}
	}
}

func manifestACF(appID, name, installDir string, stateFlags int, lastPlayed int64) string {
	return `"AppState"
{
	"appid"		"` + appID + `"
	"name"		"` + name + `"
	"StateFlags"		"` + itoa(stateFlags) + `"
	"installdir"		"` + installDir + `"
	"SizeOnDisk"		"1024"
	"LastUpdated"		"` + itoa(int(lastPlayed)) + `"
	"LastPlayed"		"` + itoa(int(lastPlayed)) + `"
}
`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func mustDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestIsToolApp(t *testing.T) {
	cases := []struct {
		name string
		m    Manifest
		want bool
	}{
		{"运行库按 appid", Manifest{AppID: "228980", Name: "whatever"}, true},
		{"Linux 运行时", Manifest{AppID: "1", Name: "Steam Linux Runtime - Soldier"}, true},
		{"Proton", Manifest{AppID: "2", Name: "Proton Experimental"}, true},
		{"真游戏", Manifest{AppID: "1245620", Name: "ELDEN RING"}, false},
		{"名字里带 steam 但不是工具", Manifest{AppID: "3", Name: "SteamWorld Dig"}, false},
	}
	for _, c := range cases {
		if got := isToolApp(c.m); got != c.want {
			t.Errorf("%s: isToolApp = %v，想要 %v", c.name, got, c.want)
		}
	}
}

// TestBlankLogoIsDropped 覆盖「白条」问题：logo.png 若是一整片纯白，
// 画到界面上就是一条白条，比没有 logo 还难看 —— 这种要当它不存在。
func TestBlankLogoIsDropped(t *testing.T) {
	root := t.TempDir()
	appID := "1245620"

	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `"AppState" { "appid" "` + appID + `" "name" "白 Logo 测试" "StateFlags" "4" "installdir" "WhiteLogo" }`
	if err := os.WriteFile(filepath.Join(root, "steamapps", "appmanifest_"+appID+".acf"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := filepath.Join(root, "appcache", "librarycache", appID)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 一整片纯白的 logo
	white := image.NewNRGBA(image.Rect(0, 0, 360, 90))
	for i := range white.Pix {
		white.Pix[i] = 0xff
	}
	logoFile, err := os.Create(filepath.Join(cacheDir, "logo.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(logoFile, white); err != nil {
		t.Fatal(err)
	}
	logoFile.Close()

	games, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("应该扫到 1 部，实际 %d", len(games))
	}
	if got := games[0].Artwork.Logo; got != "" {
		t.Errorf("纯白 logo 应该被丢掉，实际 %q", got)
	}
}
