//go:build darwin || linux

package proc

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRunningInFindsThisProcess 用「测试进程自己的可执行文件」验证探测链路：
// 它就在 os.Executable() 所在目录里，必然能被扫描到。
//
// 为什么不用「造一个子进程」做夹具：在 macOS 上复制 /bin 下的系统二进制
// 会被代码签名检查直接打死（进程立刻变 defunct），测不出东西。
func TestRunningInFindsThisProcess(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("拿不到自己的可执行文件: %v", err)
	}
	dir := filepath.Dir(exe)

	if !RunningIn(dir) {
		t.Fatalf("测试进程就在 %s 里，应该被探测到", dir)
	}

	// 它的上级目录也包含它（前缀匹配要能穿过层级）
	if !RunningIn(filepath.Dir(dir)) {
		t.Fatalf("上级目录 %s 也应该被判定为有进程在跑", filepath.Dir(dir))
	}
}

func TestRunningInEmptyDir(t *testing.T) {
	if RunningIn("") {
		t.Error("空目录应该是 false")
	}
	if RunningIn(t.TempDir()) {
		t.Error("空临时目录应该是 false")
	}
}

func TestWithin(t *testing.T) {
	cases := []struct {
		name      string
		path, dir string
		want      bool
	}{
		{"直接子文件", "/games/elden/elden.exe", "/games/elden", true},
		{"子目录里的文件", "/games/elden/bin/sub/x.exe", "/games/elden", true},
		{"前缀相同但不同目录", "/games/elden2/x.exe", "/games/elden", false},
		{"完全无关", "/games/other/x.exe", "/games/elden", false},
		{"空路径", "", "/games/elden", false},
	}
	for _, c := range cases {
		if got := within(c.path, c.dir); got != c.want {
			t.Errorf("%s: within(%q, %q) = %v，想要 %v", c.name, c.path, c.dir, got, c.want)
		}
	}
}
