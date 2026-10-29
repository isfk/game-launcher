package config

import (
	"encoding/json"
	"os"
	"path/filepath"
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
