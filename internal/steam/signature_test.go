package steam

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignatureChangesWhenALibraryChanges(t *testing.T) {
	root, lib2 := fixture(t)

	before := Signature(root)
	if before == "" {
		t.Fatal("指纹不该是空的")
	}
	if again := Signature(root); again != before {
		t.Fatal("同样的库应该给出同样的指纹")
	}

	// 装了一部新游戏
	mustWrite(t, filepath.Join(lib2, "steamapps", "appmanifest_2358720.acf"),
		manifestACF("2358720", "Black Myth: Wukong", "Black Myth Wukong", 4, 1793000000))

	after := Signature(root)
	if after == before {
		t.Fatal("新增 appmanifest 之后指纹应该变")
	}
	if !strings.Contains(after, "appmanifest_2358720.acf") {
		t.Errorf("指纹里应该包含新清单: %q", after)
	}
}

func TestSignatureSeesNewLibraryFolder(t *testing.T) {
	root, _ := fixture(t)
	before := Signature(root)

	// 新加一块盘，写进 libraryfolders.vdf
	third := filepath.Join(t.TempDir(), "ThirdLibrary")
	mustDir(t, filepath.Join(third, "steamapps"))
	mustWrite(t, filepath.Join(root, "steamapps", "libraryfolders.vdf"), `"libraryfolders"
{
	"0" { "path" "`+root+`" }
	"1" { "path" "`+third+`" }
}
`)

	after := Signature(root)
	if after == before {
		t.Fatal("库目录变化之后指纹应该变")
	}
	if !strings.Contains(after, third) {
		t.Errorf("指纹里应该有新库路径: %q", after)
	}
}

func TestSignatureOfMissingRootIsStable(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if a, b := Signature(missing), Signature(missing); a != b {
		t.Errorf("不存在的目录应该稳定: %q vs %q", a, b)
	}
}

func TestSignatureIgnoresUnrelatedFiles(t *testing.T) {
	root, _ := fixture(t)
	before := Signature(root)

	// 游戏自己的存档、日志之类，不该触发重扫
	mustDir(t, filepath.Join(root, "steamapps", "common", "ELDEN RING"))
	mustWrite(t, filepath.Join(root, "steamapps", "common", "ELDEN RING", "save.dat"), "x")
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "logs", "steam.log"), "noise")

	if after := Signature(root); after != before {
		t.Error("无关文件的变化不该改变指纹")
	}
}
