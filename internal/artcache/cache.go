// Package artcache 把 Steam CDN 上的封面 / hero 下载到本地缓存。
//
// 两个好处：
//  1. 断网也能看到封面（第一次联网打开过的游戏都会被留下）
//  2. 本地有了文件，就能从封面上取主色，背景渐变才跟着游戏变
//
// 目录结构：<cache>/artwork/<appid>/<文件名>
package artcache

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxBytes 是单张图的上限：Steam 的 hero 一般 200~500KB，
// 这里留足余量，同时防止被异常响应写爆磁盘。
const maxBytes = 8 << 20

// Cache 是一次下载任务的目标目录。
type Cache struct {
	dir    string
	client *http.Client
}

// New 建一个缓存，目录不存在时会按需创建。
func New(dir string) *Cache {
	return &Cache{
		dir: dir,
		client: &http.Client{
			Timeout: 30 * time.Second,
			// 跟着 CDN 的重定向走，但别无限跳
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("artcache: 重定向太多")
				}
				return nil
			},
		},
	}
}

// Lookup 返回已经缓存好的本地路径。
func (c *Cache) Lookup(appID, name string) (string, bool) {
	name = fileName(name)
	if c == nil {
		return "", false
	}
	path, err := c.path(appID, name)
	if err != nil {
		return "", false
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() == 0 {
		return "", false
	}
	return path, true
}

// Fetch 下载到缓存并返回本地路径；已经缓存过就直接返回，不发请求。
func (c *Cache) Fetch(ctx context.Context, appID, name, url string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("artcache: 没有配置缓存目录")
	}
	name = fileName(name)
	if local, ok := c.Lookup(appID, name); ok {
		return local, nil
	}

	path, err := c.path(appID, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "GameLauncher/0.1 (+https://github.com/)")

	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("artcache: GET %s 返回 %d", url, res.StatusCode)
	}

	// 先写 .part 再改名：中途失败不会留下半张图当缓存。
	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}

	written, copyErr := io.Copy(f, io.LimitReader(res.Body, maxBytes))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(tmp)
		if copyErr != nil {
			return "", copyErr
		}
		return "", closeErr
	}
	if written == 0 {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("artcache: %s 是空的", url)
	}

	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// path 拼出本地路径，同时挡住 appid / 文件名里的路径穿越。
func (c *Cache) path(appID, name string) (string, error) {
	if !isDigits(appID) {
		return "", fmt.Errorf("artcache: 非法 appid %q", appID)
	}
	base := fileName(name)
	if base == "" || base == "." || base == string(filepath.Separator) || strings.Contains(base, "..") {
		return "", fmt.Errorf("artcache: 非法文件名 %q", name)
	}
	return filepath.Join(c.dir, appID, base), nil
}

// fileName 从文件名或 URL 里取出安全的最后一段。
//
// Steam 的新图带时间戳参数（library_capsule_2x.jpg?t=1793242975），
// 直接当文件名会把问号也写进去，之后 art:// 地址就坏了 —— 所以统一在这里剥掉。
func fileName(name string) string {
	if name == "" {
		return ""
	}
	if u, err := url.Parse(name); err == nil && u.Path != "" {
		name = u.Path
	}
	if i := strings.IndexAny(name, "?#"); i >= 0 {
		name = name[:i]
	}
	return filepath.Base(name)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
