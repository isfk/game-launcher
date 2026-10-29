// Package storeart 为「拿不到库封面」的游戏补一份封面和 hero。
//
// 为什么需要它：Steam 的库封面（600×900）在 2024 年之后换了存放位置 ——
// 开发者上传的图挂在带内容哈希的目录下：
//
//	https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/<appid>/<哈希>/library_capsule_2x.jpg
//
// 而旧地址 https://cdn.cloudflare.steamstatic.com/steam/apps/<appid>/library_600x900.jpg
// 只有老游戏才有。所以刚发售的游戏封面会 404，本地 appcache 里也还没有。
//
// 哈希猜不出来，只能问一次 Steam 客户端自己在用的公开接口：
//
//	IStoreBrowseService/GetItems —— 不需要登录、不需要 API key。
//
// 结果会缓存到 <cache>/<appid>.json，TTL 内不再联网，所以一部游戏通常只查一次。
// 设 LAUNCHER_NO_STORE_ASSETS=1 可以完全关掉联网。
package storeart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTTL 是查询结果的保质期：游戏补图之后，过一阵子会重新查一次。
const DefaultTTL = 7 * 24 * time.Hour

// maxBody 防止异常响应写爆内存。
const maxBody = 4 << 20

// assetBase 是新版图片的存放位置（哈希目录在这里面）。
const assetBase = "https://shared.akamai.steamstatic.com/store_item_assets/"

// Assets 是一部游戏可用的封面与 hero。
type Assets struct {
	AppID     string    `json:"appId"`
	Name      string    `json:"name,omitempty"`
	Capsule   string    `json:"capsule,omitempty"` // 竖版卡面（600×900）
	Hero      string    `json:"hero,omitempty"`    // 大图（1920×620）
	FetchedAt time.Time `json:"fetchedAt"`
}

// Client 按 appid 查 Store 资源，并把结果缓存在本地。
type Client struct {
	dir  string
	ttl  time.Duration
	http *http.Client
	base string // 便于测试替换
}

// New 建一个客户端。dir 为空表示不缓存（每次都查）。
func New(dir string) *Client {
	return &Client{
		dir:  dir,
		ttl:  DefaultTTL,
		http: &http.Client{Timeout: 12 * time.Second},
		base: "https://api.steampowered.com/IStoreBrowseService/GetItems/v1/",
	}
}

// Lookup 返回一部游戏的封面和 hero。第二个返回值表示有没有可用结果：
// 网络失败且没有缓存时返回 false，调用方退回渐变占位就行。
func (c *Client) Lookup(ctx context.Context, appid string) (Assets, bool) {
	if !isDigits(appid) {
		return Assets{}, false
	}

	cached, hasCache := c.readCache(appid)
	if hasCache && time.Since(cached.FetchedAt) < c.ttl {
		return cached, cached.usable()
	}

	fresh, err := c.fetch(ctx, appid)
	if err != nil {
		// 联网失败就用过期的旧结果，总比什么都没有强。
		return cached, hasCache && cached.usable()
	}
	c.writeCache(appid, fresh)
	return fresh, fresh.usable()
}

// usable 表示至少有一张能用的图。
func (a Assets) usable() bool {
	return a.Capsule != "" || a.Hero != ""
}

// fetch 查一次 Store 接口。
func (c *Client) fetch(ctx context.Context, appid string) (Assets, error) {
	input := fmt.Sprintf(
		`{"ids":[{"appid":%s}],"context":{"language":"schinese","country_code":"CN"},`+
			`"data_request":{"include_assets":true}}`, appid)

	endpoint := c.base + "?input_json=" + url.QueryEscape(input)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Assets{}, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GameLauncher/0.1)")

	resp, err := c.http.Do(req)
	if err != nil {
		return Assets{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Assets{}, fmt.Errorf("storeart: 接口返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return Assets{}, err
	}
	assets, err := parse(appid, body)
	if err != nil {
		return Assets{}, err
	}
	assets.FetchedAt = time.Now()
	return assets, nil
}

// browseResponse 是 GetItems 响应的形状（只留要用的字段）：
//
//	{"response": {"store_items": [{"appid": 4115450, "name": "影之刃零",
//	  "assets": {"asset_url_format": "steam/apps/4115450/${FILENAME}?t=...",
//	             "library_capsule": "<哈希>/library_capsule_schinese.jpg", ...}}]}}
type browseResponse struct {
	Response struct {
		Items []struct {
			AppID  int    `json:"appid"`
			Name   string `json:"name"`
			Assets struct {
				Format          string `json:"asset_url_format"`
				LibraryCapsule  string `json:"library_capsule"`
				LibraryCapsule2 string `json:"library_capsule_2x"`
				LibraryHero     string `json:"library_hero"`
				LibraryHero2    string `json:"library_hero_2x"`
				HeroCapsule     string `json:"hero_capsule"`
				HeroCapsule2    string `json:"hero_capsule_2x"`
				MainCapsule     string `json:"main_capsule"`
				MainCapsule2    string `json:"main_capsule_2x"`
				Header          string `json:"header"`
				Header2         string `json:"header_2x"`
				PageBackground  string `json:"page_background"`
			} `json:"assets"`
		} `json:"store_items"`
	} `json:"response"`
}

// parse 从响应里挑封面和 hero。
//
// 挑图顺序按「库里的正式资源优先」：library_* 是开发者专门为库界面准备的，
// 构图适合竖版卡面；实在没有就退回商店胶囊 / 页面背景。
func parse(appid string, body []byte) (Assets, error) {
	var payload browseResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Assets{}, fmt.Errorf("storeart: 解析失败: %w", err)
	}
	if len(payload.Response.Items) == 0 {
		return Assets{}, errors.New("storeart: 响应里没有这个应用")
	}

	item := payload.Response.Items[0]
	a := item.Assets

	cover := firstSet(a.LibraryCapsule2, a.LibraryCapsule, a.MainCapsule2, a.MainCapsule,
		a.Header2, a.Header, a.HeroCapsule2, a.HeroCapsule, a.PageBackground)
	hero := firstSet(a.LibraryHero2, a.LibraryHero, a.PageBackground, a.HeroCapsule2,
		a.HeroCapsule, a.MainCapsule2, a.MainCapsule)

	return Assets{
		AppID:   appid,
		Name:    item.Name,
		Capsule: assetURL(a.Format, cover),
		Hero:    assetURL(a.Format, hero),
	}, nil
}

// assetURL 把 asset_url_format 里占位符换成实际文件名，拼成完整地址。
//
// 两种格式都见过：
//
//	"steam/apps/4115450/${FILENAME}?t=1"                  → 新版，放在 assetBase 下
//	"store_item_assets/steam/apps/4115450/${FILENAME}?t=1" → 已经带前缀
func assetURL(format, filename string) string {
	if filename == "" {
		return ""
	}
	if format == "" {
		// 没给格式就按新版约定猜一个
		format = "steam/apps/${APPID}/${FILENAME}"
	}

	url := strings.Replace(format, "${FILENAME}", filename, 1)
	if strings.Contains(url, "${APPID}") {
		url = strings.Replace(url, "${APPID}", "", 1)
	}

	switch {
	case strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "https://"):
		return url
	case strings.HasPrefix(url, "store_item_assets/"):
		return "https://shared.akamai.steamstatic.com/" + url
	default:
		return assetBase + strings.TrimPrefix(url, "/")
	}
}

func firstSet(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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

// cachePath 拼缓存文件路径。appid 要当文件名用，所以只保留数字，
// 免得任何奇怪的值（例如手动添加的游戏）把路径带出缓存目录。
func (c *Client) cachePath(appid string) string {
	safe := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, appid)
	if safe == "" {
		safe = "unknown"
	}
	path := filepath.Join(c.dir, safe+".json")
	// 双保险：即使上面的过滤被绕过，也不允许跑到缓存目录外面去
	if rel, err := filepath.Rel(c.dir, path); err != nil || strings.HasPrefix(rel, "..") {
		return filepath.Join(c.dir, "unknown.json")
	}
	return path
}

// readCache 读磁盘上的查询结果。
func (c *Client) readCache(appid string) (Assets, bool) {
	if c.dir == "" {
		return Assets{}, false
	}
	raw, err := os.ReadFile(c.cachePath(appid))
	if err != nil {
		return Assets{}, false
	}
	var assets Assets
	if err := json.Unmarshal(raw, &assets); err != nil {
		return Assets{}, false
	}
	return assets, true
}

// writeCache 把查询结果写盘，失败不影响主流程。
func (c *Client) writeCache(appid string, assets Assets) {
	if c.dir == "" {
		return
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return
	}
	raw, err := json.Marshal(assets)
	if err != nil {
		return
	}
	path := c.cachePath(appid)
	// 先写临时文件再改名，避免半截 JSON 被下次读到
	tmp := path + ".part"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return
	}
	// 改名失败只影响缓存（下次再查一遍），不值得打断主流程
	_ = os.Rename(tmp, path) //nolint:errcheck
}
