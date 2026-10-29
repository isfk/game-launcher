// Package library 是启动器的游戏模型：一份与平台无关的「游戏」表示，
// 由各个 Provider（Steam、手动添加，以后的 Epic / WeGame）填充。
// 界面只认这里的 Game，不认平台。
package library

import (
	"context"
	"time"
)

// Artwork 是一部游戏的图片素材。字段是本地绝对路径，
// 由 app 自己的 art:// 协议交给页面（M1 接通），空字符串表示没有。
type Artwork struct {
	Cover string `json:"cover,omitempty"` // 2:3 竖版封面
	Hero  string `json:"hero,omitempty"`  // 16:9 大背景图
	Logo  string `json:"logo,omitempty"`  // 透明底 logo
	Icon  string `json:"icon,omitempty"`
}

// Game 是启动器里的一格。
type Game struct {
	// Key 全局唯一，形如 "steam:1245620"、"manual:<uuid>"，是界面↔后端的句柄。
	Key      string `json:"key"`
	Title    string `json:"title"`
	Provider string `json:"provider"` // steam | manual | demo
	// AppID 是 Steam 的 appid，手动添加的游戏为空。
	AppID      string  `json:"appId,omitempty"`
	InstallDir string  `json:"installDir,omitempty"`
	Artwork    Artwork `json:"artwork"`
	// Accent 是从封面取的主色（#rrggbb），界面用它做背景渐变；空则由前端兜底。
	Accent    string `json:"accent,omitempty"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	// LastPlayed 是最后一次运行的时间，零值表示不知道。
	LastPlayed      *time.Time `json:"lastPlayed,omitempty"`
	PlaytimeMinutes int        `json:"playtimeMinutes,omitempty"`
	// Tags 是 Steam 收藏夹里的分类名。
	Tags      []string `json:"tags,omitempty"`
	Installed bool     `json:"installed"`
	Running   bool     `json:"running"`
}

// Provider 发现本机的游戏，并启动其中一部。
// 加 Epic / WeGame 只是再写一个实现，界面一行不用改。
type Provider interface {
	// ID 是 Provider 的名字，与 Game.Provider 对应。
	ID() string
	// Scan 返回这个平台在本机装好的游戏。
	Scan(ctx context.Context) ([]Game, error)
	// Launch 启动一部游戏。
	Launch(ctx context.Context, g Game) error
}
