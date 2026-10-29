package storeart

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 真实响应裁剪到要用的字段（IStoreBrowseService/GetItems，appid 4115450 影之刃零）。
const fixture = `{
  "response": {
    "store_items": [
      {
        "appid": 4115450,
        "name": "影之刃零",
        "assets": {
          "asset_url_format": "steam/apps/4115450/${FILENAME}?t=1793242975",
          "main_capsule": "b1fe30d2/capsule_616x353_schinese.jpg",
          "main_capsule_2x": "b1fe30d2/capsule_616x353_schinese_2x.jpg",
          "small_capsule": "8c12e4e9/capsule_231x87_schinese.jpg",
          "header": "40b20446/header_schinese.jpg",
          "header_2x": "40b20446/header_schinese_2x.jpg",
          "page_background": "8477e332/page_bg_v6.jpg",
          "hero_capsule": "896d7339/hero_capsule_schinese.jpg",
          "hero_capsule_2x": "896d7339/hero_capsule_schinese_2x.jpg",
          "library_capsule": "b42db019/library_capsule_schinese.jpg",
          "library_capsule_2x": "b42db019/library_capsule_schinese_2x.jpg",
          "library_hero": "1cd52ca8/library_hero_schinese.jpg",
          "library_hero_2x": "1cd52ca8/library_hero_schinese_2x.jpg"
        }
      }
    ]
  }
}`

// 没有库封面的老游戏：只有商店胶囊。
const fixtureStoreOnly = `{
  "response": {
    "store_items": [
      {
        "appid": 1234,
        "name": "Old Game",
        "assets": {
          "asset_url_format": "steam/apps/1234/${FILENAME}?t=1",
          "main_capsule": "aaa/capsule_616x353.jpg",
          "header": "bbb/header.jpg",
          "page_background": "ccc/page_bg.jpg"
        }
      }
    ]
  }
}`

func TestParsePrefersLibraryAssets(t *testing.T) {
	assets, err := parse("4115450", []byte(fixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if assets.Name != "影之刃零" {
		t.Errorf("name = %q", assets.Name)
	}
	// 封面要用 2x 库封面（600×900），不是商店的小胶囊
	if !strings.Contains(assets.Capsule, "library_capsule_schinese_2x.jpg") {
		t.Errorf("封面挑错了: %q", assets.Capsule)
	}
	if !strings.HasPrefix(assets.Capsule, assetBase) {
		t.Errorf("封面地址前缀不对: %q", assets.Capsule)
	}
	if !strings.Contains(assets.Hero, "library_hero_schinese_2x.jpg") {
		t.Errorf("hero 挑错了: %q", assets.Hero)
	}
	// 保留了时间戳参数
	if !strings.Contains(assets.Capsule, "?t=1793242975") {
		t.Errorf("丢了时间戳: %q", assets.Capsule)
	}
}

func TestParseFallsBackToStoreCapsule(t *testing.T) {
	assets, err := parse("1234", []byte(fixtureStoreOnly))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(assets.Capsule, "capsule_616x353.jpg") {
		t.Errorf("没有库封面时该退到商店胶囊，实际 %q", assets.Capsule)
	}
	if !strings.Contains(assets.Hero, "page_bg.jpg") {
		t.Errorf("没有库 hero 时该退到页面背景，实际 %q", assets.Hero)
	}
}

func TestParseRejectsEmptyAndBroken(t *testing.T) {
	if _, err := parse("4115450", []byte(`{"response": {"store_items": []}}`)); err == nil {
		t.Error("空列表应该报错")
	}
	if _, err := parse("4115450", []byte(`not json`)); err == nil {
		t.Error("坏 JSON 应该报错")
	}
}

func TestAssetURLFormats(t *testing.T) {
	cases := []struct {
		format, file, want string
	}{
		{
			format: "steam/apps/4115450/${FILENAME}?t=1",
			file:   "hash/library_capsule.jpg",
			want:   assetBase + "steam/apps/4115450/hash/library_capsule.jpg?t=1",
		},
		{
			// 有些响应已经带了前缀
			format: "store_item_assets/steam/apps/${APPID}/${FILENAME}",
			file:   "hash/h.jpg",
			want:   "https://shared.akamai.steamstatic.com/store_item_assets/steam/apps//hash/h.jpg",
		},
		{
			format: "",
			file:   "hash/h.jpg",
			want:   assetBase + "steam/apps//hash/h.jpg",
		},
	}
	for _, c := range cases {
		// ${APPID} 由调用方（parse）替换，这里模拟 format 里已展开的情况
		got := assetURL(strings.ReplaceAll(c.format, "${APPID}", ""), c.file)
		if got != c.want {
			t.Errorf("format=%q → %q，想要 %q", c.format, got, c.want)
		}
	}

	// 已经是完整地址时原样返回
	if got := assetURL("https://x/y.jpg", ""); got != "" {
		t.Errorf("空文件名应该返回空串，实际 %q", got)
	}
}

func TestLookupFetchesThenCaches(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		input := r.URL.Query().Get("input_json")
		if !strings.Contains(input, `"appid":4115450`) {
			t.Errorf("input_json 不对: %q", input)
		}
		if r.Header.Get("User-Agent") == "" {
			t.Error("应该带 User-Agent")
		}
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()

	dir := t.TempDir()
	client := New(dir)
	client.base = server.URL

	first, ok := client.Lookup(context.Background(), "4115450")
	if !ok || first.Capsule == "" {
		t.Fatalf("第一次应该拿到图: %+v", first)
	}
	if hits != 1 {
		t.Fatalf("应该只请求一次，实际 %d", hits)
	}

	second, ok := client.Lookup(context.Background(), "4115450")
	if !ok || second.Capsule != first.Capsule {
		t.Fatalf("缓存结果不一致: %+v", second)
	}
	if hits != 1 {
		t.Errorf("缓存有效期内不该再请求，实际 %d 次", hits)
	}

	if _, err := os.Stat(filepath.Join(dir, "4115450.json")); err != nil {
		t.Errorf("缓存文件没写出来: %v", err)
	}
	other := New(dir)
	other.base = "http://127.0.0.1:1/should-not-be-called"
	if got, ok := other.Lookup(context.Background(), "4115450"); !ok || got.Capsule == "" {
		t.Error("新实例应该直接读到磁盘缓存")
	}
}

func TestLookupFallsBackToStaleCacheWhenOffline(t *testing.T) {
	dir := t.TempDir()
	stale := `{"appId":"4115450","capsule":"https://example.com/c.jpg","fetchedAt":"2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(dir, "4115450.json"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	client := New(dir)
	client.base = "http://127.0.0.1:1/nope"
	assets, ok := client.Lookup(context.Background(), "4115450")
	if !ok {
		t.Fatal("断网时应该退回旧缓存")
	}
	if assets.Capsule != "https://example.com/c.jpg" {
		t.Errorf("退回了错误的内容: %+v", assets)
	}
}

func TestLookupWithoutCacheAndNetworkReturnsFalse(t *testing.T) {
	client := New("")
	client.base = "http://127.0.0.1:1/nope"
	if _, ok := client.Lookup(context.Background(), "4115450"); ok {
		t.Error("既没网也没缓存，应该返回 false")
	}
}

func TestLookupSkipsNonNumericAppID(t *testing.T) {
	client := New(t.TempDir())
	client.base = "http://127.0.0.1:1/nope"
	for _, id := range []string{"", "manual-1", "../etc"} {
		if _, ok := client.Lookup(context.Background(), id); ok {
			t.Errorf("%q 不该去查", id)
		}
	}
}

func TestCacheExpiresAfterTTL(t *testing.T) {
	dir := t.TempDir()
	old := Assets{
		AppID:     "4115450",
		Capsule:   "https://example.com/old.jpg",
		FetchedAt: time.Now().Add(-2 * DefaultTTL),
	}
	client := New(dir)
	client.writeCache(old.AppID, old)

	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte(fixture))
	}))
	defer server.Close()
	client.base = server.URL

	assets, ok := client.Lookup(context.Background(), "4115450")
	if !ok {
		t.Fatal("应该拿到新结果")
	}
	if hits != 1 {
		t.Errorf("过期后应该重新查一次，实际 %d", hits)
	}
	if assets.Capsule == "https://example.com/old.jpg" {
		t.Error("过期缓存不该被当成新结果")
	}
}

func TestCachePathIsSanitized(t *testing.T) {
	dir := t.TempDir()
	client := New(dir)
	got := client.cachePath("../../etc/passwd")
	if strings.Contains(got, "..") || !strings.HasPrefix(got, dir) {
		t.Errorf("路径穿越没防住: %q", got)
	}
}
