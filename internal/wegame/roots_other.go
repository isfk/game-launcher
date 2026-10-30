//go:build !windows

package wegame

// DefaultRoots 在非 Windows 上返回空：WeGame 只有 Windows 客户端。
// 开发机上要试逻辑，用 LAUNCHER_WEGAME_DIR 指一个假目录。
func DefaultRoots() []string { return nil }
