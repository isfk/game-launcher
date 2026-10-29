//go:build windows

package steam

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// DefaultRoots 返回 Windows 上 Steam 可能所在的根目录：
// 先问注册表，再用常见安装位置兜底。
func DefaultRoots() []string {
	var out []string
	// 用户装的 Steam（最常见）
	out = append(out, filepath.FromSlash(regString(syscall.HKEY_CURRENT_USER,
		`Software\Valve\Steam`, `SteamPath`)))
	// 机器上装的 Steam
	out = append(out, regString(syscall.HKEY_LOCAL_MACHINE,
		`SOFTWARE\WOW6432Node\Valve\Steam`, `InstallPath`))
	out = append(out, regString(syscall.HKEY_LOCAL_MACHINE,
		`SOFTWARE\Valve\Steam`, `InstallPath`))
	// 注册表被清理过时的兜底
	if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
		out = append(out, filepath.Join(pf, "Steam"))
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		out = append(out, filepath.Join(pf, "Steam"))
	}
	return dedupe(out)
}

// regString 读一个 REG_SZ 值，失败返回空串。
func regString(root syscall.Handle, path, name string) string {
	sub, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return ""
	}
	var key syscall.Handle
	if err := syscall.RegOpenKeyEx(root, sub, 0, syscall.KEY_READ, &key); err != nil {
		return ""
	}
	defer func() { _ = syscall.RegCloseKey(key) }()

	valueName, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	size := uint32(len(buf) * 2)
	var typ uint32
	if err := syscall.RegQueryValueEx(key, valueName, nil, &typ,
		(*byte)(unsafe.Pointer(&buf[0])), &size); err != nil {
		return ""
	}
	if typ != syscall.REG_SZ {
		return ""
	}
	return syscall.UTF16ToString(buf[:size/2])
}
