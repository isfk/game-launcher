// Package config 把用户设置存成一个 JSON 文件。
//
// 为什么放后端而不是 localStorage：这些设置要跨窗口、跨重启一致，
// 而且以后「手动添加的游戏」「收藏排序」也要一起落盘 —— 只有一个落盘处，
// 就不会出现「界面记得、后端不知道」的撕扯。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

func Default() Settings {
	return Settings{SoundEnabled: true, IconPos: IconPosBottomLeft}
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
