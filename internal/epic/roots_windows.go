//go:build windows

package epic

import (
	"os"
	"path/filepath"
)

// DefaultRoots 返回 Windows 上 Epic 的 Manifests 目录。
//
// 装在默认位置时是 C:\ProgramData\Epic\EpicGamesLauncher\Data\Manifests，
// 但 ProgramData 可以被改到别的盘，所以按环境变量拼而不是写死。
func DefaultRoots() []string {
	programData := os.Getenv("ProgramData")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return []string{
		filepath.Join(programData, "Epic", "EpicGamesLauncher", "Data", "Manifests"),
	}
}
