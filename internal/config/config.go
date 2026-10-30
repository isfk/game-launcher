// Package config 把用户设置存成一个 JSON 文件。
//
// 为什么放后端而不是 localStorage：这些设置要跨窗口、跨重启一致，
// 而且以后「手动添加的游戏」「收藏排序」也要一起落盘 —— 只有一个落盘处，
// 就不会出现「界面记得、后端不知道」的撕扯。
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Settings 是持久化的用户设置。加字段前想清楚默认值：
// 老版本的 settings.json 里没有这个字段，读出来是 Go 零值。
type Settings struct {
	SoundEnabled bool `json:"soundEnabled"`
	// Favorites 存的是 Game.Key（"steam:1245620"），顺序无意义。
	Favorites []string `json:"favorites,omitempty"`
	// ManualGames 是手动添加的游戏。它们的 key 由路径算出（见 internal/manual），
	// 所以文件被挪走之后这条记录会「指向不存在的路径」，加载时跳过即可。
	ManualGames []ManualGame `json:"manualGames,omitempty"`
	// IconPos 是卡片上平台图标的位置：tl / tr / bl / br。
	// bl 是「和游戏名同一行」，也是默认值。老配置没有这个字段时用默认值补。
	IconPos string `json:"iconPos,omitempty"`
	// Hidden 是「只在启动器里藏起来」的游戏 key。客户端那边什么都不动，
	// 和 Uninstall 是两件事：藏起来的游戏还能从设置里恢复。
	Hidden []string `json:"hidden,omitempty"`
	// SortBy / SortDir 是首页的排序方式与方向。
	// 收藏永远置顶，这两个只决定「同类之内」的顺序。
	SortBy  string `json:"sortBy,omitempty"`
	SortDir string `json:"sortDir,omitempty"`
	// Order 是「自定义顺序」（玩家在全部游戏里两张卡一张拖出来的）。
	// 存的是 Game.Key 的完整顺序，空表示没调过。SortBy 为 manual 时生效。
	Order []string `json:"order,omitempty"`
	// Name 是右上角的昵称。空串不是「叫空名字」，而是「用系统账户名」——
	// 第一次启动的人不该先被安上一个假名字。
	Name string `json:"name,omitempty"`
	// Avatar 是头像图片在本机的绝对路径，空串表示用昵称首字兜底。
	// 真正存的是复制进配置目录的那一份（见 SaveAvatar），所以原图挪走也不影响。
	Avatar string `json:"avatar,omitempty"`
}

// MaxNameRunes 是昵称的字数上限。够长（中文 24 字能写一句话），
// 又不至于把右上角那一行撑爆。
const MaxNameRunes = 24

// NormalizeName 收敛昵称：去首尾空白、把换行/制表压成一个空格、超长截断。
//
// 为什么要在后端做：昵称会被写进 settings.json，也可能被人手改；
// 界面上一个回车就能塞进换行，那条记录会变得没法显示。收敛一次，
// 界面就不用到处防御。返回空串表示「没设置」，由调用方决定兜底成什么。
func NormalizeName(raw string) string {
	// 换行 / 制表先变空格，再按空白切分重新拼，顺带削掉连续空格
	fields := strings.Fields(raw)
	name := strings.Join(fields, " ")

	runes := []rune(name)
	if len(runes) <= MaxNameRunes {
		return name
	}
	return strings.TrimSpace(string(runes[:MaxNameRunes]))
}

// ManualGame 是一条手动添加的记录：只要路径和显示名，没有平台信息。
type ManualGame struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Path  string    `json:"path"`
	Added time.Time `json:"added,omitempty"`
}

// Default 是「第一次启动」的设置：音效开着，没有收藏。
// 图标位置的合法取值。bl = 和游戏名同一行（默认）。
const (
	IconPosTopLeft     = "tl"
	IconPosTopRight    = "tr"
	IconPosBottomLeft  = "bl"
	IconPosBottomRight = "br"
)

// NormalizeIconPos 把任意输入收敛到合法取值，不认识的一律回默认（左下）。
// 好处是手改坏的 settings.json 不会让界面卡在一个没有对应样式的值上。
func NormalizeIconPos(pos string) string {
	switch pos {
	case IconPosTopLeft, IconPosTopRight, IconPosBottomRight:
		return pos
	default:
		return IconPosBottomLeft
	}
}

// 排序字段。recent = 最近游玩（Steam 有数据，其他平台按名字兜底）。
// manual = 玩家自己拖出来的顺序（存在 Settings.Order）。
const (
	SortByRecent   = "recent"
	SortByName     = "name"
	SortByPlaytime = "playtime"
	SortBySize     = "size"
	SortByManual   = "manual"
)

// 排序方向。
const (
	SortAsc  = "asc"
	SortDesc = "desc"
)

// SortOptions 是设置面板里轮转的顺序，也是校验白名单。
// manual 也在白名单里：顺序调好之后界面会把「自定义」当成一个正常的排序项。
var SortOptions = []string{SortByRecent, SortByName, SortByPlaytime, SortBySize, SortByManual}

// NormalizeOrder 收敛自定义顺序：去空白、去重（保留第一次出现的位置）。
//
// 手改 settings.json、或者库缩水之后重复写同一个 key，都不该让排序错位。
func NormalizeOrder(keys []string) []string {
	if len(keys) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeSortBy 收敛非法的排序字段，默认「最近游玩」。
func NormalizeSortBy(by string) string {
	for _, opt := range SortOptions {
		if by == opt {
			return by
		}
	}
	return SortByRecent
}

// NormalizeSortDir 收敛非法的方向，默认「降序」（最近的/大的排前面）。
func NormalizeSortDir(dir string) string {
	if dir == SortAsc {
		return SortAsc
	}
	return SortDesc
}

func Default() Settings {
	return Settings{
		SoundEnabled: true,
		IconPos:      IconPosBottomLeft,
		SortBy:       SortByRecent,
		SortDir:      SortDesc,
	}
}

const fileName = "settings.json"

// Store 读写 <dir>/settings.json。并发安全，够单进程应用用。
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore 创建一个落在 dir 下的设置文件，目录不存在会在保存时创建。
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// Path 返回设置文件的完整路径（日志和「打开配置目录」之类的功能会用到）。
func (s *Store) Path() string {
	return filepath.Join(s.dir, fileName)
}

// maxAvatarBytes 是头像文件的上限。头像最后只画成一个小圆圈，
// 几 MB 已经是很奢侈的原图了；再大就不是「选错了」而是「想写爆磁盘」。
const maxAvatarBytes = 8 << 20

// avatarTypes 是允许的头像类型（按内容嗅探，不看扩展名）。
// 这四种是 WebView 在各平台上都稳能画的。
var avatarTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// SaveAvatar 把用户挑的图片复制到 <dir>/avatars/ 下，返回落盘的绝对路径。
//
// 两个细节：
//  1. 存副本而不是记住原路径——原图在下载目录里，过两天被清理掉，头像就白了。
//  2. 文件名用内容散列——同一张图反复选不会多出副本；换了图就是新文件，
//     art:// 的 URL 跟着变，浏览器缓存不会拿出旧头像（头像 URL 是按路径算的）。
//     旧文件故意不删：几十 KB 的东西，删错比留着难受。
func (s *Store) SaveAvatar(src string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("config: 读头像失败: %w", err)
	}
	defer func() { _ = f.Close() }()

	// 多读一个字节：读满 maxAvatarBytes+1 就说明超标了
	raw, err := io.ReadAll(io.LimitReader(f, maxAvatarBytes+1))
	if err != nil {
		return "", fmt.Errorf("config: 读头像失败: %w", err)
	}
	if len(raw) == 0 {
		return "", errors.New("config: 这个文件是空的，换一张图试试")
	}
	if len(raw) > maxAvatarBytes {
		return "", fmt.Errorf("config: 头像太大了（上限 %d MB）", maxAvatarBytes>>20)
	}

	// 按内容判类型：把 .txt 改名成 .png 也糊弄不过去
	ext, ok := avatarTypes[http.DetectContentType(raw)]
	if !ok {
		return "", errors.New("config: 只支持 PNG / JPEG / WebP / GIF 图片")
	}

	dir := filepath.Join(s.dir, "avatars")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("config: 创建头像目录失败: %w", err)
	}

	sum := sha256.Sum256(raw)
	dst := filepath.Join(dir, hex.EncodeToString(sum[:])[:16]+ext)
	if st, err := os.Stat(dst); err == nil && !st.IsDir() && st.Size() == int64(len(raw)) {
		return dst, nil // 同一张图已经存过了
	}
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return "", fmt.Errorf("config: 写头像失败: %w", err)
	}
	return dst, nil
}

// Load 读设置。文件不存在算正常情况，直接给默认值。
//
// 文件损坏（手改坏了、写到一半断电）也不让应用起不来：把坏文件改名留档，
// 然后回默认值。宁可丢一次设置，也不能开不了机。
func (s *Store) Load() (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := os.ReadFile(s.Path())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Default(), nil
		}
		return Default(), fmt.Errorf("config: 读 %s 失败: %w", s.Path(), err)
	}

	var v Settings
	if err := json.Unmarshal(raw, &v); err != nil {
		backup := s.Path() + ".broken"
		if renameErr := os.Rename(s.Path(), backup); renameErr != nil {
			return Default(), fmt.Errorf("config: 解析 %s 失败且改名留档也失败: %w", s.Path(), err)
		}
		return Default(), fmt.Errorf("config: 解析 %s 失败（已改名为 %s）: %w", s.Path(), backup, err)
	}

	// 老配置里没有新字段（Go 读出来是零值），或者被人手改成了怪值：
	// 在这里统一收敛一次，界面就不会拿到 0 / 空串这类「看着合法、其实没意义」的值。
	v.IconPos = NormalizeIconPos(v.IconPos)
	v.SortBy = NormalizeSortBy(v.SortBy)
	v.SortDir = NormalizeSortDir(v.SortDir)
	v.Order = NormalizeOrder(v.Order)
	// 「自定义」但一个 key 都没有（老配置、手改坏了）：回落到最近游玩，
	// 否则界面会显示「自定义」却什么也排不出来。
	if v.SortBy == SortByManual && len(v.Order) == 0 {
		v.SortBy = SortByRecent
	}
	return v, nil
}

// Save 原子地写设置：先写同目录临时文件再 rename，
// 这样中途崩溃也不会留下半个 JSON。
func (s *Store) Save(v Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("config: 创建配置目录失败: %w", err)
	}

	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("config: 序列化设置失败: %w", err)
	}
	raw = append(raw, '\n')

	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("config: 写临时文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.Path()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("config: 替换 %s 失败: %w", s.Path(), err)
	}
	return nil
}
