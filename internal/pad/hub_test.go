package pad

import "testing"

func TestKindOf(t *testing.T) {
	cases := map[string]string{
		"Xbox Wireless Controller":       KindXbox,
		"Xbox 360 Controller":            KindXbox,
		"XInput Controller":              KindXbox,
		"北通游戏手柄":                         KindXbox, // XInput 兼容，按 Xbox 画
		"DualSense Wireless Controller":  KindPlayStation,
		"DualShock 4":                    KindPlayStation,
		"Sony Computer Entertainment":    KindPlayStation,
		"PS5 Controller":                 KindPlayStation,
		"Pro Controller":                 KindSwitch,
		"Nintendo Switch Pro Controller": KindSwitch,
		"Joy-Con (L)":                    KindSwitch,
		"":                               KindXbox, // 没名字也按 Xbox
	}
	for name, want := range cases {
		if got := KindOf(name); got != want {
			t.Errorf("KindOf(%q) = %q，想要 %q", name, got, want)
		}
	}
}
