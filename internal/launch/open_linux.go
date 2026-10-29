//go:build linux

package launch

import (
	"context"
	"os/exec"
)

// openURL 在 Linux 上交给 xdg-open。
func openURL(ctx context.Context, url string) error {
	return exec.CommandContext(ctx, "xdg-open", url).Run() // #nosec G204 -- url 只来自本程序拼的 steam:// 或手动添加的游戏
}
