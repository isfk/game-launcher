package artcache

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFetchWritesFileAndLookupFindsIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("fake-jpeg-bytes"))
	}))
	defer srv.Close()

	cache := New(t.TempDir())
	path, err := cache.Fetch(context.Background(), "1245620", "library_600x900.jpg", srv.URL+"/cover.jpg")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读缓存文件: %v", err)
	}
	if string(body) != "fake-jpeg-bytes" {
		t.Errorf("内容 = %q", body)
	}
	if !strings.HasSuffix(path, filepath.Join("1245620", "library_600x900.jpg")) {
		t.Errorf("缓存路径 = %q", path)
	}

	local, ok := cache.Lookup("1245620", "library_600x900.jpg")
	if !ok || local != path {
		t.Errorf("Lookup = %q, %v", local, ok)
	}
}

func TestFetchSkipsDownloadWhenCached(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte("x"))
	}))
	defer srv.Close()

	cache := New(t.TempDir())
	url := srv.URL + "/a.jpg"

	if _, err := cache.Fetch(context.Background(), "42", "a.jpg", url); err != nil {
		t.Fatalf("第一次 Fetch: %v", err)
	}
	if _, err := cache.Fetch(context.Background(), "42", "a.jpg", url); err != nil {
		t.Fatalf("第二次 Fetch: %v", err)
	}

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("应该只请求一次，实际 %d 次", got)
	}
}

func TestFetchReportsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	cache := New(t.TempDir())
	if _, err := cache.Fetch(context.Background(), "42", "a.jpg", srv.URL+"/a.jpg"); err == nil {
		t.Fatal("404 应该返回错误")
	}

	// 失败不能留下任何文件
	if _, ok := cache.Lookup("42", "a.jpg"); ok {
		t.Error("失败后不该有缓存")
	}
	entries, err := os.ReadDir(filepath.Join(cache.dir, "42"))
	if err == nil && len(entries) > 0 {
		t.Errorf("失败后目录应该是空的，实际 %d 个文件", len(entries))
	}
}

func TestPathRejectsTraversal(t *testing.T) {
	cache := New(t.TempDir())

	if _, err := cache.path("../../etc", "x.jpg"); err == nil {
		t.Error("带 .. 的 appid 应该被拒")
	}
	if _, err := cache.path("42", "../../evil.jpg"); err != nil {
		t.Fatalf("文件名会被 filepath.Base 收敛，不该报错: %v", err)
	}
	path, err := cache.path("42", "../../evil.jpg")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(path, "..") {
		t.Errorf("收敛后不该含 ..: %q", path)
	}
	if _, err := cache.path("42", ".."); err == nil {
		t.Error("文件名是 .. 时应该被拒")
	}
}

func TestNilCacheIsSafe(t *testing.T) {
	var cache *Cache
	if _, ok := cache.Lookup("42", "a.jpg"); ok {
		t.Error("nil cache 的 Lookup 应该是 false")
	}
	if _, err := cache.Fetch(context.Background(), "42", "a.jpg", "http://x"); err == nil {
		t.Error("nil cache 的 Fetch 应该报错而不是 panic")
	}
}

func TestFileNameStripsQueryAndFragment(t *testing.T) {
	cases := map[string]string{
		"library_capsule_2x.jpg?t=1793242975":                            "library_capsule_2x.jpg",
		"https://shared.akamai.steamstatic.com/x/hash/hero.jpg?t=1#frag": "hero.jpg",
		"plain.jpg":            "plain.jpg",
		"../evil/../../passwd": "passwd",
		"":                     "",
	}
	for in, want := range cases {
		if got := fileName(in); got != want {
			t.Errorf("fileName(%q) = %q，想要 %q", in, got, want)
		}
	}
}

func TestFetchKeepsQueryOutOfTheFilename(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("jpeg-bytes"))
	}))
	defer server.Close()

	cache := New(t.TempDir())
	local, err := cache.Fetch(context.Background(), "4115450", "library_capsule_2x.jpg?t=1793242975", server.URL+"/x.jpg?t=1793242975")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if filepath.Base(local) != "library_capsule_2x.jpg" {
		t.Errorf("落盘文件名 = %q", filepath.Base(local))
	}
	// 下载后再查缓存应该命中
	if again, ok := cache.Lookup("4115450", "library_capsule_2x.jpg"); !ok || again != local {
		t.Error("带不带 query 应该命中同一份缓存")
	}
}
