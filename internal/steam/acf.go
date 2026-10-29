package steam

import (
	"strconv"
	"time"
)

// StateFullyInstalled 是 appmanifest 里「装好且可用」的 StateFlags 位。
const StateFullyInstalled = 1 << 2

// Manifest 是 steamapps/appmanifest_<appid>.acf 里我们关心的字段。
type Manifest struct {
	AppID       string
	Name        string
	InstallDir  string
	SizeOnDisk  int64
	StateFlags  int
	BuildID     string
	LastUpdated time.Time
	LastPlayed  time.Time
}

// ParseManifest 解析一份 appmanifest ACF。
func ParseManifest(data []byte) (Manifest, error) {
	root, err := Parse(data)
	if err != nil {
		return Manifest{}, err
	}
	state := root.Obj("AppState")
	if state == nil {
		state = root // 少数第三方工具写出的文件没有 AppState 包裹
	}
	m := Manifest{
		AppID:      state.Str("appid"),
		Name:       state.Str("name"),
		InstallDir: state.Str("installdir"),
		BuildID:    state.Str("buildid"),
		SizeOnDisk: int64(atoi(state.Str("SizeOnDisk"))),
		StateFlags: int(atoi(state.Str("StateFlags"))),
	}
	m.LastUpdated = unix(state.Str("LastUpdated"))
	// 老版本客户端不写 LastPlayed，退回到 LastUpdated。
	if t := unix(state.Str("LastPlayed")); !t.IsZero() {
		m.LastPlayed = t
	} else {
		m.LastPlayed = m.LastUpdated
	}
	return m, nil
}

// FullyInstalled 报告这份清单是不是装好的游戏（而不是下载中、或者只剩清单）。
func (m Manifest) FullyInstalled() bool {
	return m.StateFlags&StateFullyInstalled != 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func unix(s string) time.Time {
	sec, _ := strconv.ParseInt(s, 10, 64)
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
