package steam

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAccentFromImageReadsDominantColour(t *testing.T) {
	// 一张纯红的封面：主色应该是红相，而不是灰。
	path := writeSolidJPEG(t, color.RGBA{R: 220, G: 30, B: 40, A: 255})

	accent, err := AccentFromImage(path)
	if err != nil {
		t.Fatalf("AccentFromImage: %v", err)
	}
	if len(accent) != 7 || accent[0] != '#' {
		t.Fatalf("颜色格式不对: %q", accent)
	}

	r := hexByte(accent[1:3])
	g := hexByte(accent[3:5])
	b := hexByte(accent[5:7])
	if r < 150 || r <= g || r <= b {
		t.Fatalf("应该以红为主，实际 %q (r=%d g=%d b=%d)", accent, r, g, b)
	}
}

func TestAccentFromImageSkipsGrayscale(t *testing.T) {
	// 灰色封面没有色相可提取，应该落到兜底色而不是崩掉。
	path := writeSolidJPEG(t, color.RGBA{R: 128, G: 128, B: 128, A: 255})
	accent, err := AccentFromImage(path)
	if err != nil {
		t.Fatalf("AccentFromImage: %v", err)
	}
	if accent == "" {
		t.Fatal("灰色图也该返回一个兜底颜色")
	}
}

func TestAccentFromImageHandlesPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: 90, B: 210, A: 255})
		}
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	accent, err := AccentFromImage(path)
	if err != nil {
		t.Fatalf("AccentFromImage(png): %v", err)
	}
	if hexByte(accent[5:7]) <= hexByte(accent[1:3]) {
		t.Fatalf("蓝色的图应该给出偏蓝的主色，实际 %q", accent)
	}
}

func TestAccentFromImageMissingFile(t *testing.T) {
	if _, err := AccentFromImage(filepath.Join(t.TempDir(), "nope.jpg")); err == nil {
		t.Fatal("不存在的文件应该报错")
	}
}

func writeSolidJPEG(t *testing.T, c color.RGBA) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cover.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 16, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 16; x++ {
			img.Set(x, y, c)
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func hexByte(s string) int {
	n := 0
	for _, c := range s {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		}
	}
	return n
}
