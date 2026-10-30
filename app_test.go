package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"game-launcher/internal/config"
	"game-launcher/internal/library"
	"game-launcher/internal/pad"

	"github.com/egoist/mygo"
)

// TestLibraryFallsBackToDemo 覆盖「这台机器没装 Steam」的情况：
// 后端不报错，给出演示数据，界面照常能看。
func TestLibraryFallsBackToDemo(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	l := &Launcher{}
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if res.Source != "demo" {
		t.Errorf("Source = %q，空库应该退回 demo", res.Source)
	}
	if len(res.Games) == 0 {
		t.Fatal("演示数据不该是空的")
	}
	// 演示数据用真平台名排开（顶栏统计才像真库），所以靠 Demo 位认它
	providers := make(map[string]int)
	for _, g := range res.Games {
		if !g.Demo {
			t.Errorf("%q 是演示数据，Demo 应该为 true", g.Title)
		}
		providers[g.Provider]++
	}
	for _, want := range []string{"steam", "epic", "wegame"} {
		if providers[want] == 0 {
			t.Errorf("演示数据应该覆盖 %s，实际只有 %v", want, providers)
		}
	}
	if res.Games[0].Artwork.Cover == "" {
		t.Error("演示数据应该有封面地址，否则首页是一排渐变色块")
	}
	if res.ScannedAt.IsZero() {
		t.Error("ScannedAt 应该被填上")
	}
	if len(res.Roots) == 0 {
		t.Error("Roots 应该至少报出环境变量指定的目录，方便排查")
	}
}

// TestLibraryCachesUntilRefresh 确认 refresh=false 不重复扫盘。
func TestLibraryCachesUntilRefresh(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	l := &Launcher{}
	first, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	second, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if !second.ScannedAt.Equal(first.ScannedAt) {
		t.Error("第二次调用应该复用上次扫描结果")
	}

	refreshed, err := l.Library(context.Background(), true)
	if err != nil {
		t.Fatalf("Library(refresh): %v", err)
	}
	if refreshed.ScannedAt.Before(first.ScannedAt) {
		t.Error("refresh=true 应该重新扫描")
	}
}

// TestLibraryReadsSteamLibrary 用一棵假 Steam 库验证真实路径也会被认出来。
func TestLibraryReadsSteamLibrary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `"AppState" { "appid" "1245620" "name" "ELDEN RING" "StateFlags" "4" "installdir" "ELDEN RING" "LastPlayed" "1710000000" }`
	if err := os.WriteFile(filepath.Join(root, "steamapps", "appmanifest_1245620.acf"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_STEAM_DIR", root)

	l := &Launcher{}
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if res.Source != "steam" {
		t.Fatalf("Source = %q，应该认成 steam", res.Source)
	}
	if len(res.Games) != 1 || res.Games[0].Key != "steam:1245620" {
		t.Fatalf("扫到的游戏不对: %+v", res.Games)
	}
}

func TestLaunchReportsUnknownKey(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	l := &Launcher{}
	if _, err := l.Library(context.Background(), false); err != nil {
		t.Fatalf("Library: %v", err)
	}
	if _, err := l.Launch(context.Background(), "steam:404"); err == nil {
		t.Fatal("不存在的 key 应该报错")
	}
}

func TestLaunchRefusesDemoEntry(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	l := &Launcher{}
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if _, err := l.Launch(context.Background(), res.Games[0].Key); err == nil {
		t.Fatal("演示数据不该能被启动")
	}
}

// TestUninstallAsksSteam 验证卸载走的是 Steam 的协议，而不是我们自己删文件。
func TestUninstallAsksSteam(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `"AppState" { "appid" "1245620" "name" "ELDEN RING" "StateFlags" "4" "installdir" "ELDEN RING" }`
	if err := os.WriteFile(filepath.Join(root, "steamapps", "appmanifest_1245620.acf"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_STEAM_DIR", root)

	var gotURL string
	restore := swapOpener(func(_ context.Context, url string) error {
		gotURL = url
		return nil
	})
	defer restore()

	l := &Launcher{}
	if _, err := l.Library(context.Background(), false); err != nil {
		t.Fatalf("Library: %v", err)
	}

	title, err := l.Uninstall(context.Background(), "steam:1245620")
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if title != "ELDEN RING" {
		t.Errorf("返回的名字 = %q", title)
	}
	if gotURL != "steam://uninstall/1245620" {
		t.Errorf("打开的 URL = %q，应该是 steam://uninstall/1245620", gotURL)
	}
}

// TestUninstallRejectsUnknownAndDemo 覆盖两种不该动手的情况。
func TestUninstallRejectsUnknownAndDemo(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	called := false
	restore := swapOpener(func(context.Context, string) error {
		called = true
		return nil
	})
	defer restore()

	l := &Launcher{}
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if _, err := l.Uninstall(context.Background(), "steam:404"); err == nil {
		t.Error("不存在的 key 应该报错")
	}
	if _, err := l.Uninstall(context.Background(), res.Games[0].Key); err == nil {
		t.Error("演示数据不该能被卸载")
	}
	if called {
		t.Error("这两种情况都不该去开 URL")
	}
}

// swapOpener 换掉 openURL，返回还原函数。
func swapOpener(fn func(context.Context, string) error) func() {
	old := openURL
	openURL = fn
	return func() { openURL = old }
}

// TestSettingsPersistSoundAndFavorites 覆盖设置落盘：开关和收藏都要跨重启保留。
func TestSettingsPersistSoundAndFavorites(t *testing.T) {
	dir := t.TempDir()

	first := &Launcher{}
	first.setConfig(config.NewStore(dir))
	if got := first.Settings(); !got.SoundEnabled {
		t.Error("默认应该开着音效")
	}
	if _, err := first.SetSoundEnabled(false); err != nil {
		t.Fatalf("SetSoundEnabled: %v", err)
	}
	if _, err := first.SetFavorite("steam:1245620", true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	if _, err := first.SetFavorite("steam:1091500", true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	// 再取消一个，验证是「按 key 覆盖」而不是越加越多
	if _, err := first.SetFavorite("steam:1091500", false); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}

	// 第二个实例 = 重启应用
	second := &Launcher{}
	second.setConfig(config.NewStore(dir))
	got := second.Settings()
	if got.SoundEnabled {
		t.Error("重启后音效应该还是关的")
	}
	if len(got.Favorites) != 1 || got.Favorites[0] != "steam:1245620" {
		t.Errorf("重启后收藏 = %v，应该只剩 steam:1245620", got.Favorites)
	}
}

// TestSettingsFallBackWhenUnreadable 设置文件坏了也得能起来。
func TestSettingsFallBackWhenUnreadable(t *testing.T) {
	dir := t.TempDir()
	store := config.NewStore(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), []byte("{ 半个"), 0o644); err != nil {
		t.Fatal(err)
	}

	l := &Launcher{}
	l.setConfig(store)
	if got := l.Settings(); !got.SoundEnabled {
		t.Error("坏文件应该回默认值")
	}
}

// TestSetFavoriteRequiresKey 空 key 是编程错误，不能悄悄写进文件。
func TestSetFavoriteRequiresKey(t *testing.T) {
	l := &Launcher{}
	if _, err := l.SetFavorite("", true); err == nil {
		t.Error("空 key 应该报错")
	}
}

// TestProfileDefaultsToSystemUser 覆盖「第一次启动」：没有昵称就用系统账户名，
// 不该是写死的 Master。
func TestProfileDefaultsToSystemUser(t *testing.T) {
	t.Setenv("USER", "neo")
	t.Setenv("USERNAME", "neo")
	t.Setenv("LOGNAME", "neo")

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	p := l.Profile()
	if p.Name != "neo" {
		t.Errorf("默认昵称 = %q，期望系统账户名 neo", p.Name)
	}
	if p.Avatar != "" {
		t.Errorf("没设头像时应该是空串，实际 %q", p.Avatar)
	}
}

// TestSetNicknameNormalizesAndPersists 昵称要跨重启保留，并且收敛掉多余的空白。
func TestSetNicknameNormalizesAndPersists(t *testing.T) {
	t.Setenv("USER", "neo")
	dir := t.TempDir()

	first := &Launcher{}
	first.setConfig(config.NewStore(dir))
	p, err := first.SetNickname("  阿伞\n ") // 用户可能从别处粘进来带空行
	if err != nil {
		t.Fatalf("SetNickname: %v", err)
	}
	if p.Name != "阿伞" {
		t.Errorf("昵称 = %q，期望 阿伞", p.Name)
	}

	// 重启一次：还是这个昵称
	second := &Launcher{}
	second.setConfig(config.NewStore(dir))
	if got := second.Profile().Name; got != "阿伞" {
		t.Errorf("重启后昵称 = %q", got)
	}

	// 清空 = 回到系统账户名
	if got, err := second.SetNickname("   "); err != nil {
		t.Fatalf("SetNickname: %v", err)
	} else if got.Name != "neo" {
		t.Errorf("清空昵称后 = %q，期望回落到 neo", got.Name)
	}
}

// swapImagePicker 换掉头像的文件选择框，返回还原函数。
func swapImagePicker(fn func(*mygo.Window) (string, error)) func() {
	old := pickImage
	pickImage = fn
	return func() { pickImage = old }
}

// fakePNG 造一张真的 1x1 PNG（内容是图片，不是改名文本）。
func fakePNG(t *testing.T) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "avatar.png")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestPickAvatarCopiesIntoConfig 覆盖头像：选中的图复制进配置目录，页面拿到 URL。
func TestPickAvatarCopiesIntoConfig(t *testing.T) {
	dir := t.TempDir()
	image := fakePNG(t)

	restore := swapImagePicker(func(*mygo.Window) (string, error) { return image, nil })
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(dir))
	p, err := l.PickAvatar(context.Background())
	if err != nil {
		t.Fatalf("PickAvatar: %v", err)
	}
	if p.Avatar == "" {
		t.Fatal("选完头像后应该有一个可加载的 URL")
	}
	if !strings.HasPrefix(p.Avatar, "art://localhost/") && !strings.HasPrefix(p.Avatar, "http://art.localhost/") {
		t.Errorf("头像 URL 不像 art:// 地址: %q", p.Avatar)
	}

	// 落盘的是复制件，不是原图路径
	stored := l.Settings().Avatar
	if !strings.HasPrefix(stored, dir) {
		t.Errorf("头像副本应该落在配置目录里，实际 %q", stored)
	}
	if stored == image {
		t.Error("不该直接记住原图路径（原图挪走头像就白了）")
	}

	// 重启后还是同一个头像（同一个文件 → 同一个 URL）
	again := &Launcher{}
	again.setConfig(config.NewStore(dir))
	if got := again.Profile().Avatar; got != p.Avatar {
		t.Errorf("重启后头像 = %q，期望 %q", got, p.Avatar)
	}

	// 取消选择不该弄丢已有的头像
	restore()
	swapImagePicker(func(*mygo.Window) (string, error) { return "", nil })
	if got, err := again.PickAvatar(context.Background()); err != nil {
		t.Fatalf("取消不该报错: %v", err)
	} else if got.Avatar != p.Avatar {
		t.Errorf("取消后头像变了: %q", got.Avatar)
	}
}

// TestClearAvatarFallsBackToInitial 清掉头像 = 回到「昵称首字 + 渐变」。
func TestClearAvatarFallsBackToInitial(t *testing.T) {
	dir := t.TempDir()
	restore := swapImagePicker(func(*mygo.Window) (string, error) { return fakePNG(t), nil })
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(dir))
	if _, err := l.PickAvatar(context.Background()); err != nil {
		t.Fatalf("PickAvatar: %v", err)
	}

	p, err := l.ClearAvatar()
	if err != nil {
		t.Fatalf("ClearAvatar: %v", err)
	}
	if p.Avatar != "" {
		t.Errorf("清掉后不该还有头像，实际 %q", p.Avatar)
	}
	if l.Settings().Avatar != "" {
		t.Error("清掉后设置里该是空串")
	}
}

// fakeExecutable 造一个能被 ValidPath 接受的可执行文件。
func fakeExecutable(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	script := "#!/bin/sh\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// swapPicker 换掉文件选择框，返回还原函数。
func swapPicker(fn func(*mygo.Window) (string, error)) func() {
	old := pickExecutable
	pickExecutable = fn
	return func() { pickExecutable = old }
}

// TestAddManualGamePersists 覆盖手动添加：选中的文件变成库里的第一张卡，
// 并且真的写进了 settings.json（重启还在）。
func TestAddManualGamePersists(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())
	dir := t.TempDir()
	exe := fakeExecutable(t, "MyGame.sh")

	restore := swapPicker(func(*mygo.Window) (string, error) { return exe, nil })
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(dir))

	res, err := l.AddManualGame(context.Background())
	if err != nil {
		t.Fatalf("AddManualGame: %v", err)
	}
	if len(res.Games) == 0 || res.Games[0].Provider != "manual" {
		t.Fatalf("手动添加的游戏应该排在第一位，实际 %+v", res.Games)
	}
	if res.Games[0].Title != "MyGame" {
		t.Errorf("Title = %q，期望从文件名推出 MyGame", res.Games[0].Title)
	}

	// 重启一次：读同一份配置，游戏还在
	again := &Launcher{}
	again.setConfig(config.NewStore(dir))
	res2, err := again.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if len(res2.Games) == 0 || res2.Games[0].Key != res.Games[0].Key {
		t.Errorf("重启后手动游戏丢了: %+v", res2.Games)
	}
}

// TestAddManualGameIgnoresCancelAndDuplicates 覆盖两种「什么也没发生」：
// 取消选择、以及同一个文件加第二次。
func TestAddManualGameIgnoresCancelAndDuplicates(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())
	exe := fakeExecutable(t, "Dup.sh")

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	// 取消
	restore := swapPicker(func(*mygo.Window) (string, error) { return "", nil })
	if _, err := l.AddManualGame(context.Background()); err != nil {
		t.Fatalf("取消不该报错: %v", err)
	}
	restore()

	// 加两次同一个文件
	restore = swapPicker(func(*mygo.Window) (string, error) { return exe, nil })
	defer restore()
	for i := 0; i < 2; i++ {
		if _, err := l.AddManualGame(context.Background()); err != nil {
			t.Fatalf("第 %d 次添加: %v", i+1, err)
		}
	}
	if got := len(l.Settings().ManualGames); got != 1 {
		t.Errorf("手动记录 = %d 条，重复添加不该产生第二条", got)
	}
}

// TestAddManualGameRejectsMissingFile 确认选中一个已经被删掉的文件会报错。
func TestAddManualGameRejectsMissingFile(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	restore := swapPicker(func(*mygo.Window) (string, error) {
		return filepath.Join(t.TempDir(), "gone.exe"), nil
	})
	defer restore()

	if _, err := l.AddManualGame(context.Background()); err == nil {
		t.Error("路径不存在应该报错")
	}
}

// TestRemoveManualGameDropsFavoriteToo 覆盖移除：记录没了，收藏里的残留也清掉，
// 而且不改磁盘上的文件。
func TestRemoveManualGameDropsFavoriteToo(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())
	exe := fakeExecutable(t, "Bye.sh")

	restore := swapPicker(func(*mygo.Window) (string, error) { return exe, nil })
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.AddManualGame(context.Background())
	if err != nil {
		t.Fatalf("AddManualGame: %v", err)
	}
	key := res.Games[0].Key
	if _, err := l.SetFavorite(key, true); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}

	after, err := l.RemoveManualGame(context.Background(), key)
	if err != nil {
		t.Fatalf("RemoveManualGame: %v", err)
	}
	for _, g := range after.Games {
		if g.Key == key {
			t.Error("移除后不该还在库里")
		}
	}
	if got := l.Settings().Favorites; len(got) != 0 {
		t.Errorf("收藏里残留了 %v", got)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Errorf("不该动磁盘上的文件: %v", err)
	}

	// 再移除一次应该报错（说明它真的不在了）
	if _, err := l.RemoveManualGame(context.Background(), key); err == nil {
		t.Error("移除不存在的记录应该报错")
	}
}

// TestLaunchManualGameOpensPath 确认手动游戏是「把路径交给系统」，
// 而不是拼一个 steam:// 出来。
func TestLaunchManualGameOpensPath(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())
	exe := fakeExecutable(t, "Run.sh")

	restorePicker := swapPicker(func(*mygo.Window) (string, error) { return exe, nil })
	defer restorePicker()

	var gotTarget string
	restoreOpen := swapOpener(func(_ context.Context, target string) error {
		gotTarget = target
		return nil
	})
	defer restoreOpen()

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.AddManualGame(context.Background())
	if err != nil {
		t.Fatalf("AddManualGame: %v", err)
	}

	// 别让「等游戏退出」的协程把测试拖住
	watchProcesses = false
	defer func() { watchProcesses = true }()

	title, err := l.Launch(context.Background(), res.Games[0].Key)
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if title != "Run" {
		t.Errorf("返回的名字 = %q", title)
	}
	if gotTarget != exe {
		t.Errorf("打开的目标 = %q，期望就是选中的路径 %q", gotTarget, exe)
	}
}

// TestUninstallRefusesManual 手动添加的游戏不能走 Steam 卸载。
func TestUninstallRefusesManual(t *testing.T) {
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())
	exe := fakeExecutable(t, "Keep.sh")

	restore := swapPicker(func(*mygo.Window) (string, error) { return exe, nil })
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.AddManualGame(context.Background())
	if err != nil {
		t.Fatalf("AddManualGame: %v", err)
	}

	if _, err := l.Uninstall(context.Background(), res.Games[0].Key); err == nil {
		t.Error("手动游戏不该能被卸载")
	}
}

// TestGatePadStates 是「游戏里按 A 不该触发启动」这条的核心：
// 窗口不在前台时，手柄状态一律当作空，界面收不到任何按键。
func TestGatePadStates(t *testing.T) {
	held := func() []pad.State {
		return []pad.State{{Name: "Xbox 手柄", Buttons: []string{"south"}, Connected: true}}
	}

	if got := gatePadStates(false, held); len(got) != 0 {
		t.Errorf("失焦时应该返回空状态，实际 %+v", got)
	}
	if got := gatePadStates(true, held); len(got) != 1 {
		t.Errorf("在前台时应该原样返回，实际 %+v", got)
	}
}

// TestFocusedDefaultsToOn 确认没有窗口（测试环境）时不会把柄功能关掉。
func TestFocusedDefaultsToOn(t *testing.T) {
	l := &Launcher{}
	l.SetFocused(true)
	if !l.focusedNow() {
		t.Error("没挂窗口时应该按事件记下的值来")
	}
	l.SetFocused(false)
	if l.focusedNow() {
		t.Error("SetFocused(false) 之后应该报告失焦")
	}
}

// TestPadStatesFollowsFocus 端到端地把「焦点 → 手柄状态」串一遍。
func TestPadStatesFollowsFocus(t *testing.T) {
	l := &Launcher{}
	l.SetFocused(false)
	if got := l.padStates(); len(got) != 0 {
		t.Errorf("失焦时 padStates 应该为空，实际 %+v", got)
	}
	// 回到前台不 panic 就行：真手柄状态取决于这台机器接了什么
	l.SetFocused(true)
	_ = l.padStates()
}

// TestLaunchRefusesWhileStarting 挡住「连点两下 A」：第一次请求还没落地时，
// 第二次直接拒绝，不会真的开两份游戏。
func TestLaunchRefusesWhileStarting(t *testing.T) {
	l := &Launcher{}
	l.games = []library.Game{{Key: "steam:1", Title: "ELDEN RING", Provider: "steam"}}
	l.starting = true

	if _, err := l.Launch(context.Background(), "steam:1"); err == nil {
		t.Fatal("正在启动时不该允许再启动一次")
	}
}

// TestLaunchRefusesWhileGameRunning 挡住「已经开着还按 A」。
func TestLaunchRefusesWhileGameRunning(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	l := &Launcher{}
	l.games = []library.Game{
		{Key: "steam:1", Title: "ELDEN RING", Provider: "steam"},
		{Key: "steam:2", Title: "Cyberpunk 2077", Provider: "steam"},
	}
	l.runningKey = "steam:1"

	_, err := l.Launch(context.Background(), "steam:2")
	if err == nil {
		t.Fatal("有游戏在跑时不该允许启动别的")
	}
	if !strings.Contains(err.Error(), "ELDEN RING") {
		t.Errorf("错误里该说清是哪部在跑，实际 %v", err)
	}
}

// TestWatchAfterLaunchClearsStarting 确认不守护进程时，「正在启动」不会卡死在那。
func TestWatchAfterLaunchClearsStarting(t *testing.T) {
	watchProcesses = false
	defer func() { watchProcesses = true }()

	l := &Launcher{}
	l.starting = true
	l.watchAfterLaunch(library.Game{Key: "steam:1"})
	if l.starting {
		t.Error("不守护进程时应该立刻放开 starting，否则再也启动不了")
	}
}

// TestEpicGamesMergedIntoLibrary 覆盖多平台合并：Steam 和 Epic 各贡献几部，
// source 要报出两个平台（界面据此显示 "Steam + Epic"）。
func TestEpicGamesMergedIntoLibrary(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")

	steamRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(steamRoot, "steamapps"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `"AppState" { "appid" "1245620" "name" "ELDEN RING" "StateFlags" "4" "installdir" "ELDEN RING" }`
	if err := os.WriteFile(filepath.Join(steamRoot, "steamapps", "appmanifest_1245620.acf"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_STEAM_DIR", steamRoot)

	epicDir := t.TempDir()
	item := `{"AppName":"Fortnite","DisplayName":"Fortnite","AppCategories":["public","games"],"InstallLocation":"D:\\Epic\\Fortnite","InstallSize":100}`
	if err := os.WriteFile(filepath.Join(epicDir, "Fortnite.item"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_EPIC_DIR", epicDir)

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if res.Source != "steam+epic" {
		t.Errorf("Source = %q，期望 steam+epic", res.Source)
	}

	providers := map[string]int{}
	for _, g := range res.Games {
		providers[g.Provider]++
	}
	if providers["steam"] != 1 || providers["epic"] != 1 {
		t.Errorf("两个平台各该有一部，实际 %+v", providers)
	}

	// Roots 里要能看见 Epic 的目录，扫不到时好排查
	found := false
	for _, r := range res.Roots {
		if r == epicDir {
			found = true
		}
	}
	if !found {
		t.Errorf("Roots 里应该包含 Epic 目录: %v", res.Roots)
	}
}

// TestEpicOnlyLibrary 确认只装了 Epic 时不会退回演示数据。
func TestEpicOnlyLibrary(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	epicDir := t.TempDir()
	item := `{"AppName":"AlanWake2","DisplayName":"Alan Wake 2","AppCategories":["games"],"InstallLocation":"D:\\Epic\\AW2"}`
	if err := os.WriteFile(filepath.Join(epicDir, "AlanWake2.item"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_EPIC_DIR", epicDir)

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}
	if res.Source != "epic" {
		t.Errorf("Source = %q，期望 epic", res.Source)
	}
	if len(res.Games) != 1 || res.Games[0].Title != "Alan Wake 2" {
		t.Errorf("游戏列表不对: %+v", res.Games)
	}
}

// TestLaunchEpicUsesProtocol 确认 Epic 游戏走客户端的协议启动。
func TestLaunchEpicUsesProtocol(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	epicDir := t.TempDir()
	item := `{"AppName":"Fortnite","DisplayName":"Fortnite","AppCategories":["games"],"InstallLocation":"D:\\Epic\\Fortnite"}`
	if err := os.WriteFile(filepath.Join(epicDir, "Fortnite.item"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_EPIC_DIR", epicDir)

	var gotTarget string
	restore := swapOpener(func(_ context.Context, target string) error {
		gotTarget = target
		return nil
	})
	defer restore()

	watchProcesses = false
	defer func() { watchProcesses = true }()

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}

	if _, err := l.Launch(context.Background(), res.Games[0].Key); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	want := "com.epicgames.launcher://apps/Fortnite?action=launch&silent=true"
	if gotTarget != want {
		t.Errorf("打开的目标 = %q，期望 %q", gotTarget, want)
	}
}

// TestUninstallEpicUsesProtocol 确认 Epic 的卸载也交给客户端。
func TestUninstallEpicUsesProtocol(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	epicDir := t.TempDir()
	item := `{"AppName":"Fortnite","DisplayName":"Fortnite","AppCategories":["games"],"InstallLocation":"D:\\Epic\\Fortnite"}`
	if err := os.WriteFile(filepath.Join(epicDir, "Fortnite.item"), []byte(item), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LAUNCHER_EPIC_DIR", epicDir)

	var gotTarget string
	restore := swapOpener(func(_ context.Context, target string) error {
		gotTarget = target
		return nil
	})
	defer restore()

	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))
	res, err := l.Library(context.Background(), false)
	if err != nil {
		t.Fatalf("Library: %v", err)
	}

	title, err := l.Uninstall(context.Background(), res.Games[0].Key)
	if err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if title != "Fortnite" {
		t.Errorf("返回的名字 = %q", title)
	}
	want := "com.epicgames.launcher://apps/Fortnite?action=uninstall&silent=true"
	if gotTarget != want {
		t.Errorf("打开的 URL = %q，期望 %q", gotTarget, want)
	}
}

// TestSetIconPosValidates 覆盖图标位置的落盘与校验：
// 传什么都要收敛到四个合法值之一，坏值回默认。
func TestSetIconPosValidates(t *testing.T) {
	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	if got := l.Settings().IconPos; got != config.IconPosBottomLeft {
		t.Errorf("默认图标位置 = %q，期望左下", got)
	}

	for _, pos := range []string{"tl", "tr", "br"} {
		s, err := l.SetIconPos(pos)
		if err != nil {
			t.Fatalf("SetIconPos(%q): %v", pos, err)
		}
		if s.IconPos != pos {
			t.Errorf("SetIconPos(%q) → %q", pos, s.IconPos)
		}
	}

	s, err := l.SetIconPos("随便写的")
	if err != nil {
		t.Fatalf("坏值不该报错: %v", err)
	}
	if s.IconPos != config.IconPosBottomLeft {
		t.Errorf("坏值应该回落到左下，实际 %q", s.IconPos)
	}
}

// TestSetHiddenToggles 覆盖「只在启动器里隐藏」：加、去重、恢复，
// 并且全程不碰客户端也不碰磁盘。
func TestSetHiddenToggles(t *testing.T) {
	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	s, err := l.SetHidden("steam:1", true)
	if err != nil {
		t.Fatalf("SetHidden: %v", err)
	}
	if len(s.Hidden) != 1 || s.Hidden[0] != "steam:1" {
		t.Fatalf("隐藏列表 = %v", s.Hidden)
	}

	// 重复隐藏不该出现两条
	s, err = l.SetHidden("steam:1", true)
	if err != nil {
		t.Fatalf("SetHidden: %v", err)
	}
	if len(s.Hidden) != 1 {
		t.Errorf("重复隐藏产生了 %d 条", len(s.Hidden))
	}

	// 再藏一部，然后恢复第一部
	if _, err := l.SetHidden("steam:2", true); err != nil {
		t.Fatal(err)
	}
	s, err = l.SetHidden("steam:1", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Hidden) != 1 || s.Hidden[0] != "steam:2" {
		t.Errorf("恢复后隐藏列表 = %v，期望只剩 steam:2", s.Hidden)
	}

	if _, err := l.SetHidden("", true); err == nil {
		t.Error("空 key 应该报错")
	}
}

// TestSetSortNormalizes 确认排序字段/方向的非法值都会被收敛。
func TestSetSortNormalizes(t *testing.T) {
	l := &Launcher{}
	l.setConfig(config.NewStore(t.TempDir()))

	s, err := l.SetSort("playtime", "asc")
	if err != nil {
		t.Fatalf("SetSort: %v", err)
	}
	if s.SortBy != config.SortByPlaytime || s.SortDir != config.SortAsc {
		t.Errorf("排序 = %q/%q", s.SortBy, s.SortDir)
	}

	s, err = l.SetSort("乱七八糟", "sideways")
	if err != nil {
		t.Fatal(err)
	}
	if s.SortBy != config.SortByRecent || s.SortDir != config.SortDesc {
		t.Errorf("坏值应该回落默认，实际 %q/%q", s.SortBy, s.SortDir)
	}
}

// TestOpenFolderUsesInstallDir 覆盖「打开安装目录」：交给系统文件管理器，
// 目录不存在时报错（不然会静默打开一个空窗口）。
func TestOpenFolderUsesInstallDir(t *testing.T) {
	t.Setenv("LAUNCHER_NO_STORE_ASSETS", "1")
	t.Setenv("LAUNCHER_STEAM_DIR", t.TempDir())

	dir := t.TempDir()
	l := &Launcher{}
	l.games = []library.Game{{Key: "steam:1", Title: "ELDEN RING", Provider: "steam", InstallDir: dir}}

	var got string
	restore := swapOpener(func(_ context.Context, target string) error {
		got = target
		return nil
	})
	defer restore()

	if err := l.OpenFolder(context.Background(), "steam:1"); err != nil {
		t.Fatalf("OpenFolder: %v", err)
	}
	if got != dir {
		t.Errorf("打开的目标 = %q，期望安装目录 %q", got, dir)
	}

	// 目录被删掉之后要报错
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if err := l.OpenFolder(context.Background(), "steam:1"); err == nil {
		t.Error("目录不存在应该报错")
	}

	// 没有安装目录的游戏也要报错
	l.games = append(l.games, library.Game{Key: "wegame:x", Title: "某游戏", Provider: "wegame"})
	if err := l.OpenFolder(context.Background(), "wegame:x"); err == nil {
		t.Error("没有安装目录应该报错")
	}
	if err := l.OpenFolder(context.Background(), "steam:404"); err == nil {
		t.Error("不存在的 key 应该报错")
	}
}
