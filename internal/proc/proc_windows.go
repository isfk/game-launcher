//go:build windows

package proc

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procCreateToolhelp32Snapshot   = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW            = kernel32.NewProc("Process32FirstW")
	procProcess32NextW             = kernel32.NewProc("Process32NextW")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
)

const (
	th32csSnapProcess       = 0x00000002
	processQueryLimitedInfo = 0x1000
	invalidHandleValue      = ^uintptr(0)
	errorInsufficientBuffer = 122
	maxPathW                = 32768
)

// processEntry32 是 PROCESSENTRY32W 的最小可用字段布局。
type processEntry32 struct {
	Size            uint32
	CntUsage        uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	CntThreads      uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// RunningIn 报告有没有进程的可执行文件在 dir 里。
// 走 Toolhelp 快照 + QueryFullProcessImageNameW，不依赖 wmic（Win11 已弃用）。
func RunningIn(dir string) bool {
	if dir == "" {
		return false
	}

	snap, _, _ := procCreateToolhelp32Snapshot.Call(th32csSnapProcess, 0)
	if snap == invalidHandleValue || snap == 0 {
		return false
	}
	defer func() { _, _, _ = procCloseHandle.Call(snap) }()

	var entry processEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))

	ok, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	for ok != 0 {
		if path := imagePath(entry.ProcessID); path != "" && within(path, dir) {
			return true
		}
		ok, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&entry)))
	}
	return false
}

// imagePath 拿一个进程的可执行文件全路径，拿不到就返回空。
// 权限不够（系统进程）会失败，这里直接跳过。
func imagePath(pid uint32) string {
	handle, _, _ := procOpenProcess.Call(processQueryLimitedInfo, 0, uintptr(pid))
	if handle == 0 {
		return ""
	}
	defer func() { _, _, _ = procCloseHandle.Call(handle) }()

	buf := make([]uint16, maxPathW)
	size := uint32(len(buf))
	ok, _, _ := procQueryFullProcessImageNameW.Call(
		handle,
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}
