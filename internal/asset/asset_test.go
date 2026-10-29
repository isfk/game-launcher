package asset

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestURLIsStableAndOpaque(t *testing.T) {
	store := NewStore()
	path := filepath.Join(t.TempDir(), "library_600x900.jpg")

	first := store.URL(path)
	second := store.URL(path)
	if first != second {
		t.Fatalf("同一路径应该得到同一 URL: %q vs %q", first, second)
	}
	if !strings.HasSuffix(first, ".jpg") {
		t.Errorf("URL 应保留后缀让浏览器认类型: %q", first)
	}
	if strings.Contains(first, path) {
		t.Errorf("URL 不该暴露真实路径: %q", first)
	}
	if store.URL("") != "" {
		t.Error("空路径应该得到空 URL")
	}
}

func TestBaseURLMatchesPlatform(t *testing.T) {
	got := baseURL()
	if runtime.GOOS == "windows" {
		if !strings.HasPrefix(got, "http://art.localhost/") {
			t.Fatalf("WebView2 要求 http://art.localhost/，实际 %q", got)
		}
		return
	}
	if got != "art://localhost/" {
		t.Fatalf("baseURL = %q", got)
	}
}

func TestHandlerServesRegisteredFile(t *testing.T) {
	store := NewStore()

	// 造一张最小的 PNG（1×1，透明）
	png := []byte{
		0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R',
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0a, 'I', 'D', 'A', 'T',
		0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00, 0x05,
		0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00,
		0x00, 0x00, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
	}
	path := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}

	url := store.URL(path)
	id := url[strings.LastIndex(url, "/")+1:]

	srv := httptest.NewServer(store.Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/" + id)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d，想要 200", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
	if res.Header.Get("Cache-Control") == "" {
		t.Error("应该有 Cache-Control")
	}
}

func TestHandlerRejectsUnknownID(t *testing.T) {
	store := NewStore()
	srv := httptest.NewServer(store.Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/deadbeef.png")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("没登记的 id 应该 404，实际 %d", res.StatusCode)
	}
}

func TestHandlerRejectsDeletedFile(t *testing.T) {
	store := NewStore()
	path := filepath.Join(t.TempDir(), "gone.jpg")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := store.URL(path)
	id := url[strings.LastIndex(url, "/")+1:]
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(store.Handler())
	defer srv.Close()

	res, err := http.Get(srv.URL + "/" + id)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("文件没了应该 404，实际 %d", res.StatusCode)
	}
}
