package config

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMissingGivesDefault 覆盖「第一次启动」：没有文件也要拿到可用设置。
func TestLoadMissingGivesDefault(t *testing.T) {
	store := NewStore(t.TempDir())

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.SoundEnabled {
		t.Error("默认应该开着音效")
	}
	if len(got.Favorites) != 0 {
		t.Errorf("默认不该有收藏: %v", got.Favorites)
	}
}

// TestSaveLoadRoundTrip 验证写进去的东西能原样读回来。
func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	want := Settings{SoundEnabled: false, Favorites: []string{"steam:1245620", "manual:abc"}}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SoundEnabled != want.SoundEnabled {
		t.Errorf("SoundEnabled = %v，想要 %v", got.SoundEnabled, want.SoundEnabled)
	}
	if len(got.Favorites) != len(want.Favorites) {
		t.Fatalf("Favorites = %v，想要 %v", got.Favorites, want.Favorites)
	}
	for i := range want.Favorites {
		if got.Favorites[i] != want.Favorites[i] {
			t.Errorf("Favorites[%d] = %q，想要 %q", i, got.Favorites[i], want.Favorites[i])
		}
	}

	// 目录里只该留下正式文件，临时文件不能残留
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		t.Errorf("目录内容 = %v，应该只有 %s", entries, fileName)
	}
}

// TestLoadCorruptBacksUp 坏文件不能让应用起不来，也不能把坏数据留在原地。
func TestLoadCorruptBacksUp(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	if err := os.WriteFile(store.Path(), []byte("{ 半个 json"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err == nil {
		t.Error("坏文件应该报错，好让调用方记日志")
	}
	if !got.SoundEnabled {
		t.Error("坏文件应该回默认值")
	}
	if _, statErr := os.Stat(store.Path() + ".broken"); statErr != nil {
		t.Errorf("坏文件应该被改名留档: %v", statErr)
	}
}

// TestSaveCreatesDir 目录不存在时自己建出来（用户可能清过配置目录）。
func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "GameLauncher")
	store := NewStore(dir)

	if err := store.Save(Default()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		t.Fatalf("设置文件没写出来: %v", err)
	}

	// 落盘的内容要是人能看懂的 JSON（方便出问题时手改）
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("写出来的不是合法 JSON: %v", err)
	}
	if probe["soundEnabled"] != true {
		t.Errorf("soundEnabled = %v", probe["soundEnabled"])
	}
}

// TestLoadUnknownFieldsIgnored 老版本读到新版本的文件不该崩。
func TestLoadUnknownFieldsIgnored(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := os.MkdirAll(store.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"soundEnabled": false, "futureField": {"a": 1}}`
	if err := os.WriteFile(store.Path(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SoundEnabled {
		t.Error("文件里是 false，读出来不该是 true")
	}
}

// TestIconPosDefaultsAndNormalizes 覆盖图标位置：老配置里没有这个字段时
// 必须回落到「左下（与游戏名同一行）」，并且手改坏的值也能被收敛。
func TestIconPosDefaultsAndNormalizes(t *testing.T) {
	if got := Default().IconPos; got != IconPosBottomLeft {
		t.Errorf("默认图标位置 = %q，期望 %q", got, IconPosBottomLeft)
	}

	cases := map[string]string{
		"tl": IconPosTopLeft,
		"tr": IconPosTopRight,
		"br": IconPosBottomRight,
		"bl": IconPosBottomLeft,
		"":   IconPosBottomLeft,
		"??": IconPosBottomLeft, // 手改坏的值
		"TL": IconPosBottomLeft, // 大小写不认，收敛到默认
	}
	for in, want := range cases {
		if got := NormalizeIconPos(in); got != want {
			t.Errorf("NormalizeIconPos(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// TestIconPosPersists 确认图标位置会落盘并在下次读出来。
func TestIconPosPersists(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	s := Default()
	s.IconPos = IconPosTopRight
	if err := store.Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	again, err := NewStore(dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if again.IconPos != IconPosTopRight {
		t.Errorf("读回来的图标位置 = %q，期望 %q", again.IconPos, IconPosTopRight)
	}
}

// TestSortDefaultsAndNormalizes 覆盖排序字段/方向的默认值与非法值收敛。
func TestSortDefaultsAndNormalizes(t *testing.T) {
	d := Default()
	if d.SortBy != SortByRecent || d.SortDir != SortDesc {
		t.Errorf("默认排序 = %q/%q，期望 recent/desc", d.SortBy, d.SortDir)
	}

	for _, by := range SortOptions {
		if got := NormalizeSortBy(by); got != by {
			t.Errorf("NormalizeSortBy(%q) = %q", by, got)
		}
	}
	for _, bad := range []string{"", "name_", "NAME", "最近"} {
		if got := NormalizeSortBy(bad); got != SortByRecent {
			t.Errorf("坏值 %q 应该回落到 recent，实际 %q", bad, got)
		}
	}

	if got := NormalizeSortDir("asc"); got != SortAsc {
		t.Errorf("NormalizeSortDir(asc) = %q", got)
	}
	for _, bad := range []string{"", "up", "ASC"} {
		if got := NormalizeSortDir(bad); got != SortDesc {
			t.Errorf("坏值 %q 应该回落到 desc，实际 %q", bad, got)
		}
	}
}

// TestNormalizeName 覆盖昵称收敛：空白、换行、超长都得变成一行能显示的字。
func TestNormalizeName(t *testing.T) {
	cases := map[string]string{
		"  阿伞  ":                "阿伞",
		"阿伞\n第二行":               "阿伞 第二行",
		"a\t\tb":                "a b",
		"":                      "",
		"   ":                   "",
		strings.Repeat("喵", 30): strings.Repeat("喵", MaxNameRunes),
	}
	for in, want := range cases {
		if got := NormalizeName(in); got != want {
			t.Errorf("NormalizeName(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// tinyPNG 是一个 1x1 的真 PNG：用它验证「按内容判类型」那一套。
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8DwHwAFAAH/q842iQAAAABJRU5ErkJggg==",
	)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestSaveAvatarCopiesAndDedupes 覆盖头像落盘：复制进配置目录、同一张图不存两份。
func TestSaveAvatarCopiesAndDedupes(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	src := filepath.Join(t.TempDir(), "me.png")
	if err := os.WriteFile(src, tinyPNG(t), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := store.SaveAvatar(src)
	if err != nil {
		t.Fatalf("SaveAvatar: %v", err)
	}
	if filepath.Dir(first) != filepath.Join(dir, "avatars") {
		t.Errorf("头像应该存进配置目录，实际 %q", first)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("头像没写出来: %v", err)
	}

	second, err := store.SaveAvatar(src)
	if err != nil {
		t.Fatalf("SaveAvatar 第二次: %v", err)
	}
	if second != first {
		t.Errorf("同一张图应该落到同一个文件，%q != %q", second, first)
	}

	// 删掉原图：配置目录里的副本还在（这正是复制而不是记路径的意义）
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first); err != nil {
		t.Errorf("原图删掉后副本应该还在: %v", err)
	}
}

// TestSaveAvatarRejectsNonImage 改名成 .png 的文本文件不能混进来。
func TestSaveAvatarRejectsNonImage(t *testing.T) {
	src := filepath.Join(t.TempDir(), "fake.png")
	if err := os.WriteFile(src, []byte("这不是图片"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := NewStore(t.TempDir())
	if _, err := store.SaveAvatar(src); err == nil {
		t.Error("非图片应该报错")
	}

	empty := filepath.Join(t.TempDir(), "empty.png")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveAvatar(empty); err == nil {
		t.Error("空文件应该报错")
	}
}

// TestLoadFillsLegacyDefaults 覆盖老配置文件：缺 iconPos/sortBy/sortDir/gridCols
// 的时候必须回落到默认值，而不是把零值（0 / 空串）交给界面。
func TestLoadFillsLegacyDefaults(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	// 手写一份「旧版本」的配置：只有音效和收藏
	legacy := `{"soundEnabled": false, "favorites": ["steam:1"]}`
	if err := os.WriteFile(store.Path(), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.IconPos != IconPosBottomLeft {
		t.Errorf("IconPos = %q，期望 %q", got.IconPos, IconPosBottomLeft)
	}
	if got.SortBy != SortByRecent || got.SortDir != SortDesc {
		t.Errorf("排序 = %q/%q，期望 recent/desc", got.SortBy, got.SortDir)
	}
	if got.SoundEnabled {
		t.Error("音效开关应该保留旧配置里的 false")
	}
}
