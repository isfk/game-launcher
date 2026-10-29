// Package pad 把 mygo 的手柄读取包装成「帧」。
//
// 页面只关心按了哪些键、摇杆推到哪个方向这样的离散量，不关心模拟量：
// 量化在这里做完，界面那边就不用担心摇杆抖动和 60Hz 重绘。
package pad

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/plugins/game/gamepad"
)

// State 是一只手柄在某一刻的可操作状态。
type State struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind,omitempty"` // 键位体系：xbox / playstation / switch
	Buttons   []string `json:"buttons"`        // 此刻按住的动作键
	Dir       string   `json:"dir,omitempty"`  // 方向：up/down/left/right
	Connected bool     `json:"connected"`
}

// 键位体系。按键本身的语义在 mygo 里已经统一成 Xbox 标准布局
// （south = 下方那个键），这里只决定界面上画什么符号。
const (
	KindXbox        = "xbox"
	KindPlayStation = "playstation"
	KindSwitch      = "switch"
)

// KindOf 从设备名猜键位体系。mygo 只给名字，所以这里靠名字里的关键字。
// 认不出来的一律当 Xbox —— XInput 手柄在 Windows 上名字也很朴素。
func KindOf(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "dualsense"), strings.Contains(n, "dualshock"),
		strings.Contains(n, "playstation"), strings.Contains(n, "sony"),
		strings.Contains(n, "ps4"), strings.Contains(n, "ps5"):
		return KindPlayStation
	case strings.Contains(n, "nintendo"), strings.Contains(n, "switch"),
		strings.Contains(n, "joy-con"), strings.Contains(n, "joycon"),
		strings.Contains(n, "pro controller"):
		return KindSwitch
	default:
		return KindXbox
	}
}

// 标准布局 → 界面认识的按键名。方向键单独用 Dir 表示。
var buttonNames = []struct {
	button gamepad.Button
	name   string
}{
	{gamepad.South, "south"}, // A / ✕
	{gamepad.East, "east"},   // B / ○
	{gamepad.West, "west"},   // X / □
	{gamepad.North, "north"}, // Y / △
	{gamepad.LeftShoulder, "lb"},
	{gamepad.RightShoulder, "rb"},
	{gamepad.LeftTrigger, "lt"},
	{gamepad.RightTrigger, "rt"},
	{gamepad.Back, "back"},
	{gamepad.Start, "start"},
	{gamepad.LeftStickButton, "lsb"},
	{gamepad.RightStickButton, "rsb"},
	{gamepad.Guide, "guide"},
}

// Snapshot 读一次所有手柄。没有手柄时返回空切片。
func Snapshot() []State {
	pads := gamepad.Pads()
	out := make([]State, 0, len(pads))
	for _, p := range pads {
		if p == nil || !p.Connected() {
			continue
		}
		st := p.State()
		buttons := make([]string, 0, len(buttonNames))
		for _, bn := range buttonNames {
			if st.Pressed(bn.button) {
				buttons = append(buttons, bn.name)
			}
		}
		sort.Strings(buttons) // 稳定的顺序，方便下面比较签名
		out = append(out, State{
			Name:      p.Name(),
			Kind:      KindOf(p.Name()),
			Buttons:   buttons,
			Dir:       direction(st),
			Connected: true,
		})
	}
	return out
}

// direction 把十字键和左摇杆合成一个方向，十字键优先。
// 摇杆过 0.5 才算推到底，避免轻碰就翻页。
func direction(st gamepad.State) string {
	switch {
	case st.Pressed(gamepad.Up):
		return "up"
	case st.Pressed(gamepad.Down):
		return "down"
	case st.Pressed(gamepad.Left):
		return "left"
	case st.Pressed(gamepad.Right):
		return "right"
	}

	x, y := st.LeftStick()
	switch {
	case y <= -0.5:
		return "up"
	case y >= 0.5:
		return "down"
	case x <= -0.5:
		return "left"
	case x >= 0.5:
		return "right"
	}
	return ""
}

// Signature 把一帧压成字符串：只有签名变了才需要推给页面。
func Signature(states []State) string {
	var b strings.Builder
	for _, s := range states {
		b.WriteString(s.Name)
		b.WriteByte('|')
		b.WriteString(strings.Join(s.Buttons, ","))
		b.WriteByte('|')
		b.WriteString(s.Dir)
		b.WriteByte(';')
	}
	return b.String()
}
