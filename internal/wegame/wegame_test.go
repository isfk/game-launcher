package wegame

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"game-launcher/internal/library"
)

// mkGame 造一个「游戏目录 + 主程序」的夹具。
func mkGame(t *testing.T, root, dir string, exes ...string) {
	t.Helper()
	base := filepath.Join(root, dir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, exe := range exes {
		path := filepath.Join(base, exe)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("MZ"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestScanPicksGamesAndSkipsNoise 覆盖 rail_apps 的真实形状：
// 里面既有游戏，也有 WeGame 自己的组件目录。
func TestScanPicksGamesAndSkipsNoise(t *testing.T) {
	root := t.TempDir()

	mkGame(t, root, "LOL", "LOL.exe")
	mkGame(t, root, "DNF", "bin", "DNF.exe")
	// 没有 exe 的目录：不是游戏（或者还没下载完）
	if err := os.MkdirAll(filepath.Join(root, "EmptyGame"), 0o755); err != nil {
		t.Fatal(err)
	}
	// WeGame 自己的组件目录：跳过
	mkGame(t, root, "wegame", "wegame.exe")
	mkGame(t, root, "redist", "vcredist_x64.exe")
	mkGame(t, root, "plugins", "plugin.exe")

	games, err := Scan(context.Background(), root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	got := map[string]bool{}
	for _, g := range games {
		got[g.Title] = true
		if g.Provider != "wegame" {
			t.Errorf("Provider = %q", g.Provider)
		}
		if g.Exec == "" {
			t.Errorf("%s 没挑出主程序", g.Title)
		}
		if g.Key != "wegame:"+g.Title {
			t.Errorf("Key = %q", g.Key)
		}
	}
	if len(got) != 2 || !got["LOL"] || !got["DNF"] {
		t.Fatalf("应该只留 LOL 和 DNF，实际 %v", got)
	}
}

// TestPickExecPrefersSameName 覆盖主程序挑选：与目录同名的优先。
func TestPickExecPrefersSameName(t *testing.T) {
	root := t.TempDir()
	mkGame(t, root, "LOL",
		"LOLClient.exe",               // 名字对不上，但在根目录
		"LOL.exe",                     // 名字对上了 → 应该选它
		filepath.Join("bin", "x.exe"), // 更深
	)

	dir := filepath.Join(root, "LOL")
	got := pickExec(dir, "LOL")
	if filepath.Base(got) != "LOL.exe" {
		t.Errorf("挑到的是 %q，期望 LOL.exe", filepath.Base(got))
	}
}

// TestPickExecSkipsNoise 覆盖噪声可执行文件：卸载器、运行库安装包不能当主程序。
func TestPickExecSkipsNoise(t *testing.T) {
	root := t.TempDir()
	mkGame(t, root, "Game",
		"unins000.exe",
		"vcredist_x64.exe",
		"dxsetup.exe",
		"crashreport.exe",
		"RealGame.exe",
	)

	got := pickExec(filepath.Join(root, "Game"), "Game")
	if filepath.Base(got) != "RealGame.exe" {
		t.Errorf("挑到的是 %q，期望 RealGame.exe", filepath.Base(got))
	}
}

// TestPickExecEmpty 覆盖「目录里全是垃圾」的情况：返回空，让上层跳过。
func TestPickExecEmpty(t *testing.T) {
	root := t.TempDir()
	mkGame(t, root, "Junk", "unins000.exe", "vcredist_x86.exe")
	if got := pickExec(filepath.Join(root, "Junk"), "Junk"); got != "" {
		t.Errorf("应该找不到主程序，实际 %q", got)
	}
}

// TestProviderScanUsesEnv 确认 LAUNCHER_WEGAME_DIR 能接管目录。
func TestProviderScanUsesEnv(t *testing.T) {
	root := t.TempDir()
	mkGame(t, root, "王者荣耀", "王者荣耀.exe")
	t.Setenv("LAUNCHER_WEGAME_DIR", root)

	games, err := Provider{}.Scan(context.Background())
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(games) != 1 || games[0].Title != "王者荣耀" {
		t.Fatalf("没读到环境变量指定的目录: %+v", games)
	}
}

// TestProviderScanWithoutWeGame 确认没装 WeGame 时不报错（空库是合法状态）。
func TestProviderScanWithoutWeGame(t *testing.T) {
	t.Setenv("LAUNCHER_WEGAME_DIR", "")
	if runtime.GOOS != "windows" {
		games, err := Provider{}.Scan(context.Background())
		if err != nil {
			t.Errorf("非 Windows 上不该报错: %v", err)
		}
		if len(games) != 0 {
			t.Errorf("非 Windows 上应该是空列表，实际 %d 部", len(games))
		}
	}
}

// TestLaunchRequiresExec 确认没找到主程序时给的是人话错误，而不是静默失败。
func TestLaunchRequiresExec(t *testing.T) {
	err := (Provider{}).Launch(context.Background(), library.Game{Title: "某游戏", Provider: "wegame"})
	if err == nil {
		t.Fatal("没有 Exec 时应该报错")
	}
	if !strings.Contains(err.Error(), "某游戏") {
		t.Errorf("错误里该说清是哪部游戏: %v", err)
	}
}

// TestSignatureChangesWithNewGame 确认库指纹会跟着目录变化，
// app.go 靠它发现「WeGame 里刚装完一款」。
func TestSignatureChangesWithNewGame(t *testing.T) {
	root := t.TempDir()
	before := Signature([]string{root})

	mkGame(t, root, "新游戏", "新游戏.exe")
	after := Signature([]string{root})

	if before == after {
		t.Error("新增游戏目录后指纹应该变")
	}
	if again := Signature([]string{root}); again != after {
		t.Error("没有变化时指纹应该稳定")
	}
}

// TestSignatureNowUsesEnv 覆盖环境变量路径下的指纹。
func TestSignatureNowUsesEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LAUNCHER_WEGAME_DIR", root)
	mkGame(t, root, "G", "G.exe")

	if SignatureNow() == Signature([]string{t.TempDir()}) {
		t.Error("环境变量指定的目录应该参与指纹")
	}
}
