//go:build darwin || linux

package proc

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// RunningIn 报告有没有进程的可执行文件在 dir 里。
// 用 ps 而不是遍历 /proc：两个平台都有 ps，代码只有一份。
func RunningIn(dir string) bool {
	if dir == "" {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// args 是完整命令行，第一个空白前的就是可执行文件路径；
	// 用 -ww 避免长路径被截断。
	out, err := exec.CommandContext(ctx, "ps", "-Ao", "args=ww").Output()
	if err != nil {
		return false
	}

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if within(fields[0], dir) {
			return true
		}
	}
	return false
}
