// Package launch 启动外部程序：Steam 的 URL 协议，以及以后的本地可执行文件。
// 单独成一个包，是为了让 internal/steam 只负责读文件、不依赖 GUI 框架。
package launch

import (
	"context"
	"fmt"
)

// Steam 让 Steam 客户端自己启动一部游戏。
//
// 为什么不用 exe 直接跑：DRM、Steam overlay、云存档、反作弊都依赖客户端
// 接管启动，绕过它轻则 overlay 失效，重则游戏直接崩。
func Steam(ctx context.Context, appID string) error {
	if appID == "" {
		return fmt.Errorf("launch: 缺少 appid")
	}
	return OpenURL(ctx, "steam://rungameid/"+appID)
}

// OpenURL 用系统的方式打开一个 URL（交给默认处理程序）。
// 各平台的实现在 open_<goos>.go 里，都不经过 shell。
func OpenURL(ctx context.Context, url string) error {
	if err := openURL(ctx, url); err != nil {
		return fmt.Errorf("launch: 打开 %s 失败: %w", url, err)
	}
	return nil
}
