//go:build windows

package launch

import (
	"context"
	"fmt"
	"syscall"
	"unsafe"
)

// shell32 的 ShellExecuteW：系统自己的「用默认程序打开」，
// 不经过 cmd.exe，所以没有命令注入面。
var procShellExecuteW = syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")

// openURL 在 Windows 上调用 ShellExecuteW。
func openURL(_ context.Context, url string) error {
	verb, err := syscall.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	file, err := syscall.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	const swShownormal = 1
	r, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0, 0,
		swShownormal,
	)
	if r <= 32 { // ShellExecuteW 返回值 <= 32 都是错误码
		return fmt.Errorf("ShellExecuteW 返回 %d: %v", r, callErr)
	}
	return nil
}
