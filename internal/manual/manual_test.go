package manual

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"game-launcher/internal/config"
)

// TestIDForIsStableAndDistinct 覆盖 key 的生成：同一条路径永远同一个 key
// （重复添加不会变两条），不同路径不会撞车。
func TestIDForIsStableAndDistinct(t *testing.T) {
	a := IDFor("/games/one/Game.exe")
	b := IDFor("/games/one/Game.exe")
	c := IDFor("/games/two/Game.exe")

	if a != b {
		t.Errorf("同一路径的 key 不稳定: %q vs %q", a, b)
	}
	if a == c {
		t.Errorf("不同路径撞 key 了: %q", a)
	}
	if !strings.HasPrefix(a, "manual:") {
		t.Errorf("key 应该有 manual: 前缀，实际 %q", a)
	}
	if got := len(strings.TrimPrefix(a, "manual:")); got != 10 {
		t.Errorf("hash 长度 = %d，期望 10", got)
	}
}

// TestTitleForStripsExtension 覆盖显示名：去掉扩展名，游戏名里的标点保留原样。
func TestTitleForStripsExtension(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/Games/NieR_Automata/NieR_Automata.exe", "NieR_Automata"},
		{"/Applications/Hollow Knight.app", "Hollow Knight"},
		{"/opt/stardew/stardew.sh", "stardew"},
		{"/games/wrapper", "wrapper"},
	}
	for _, c := range cases {
		if got := TitleFor(c.in); got != c.want {
			t.Errorf("TitleFor(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}

	// .exe 的剥离与平台无关：darwin 上也可能有 Wine 游戏
	if runtime.GOOS != "darwin" {
		if got := TitleFor("/x/Thing.app"); got != "Thing.app" {
			t.Errorf("非 macOS 不该剥 .app，得到 %q", got)
		}
	}
}

// TestValidPathRejects 覆盖不该被加进库的东西：空、不存在、目录。
func TestValidPathRejects(t *testing.T) {
	if err := ValidPath(""); err == nil {
		t.Error("空路径应该报错")
	}
	if err := ValidPath(filepath.Join(t.TempDir(), "nope.exe")); err == nil {
		t.Error("不存在的文件应该报错")
	}
	if err := ValidPath(t.TempDir()); err == nil {
		t.Error("普通文件夹应该报错")
	}
}

// TestValidPathAcceptsExecutable 确认可执行文件能通过。
func TestValidPathAcceptsExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 上按扩展名判断，另有用例")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "game.sh")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidPath(exe); err != nil {
		t.Errorf("可执行脚本应该能过: %v", err)
	}

	// 没有执行位的不收：不然点启动就「什么也没发生」
	plain := filepath.Join(dir, "readme.sh")
	if err := os.WriteFile(plain, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ValidPath(plain); err == nil {
		t.Error("没有执行位的文件应该报错")
	}
}

// TestValidPathAcceptsAppBundle 确认 macOS 的 .app 目录算可执行。
func TestValidPathAcceptsAppBundle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("只有 macOS 有 .app 包")
	}
	bundle := filepath.Join(t.TempDir(), "Some Game.app")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidPath(bundle); err != nil {
		t.Errorf(".app 包应该能过: %v", err)
	}
}

// TestGamesMapsEntries 覆盖「落盘记录 → 界面 Game」的转换。
func TestGamesMapsEntries(t *testing.T) {
	if got := Games(nil); len(got) != 0 {
		t.Errorf("空记录应该得到空列表，实际 %d 条", len(got))
	}

	path := filepath.Join("games", "one", "Game.exe")
	got := Games([]config.ManualGame{{ID: "manual:x", Title: "我的游戏", Path: path}})
	if len(got) != 1 {
		t.Fatalf("应该转出 1 条，实际 %d", len(got))
	}
	g := got[0]
	if g.Provider != "manual" {
		t.Errorf("Provider = %q", g.Provider)
	}
	if g.Title != "我的游戏" {
		t.Errorf("Title = %q", g.Title)
	}
	if g.Key != IDFor(path) {
		t.Errorf("Key = %q，期望 %q", g.Key, IDFor(path))
	}
	if g.InstallDir != filepath.Dir(path) {
		t.Errorf("InstallDir = %q，期望 %q", g.InstallDir, filepath.Dir(path))
	}
	if !g.Installed {
		t.Error("手动添加的游戏应该算已安装")
	}
}

// TestGamesSkipsEmptyPath 确认坏记录不会让界面冒出一张点不开的卡。
func TestGamesSkipsEmptyPath(t *testing.T) {
	got := Games([]config.ManualGame{{ID: "manual:x", Title: "ghost"}})
	if len(got) != 0 {
		t.Errorf("路径为空的记录应该被跳过，实际 %d 条", len(got))
	}
}

// TestEntryFillsDefaults 确认不传名字时从路径猜，且路径转成绝对路径。
func TestEntryFillsDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Cool Game.sh")
	e := Entry(path, "")
	if e.Title != "Cool Game" {
		t.Errorf("Title = %q，期望从文件名推出 Cool Game", e.Title)
	}
	if !filepath.IsAbs(e.Path) {
		t.Errorf("Path 应该是绝对路径: %q", e.Path)
	}
	if e.ID != IDFor(e.Path) {
		t.Errorf("ID 和路径不一致: %q", e.ID)
	}
	if e.Added.IsZero() {
		t.Error("Added 应该被填上")
	}

	// 显式给名字就用给的
	if got := Entry(path, "自定义"); got.Title != "自定义" {
		t.Errorf("Title = %q，期望用的是传入的名字", got.Title)
	}
}
