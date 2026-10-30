package imgcheck

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// writePNG 在临时目录里造一张图，名字随便给。
func writePNG(t *testing.T, name string, img image.Image) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// solid 造一张纯色图（模拟「白条」）。
func solid(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// glyph 造一张「透明底 + 中间一块不透明图形」的图（模拟正常 logo）。
func glyph(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// 透明底默认就是全零，这里只画中间那一条
	for y := h / 3; y < 2*h/3; y++ {
		for x := w / 4; x < 3*w/4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	return img
}

// TestBlankDetectsSolidWhite 是这条修复的核心：纯白的 logo.png 必须被判定为空白。
func TestBlankDetectsSolidWhite(t *testing.T) {
	path := writePNG(t, "logo.png", solid(360, 90, color.NRGBA{R: 255, G: 255, B: 255, A: 255}))
	if !Blank(path) {
		t.Error("纯白图应该被判为空白（否则界面上就是一条白条）")
	}
}

// TestBlankDetectsTransparent 全透明的图什么也看不见，同样算空白。
func TestBlankDetectsTransparent(t *testing.T) {
	path := writePNG(t, "logo.png", solid(360, 90, color.NRGBA{}))
	if !Blank(path) {
		t.Error("全透明图应该被判为空白")
	}
}

// TestBlankKeepsRealLogo 正常的「透明底 + 文字」logo 不能被误杀。
func TestBlankKeepsRealLogo(t *testing.T) {
	path := writePNG(t, "logo.png", glyph(360, 90))
	if Blank(path) {
		t.Error("有内容的白字 logo 不该被判为空白")
	}
}

// TestBlankKeepsColorful 彩色的图当然不算空白。
func TestBlankKeepsColorful(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 6), G: uint8(y * 6), B: 128, A: 255})
		}
	}
	if Blank(writePNG(t, "logo.png", img)) {
		t.Error("彩色图不该被判为空白")
	}
}

// TestBlankToleratesBadInput 读不到、不是图片、给了个目录，都不该说「空白」
// （这类问题交给浏览器自己报错降级）。
func TestBlankToleratesBadInput(t *testing.T) {
	dir := t.TempDir()
	if Blank(filepath.Join(dir, "nope.png")) {
		t.Error("文件不存在不该说空白")
	}
	if Blank(dir) {
		t.Error("目录不该说空白")
	}

	junk := filepath.Join(dir, "junk.png")
	if err := os.WriteFile(junk, []byte("这不是图片"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Blank(junk) {
		t.Error("解码失败不该说空白")
	}
}

// TestBlankPathSkipsURL CDN 地址一律当不空白：那是浏览器的事，
// 我们不该为了判断多花一次网络请求。
func TestBlankPathSkipsURL(t *testing.T) {
	if BlankPath("https://cdn.cloudflare.steamstatic.com/steam/apps/1/logo.png") {
		t.Error("CDN 地址不该被判为空白")
	}
	if BlankPath("") {
		t.Error("空路径不该被判为空白")
	}

	// 本地文件走真正的判定
	white := writePNG(t, "logo.png", solid(10, 10, color.NRGBA{R: 255, G: 255, B: 255, A: 255}))
	if !BlankPath(white) {
		t.Error("本地纯白图应该被判为空白")
	}
}

// TestBlankDetectsNearWhiteBand 是真实踩到的形状：那条「白条」不是纯 255 白，
// 而是一片 231~241 的极浅渐变（Steam 的 logo.png 常这样）。
// 只看「是否纯色」会漏掉它，所以还要看「铺满 + 很亮」。
func TestBlankDetectsNearWhiteBand(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 360, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 360; x++ {
			v := uint8(231 + (x*10)/360) // 231..241 的细微渐变
			img.SetNRGBA(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	if !Blank(writePNG(t, "logo.png", img)) {
		t.Error("铺满整幅的极浅渐变白带应该被判为空白")
	}
}

// TestBlankKeepsWhitePlateWithText 白底黑字的牌子是正常 logo（Steam 上不少），
// 它是「亮但有大块深色文字」，不能当成白条干掉。
func TestBlankKeepsWhitePlateWithText(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 200, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 200; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 250, G: 250, B: 250, A: 255})
		}
	}
	// 中间来一块深色「文字」
	for y := 25; y < 55; y++ {
		for x := 40; x < 160; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 20, B: 20, A: 255})
		}
	}
	if Blank(writePNG(t, "logo.png", img)) {
		t.Error("白底黑字的 logo 不该被判为空白")
	}
}
