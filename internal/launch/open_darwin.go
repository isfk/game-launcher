//go:build darwin

package launch

import (
	"context"
	"os/exec"
)

// openURL 在 macOS 上交给 LaunchServices。
func openURL(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "open", url).Run() // #nosec G204 -- url 只来自本程序拼的 steam:// 或手动添加的游戏
}
