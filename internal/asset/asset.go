// Package asset 把本机的图片文件变成页面能加载的 URL。
//
// 为什么需要它：WebView 不能直接读 `C:\...\cover.jpg` 这种文件路径，
// 而 Steam 的封面都躺在本地 appcache 里。这里用一个自定义 scheme
// 把「路径」换成「URL」，页面就可以正常 <img src>：
//
//	macOS / Linux:  art://localhost/<id>
//	Windows:        http://art.localhost/<id>   ← WebView2 的要求
//
// id 是路径的散列，页面拿不到真实路径，也就不能顺着它读别的文件。
package asset

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Scheme 是自定义协议名。
const Scheme = "art"

// Store 记住 id → 文件路径的对应关系，并负责把它们服务出去。
type Store struct {
	mu    sync.RWMutex
	paths map[string]string
}

// NewStore 建一个空的 Store。
func NewStore() *Store {
	return &Store{paths: make(map[string]string)}
}

// URL 登记一个本地文件，返回页面能直接用的 URL。
// 同一个路径永远得到同一个 URL，重复登记不会让 map 变大。
func (s *Store) URL(path string) string {
	if path == "" {
		return ""
	}
	id := idOf(path)
	s.mu.Lock()
	s.paths[id] = path
	s.mu.Unlock()
	return baseURL() + id
}

// Lookup 把 id 还原成路径。
func (s *Store) Lookup(id string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, ok := s.paths[id]
	return path, ok
}

// Handler 是这个 scheme 的实现。只服务登记过的文件，别的 id 一律 404。
func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/")
		path, ok := s.Lookup(id)
		if !ok {
			http.NotFound(w, r)
			return
		}

		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = f.Close() }()

		st, err := f.Stat()
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}

		// 封面不会天天变；Steam 换图时文件名通常也会换。
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if ct := contentTypeOf(path); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		http.ServeContent(w, r, st.Name(), st.ModTime(), f)
	})
}

// baseURL 是各平台上页面访问自定义 scheme 的前缀。
func baseURL() string {
	if runtime.GOOS == "windows" {
		// WebView2 把自定义 scheme 暴露成 <scheme>.localhost。
		return "http://" + Scheme + ".localhost/"
	}
	return Scheme + "://localhost/"
}

// idOf 生成稳定的 id：路径散列 + 原始后缀（保留后缀是为了让
// ServeContent 和浏览器都能认出类型）。
//
// 用 sha256 而不是更短的散列：id 就是页面能访问的唯一凭据，
// 不能被猜出来去翻别的文件。
func idOf(path string) string {
	sum := sha256.Sum256([]byte(path))
	ext := strings.ToLower(filepath.Ext(path))
	if len(ext) > 8 {
		ext = ""
	}
	return hex.EncodeToString(sum[:])[:20] + ext
}

func contentTypeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	default:
		return ""
	}
}
