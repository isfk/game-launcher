//go:build e2e

package main

// 打开「外部进程驱动应用」的能力，只给端到端测试用：
//
//	go build -tags e2e -o /tmp/app . && MYGO_AUTOMATION=/tmp/app.sock /tmp/app
//
// 然后照 docs/plugins/automation.md 的协议连 Unix socket，可以
// windows / eval / capture（截图）/ input（按键）。见 docs/PLAN.md 的测试一节。
import _ "github.com/egoist/mygo/plugins/automation"
