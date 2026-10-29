package steam

import "testing"

const libraryFoldersNew = `"libraryfolders"
{
	"0"
	{
		"path"		"/Users/x/Library/Application Support/Steam"
		"label"		""
		"contentid"		"123"
		"totalsize"		"0"
		"apps"
		{
			"228980"		"123"
		}
	}
	"1"
	{
		"path"		"/Volumes/Games/SteamLibrary"
	}
}
`

const libraryFoldersOld = `"LibraryFolders"
{
	"TimeNextStatsReport"		"1610000000"
	"ContentStatsID"		"-1234567890"
	"1"		"E:\\SteamLibrary"
	"2"		"D:\\Games\\Steam"
}
`

const appManifest = `"AppState"
{
	"appid"		"1245620"
	"name"		"ELDEN RING"
	"StateFlags"		"4"
	"installdir"		"ELDEN RING"
	"SizeOnDisk"		"51380224000"
	"buildid"		"1234"
	"LastUpdated"		"1700000000"
	"LastPlayed"		"1710000000"
}
`

func TestParseLibraryFoldersNewFormat(t *testing.T) {
	obj, err := Parse([]byte(libraryFoldersNew))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	folders := obj.Obj("libraryfolders")
	if folders == nil {
		t.Fatal("libraryfolders 对象没解析出来")
	}
	if got := folders.Obj("0").Str("path"); got != "/Users/x/Library/Application Support/Steam" {
		t.Errorf("path[0] = %q", got)
	}
	if got := folders.Obj("1").Str("path"); got != "/Volumes/Games/SteamLibrary" {
		t.Errorf("path[1] = %q", got)
	}
}

func TestParseLibraryFoldersOldFormat(t *testing.T) {
	obj, err := Parse([]byte(libraryFoldersOld))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	folders := obj.Obj("LibraryFolders")
	if folders == nil {
		t.Fatal("LibraryFolders 对象没解析出来")
	}
	// 老格式里路径直接挂在数字键上，Scan 会跳过不是路径的元数据键
	if got := folders["1"]; got != `E:\SteamLibrary` {
		t.Errorf(`folders["1"] = %#v`, got)
	}
	if got := folders["2"]; got != `D:\Games\Steam` {
		t.Errorf(`folders["2"] = %#v`, got)
	}
}

func TestParseManifest(t *testing.T) {
	m, err := ParseManifest([]byte(appManifest))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.AppID != "1245620" {
		t.Errorf("AppID = %q", m.AppID)
	}
	if m.Name != "ELDEN RING" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.InstallDir != "ELDEN RING" {
		t.Errorf("InstallDir = %q", m.InstallDir)
	}
	if m.SizeOnDisk != 51380224000 {
		t.Errorf("SizeOnDisk = %d", m.SizeOnDisk)
	}
	if !m.FullyInstalled() {
		t.Error("StateFlags 4 应该是装好的")
	}
	if m.LastPlayed.Unix() != 1710000000 {
		t.Errorf("LastPlayed = %d", m.LastPlayed.Unix())
	}
}

func TestManifestNotInstalled(t *testing.T) {
	m, err := ParseManifest([]byte(`"AppState" { "appid" "10" "StateFlags" "2" }`))
	if err != nil {
		t.Fatalf("ParseManifest: %v", err)
	}
	if m.FullyInstalled() {
		t.Error("StateFlags 2（下载中）不该算装好")
	}
}

func TestParseToleratesCommentsAndEscapes(t *testing.T) {
	obj, err := Parse([]byte("// 注释\n\"A\"\n{\n\t\"b\"\t\t\"x\\\\y\" // 尾注释\n}\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := obj.Obj("A").Str("b"); got != `x\y` {
		t.Errorf("b = %q", got)
	}
}
