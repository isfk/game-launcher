//go:build ignore

// genicon 画启动器的应用图标：深色圆角砖块 + 主色光晕 + 一只白色手柄。
//
//	go run scripts/genicon/main.go -out resources/icon.png
//
// 为什么用代码画而不是画图工具：图标要在 16×16 到 1024×1024 都可辨认，
// 打包成 .icns / .ico 后边缘质量敏感。程序化绘制（SDF + 4× 超采样）
// 得到确定、干净、可复现的结果，也不依赖任何外部素材。
package main

import (
	"flag"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

const (
	size     = 1024  // 输出边长
	ss       = 4     // 超采样倍数
	squircle = 0.224 // 圆角半径 / 边长，接近 macOS 的 squircle

	// artTileFraction 是素材模式下「砖块」占画布的比例：
	// macOS 的图标就是这么留边的（可见部分比画布小一圈）。
	artTileFraction = 0.82
)

// 主色（与界面一致的金琥珀）
var accent = hex(0xd8a92a)

func main() {
	out := flag.String("out", "resources/icon.png", "输出的 PNG 路径")
	art := flag.String("art", "", "用这张图做图标（居中缩放到圆角砖里）；留空则程序化绘制")
	flag.Parse()

	var small *image.RGBA
	if *art != "" {
		img, err := renderArt(*art)
		if err != nil {
			log.Fatal(err)
		}
		small = img
	} else {
		canvas := size * ss
		small = downsample(render(canvas), ss)
	}

	if err := writePNG(*out, small); err != nil {
		log.Fatal(err)
	}
	log.Printf("图标已写入 %s (%d×%d)", *out, size, size)

	// 小尺寸预览：16/32/128 才是真正考验图标的地方
	for _, s := range []int{16, 32, 128} {
		preview := small
		for preview.Bounds().Dx() > s {
			preview = downsample(preview, 2)
		}
		path := filepath.Join("build", "icons", "icon-"+strconv.Itoa(s)+".png")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			log.Fatal(err)
		}
		if err := writePNG(path, preview); err != nil {
			log.Fatal(err)
		}
		log.Printf("预览已写入 %s", path)
	}
}

// renderArt 用外部素材作图：素材居中缩放到 0.8 倍画布，套上圆角砖遮罩。
// macOS 的图标规范就是这样：可见的砖块比画布小一圈，四周留透明。
func renderArt(path string) (*image.RGBA, error) {
	src, err := loadPNG(path)
	if err != nil {
		return nil, err
	}

	// math.Round 让表达式不再是常量，才能转 int
	tile := int(math.Round(float64(size) * artTileFraction))
	scaled := fitSquare(src, tile*ss, tile*ss)

	// 把砖块贴到画布中间
	canvas := image.NewRGBA(image.Rect(0, 0, size*ss, size*ss))
	offset := (size*ss - tile*ss) / 2
	for y := 0; y < tile*ss; y++ {
		for x := 0; x < tile*ss; x++ {
			canvas.Set(offset+x, offset+y, scaled.RGBAAt(x, y))
		}
	}

	small := downsample(canvas, ss)
	applySquircleMask(small, float64(tile)/float64(size))
	return small, nil
}

// applySquircleMask 把砖块区域裁成圆角（1px 抗锯齿）。
func applySquircleMask(img *image.RGBA, tileFraction float64) {
	n := img.Bounds().Dx()
	f := float64(n)
	tile := f * tileFraction
	half := tile / 2
	center := f / 2
	radius := squircle * tile

	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			lx := float64(x) + 0.5 - center
			ly := float64(y) + 0.5 - center
			d := roundedRectDistance(lx, ly, half, half, radius) // 像素单位
			if d <= 0 {
				continue
			}
			if d >= 1 {
				img.Set(x, y, color.RGBA{})
				continue
			}
			// 边缘一像素做渐变，避免锯齿
			c := img.RGBAAt(x, y)
			c.A = uint8(float64(c.A) * (1 - d))
			img.Set(x, y, c)
		}
	}
}

// loadPNG 读一张 PNG。
func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

// fitSquare 把图按短边居中裁成正方形，再双线性缩放到 w×h。
func fitSquare(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	offX := b.Min.X + (b.Dx()-side)/2
	offY := b.Min.Y + (b.Dy()-side)/2

	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		sy := float64(offY) + (float64(y)+0.5)*float64(side)/float64(h) - 0.5
		for x := 0; x < w; x++ {
			sx := float64(offX) + (float64(x)+0.5)*float64(side)/float64(w) - 0.5
			dst.Set(x, y, bilinear(src, sx, sy))
		}
	}
	return dst
}

// bilinear 双线性采样一个像素。
func bilinear(src image.Image, sx, sy float64) color.RGBA {
	x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
	fx, fy := sx-float64(x0), sy-float64(y0)

	b := src.Bounds()
	at := func(x, y int) color.RGBA {
		if x < b.Min.X {
			x = b.Min.X
		}
		if y < b.Min.Y {
			y = b.Min.Y
		}
		if x >= b.Max.X {
			x = b.Max.X - 1
		}
		if y >= b.Max.Y {
			y = b.Max.Y - 1
		}
		r, g, bl, a := src.At(x, y).RGBA()
		return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)}
	}

	c00, c10 := at(x0, y0), at(x0+1, y0)
	c01, c11 := at(x0, y0+1), at(x0+1, y0+1)

	mixf := func(a, b uint8, t float64) float64 {
		return float64(a)*(1-t) + float64(b)*t
	}

	return color.RGBA{
		R: uint8(mixf(uint8(mixf(c00.R, c10.R, fx)), uint8(mixf(c01.R, c11.R, fx)), fy)),
		G: uint8(mixf(uint8(mixf(c00.G, c10.G, fx)), uint8(mixf(c01.G, c11.G, fx)), fy)),
		B: uint8(mixf(uint8(mixf(c00.B, c10.B, fx)), uint8(mixf(c01.B, c11.B, fx)), fy)),
		A: uint8(mixf(uint8(mixf(c00.A, c10.A, fx)), uint8(mixf(c01.A, c11.A, fx)), fy)),
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// render 在 canvas×canvas 的图上作画。
func render(n int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	f := float64(n)

	drawTile(img, n, f)
	drawGlow(img, n, f)
	drawController(img, n, f)
	roundTile(img, n, f)
	return img
}

// drawTile 深色底板 + 四角冷色晕。
func drawTile(img *image.RGBA, n int, f float64) {
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			u := float64(x) / f
			v := float64(y) / f

			base := mix(hex(0x111722), hex(0x05060a), clamp01(v*1.15))
			d := math.Hypot(u-0.5, v-0.5) * 1.42
			base = mix(base, hex(0x0a1020), clamp01(d*d)*0.55)

			img.Set(x, y, base)
		}
	}
}

// drawGlow 手柄背后往上打一层主色光。
func drawGlow(img *image.RGBA, n int, f float64) {
	const cx, cy = 0.5, 0.47
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			u := float64(x) / f
			v := float64(y) / f
			d := math.Hypot((u-cx)*0.9, (v-cy)*1.1)
			w := math.Exp(-(d * d) / (2 * 0.155 * 0.155))
			if w < 0.004 {
				continue
			}
			img.Set(x, y, addLight(img.RGBAAt(x, y), accent, w*0.95))
		}
	}
}

// drawController 用 SDF 画手柄：主体横条 + 两个握把，平滑并集。
func drawController(img *image.RGBA, n int, f float64) {
	ink := hex(0x0b0e16)
	topFace := hex(0xffffff)
	bottomFace := hex(0xd7dde9)
	// SDF 用归一化坐标（1.0 = 整块画布），所以 2.6px 要换算成 2.6/1024
	const outline = 2.6 / 1024

	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			// 归一化到 [-0.5, 0.5]
			px := float64(x)/f - 0.5
			py := (float64(y)/f - 0.5) * 1.0

			d := controllerSDF(px, py)
			if d > 0 {
				continue
			}
			if d > -outline {
				img.Set(x, y, ink) // 描边
				continue
			}

			// 机身上浅下深的渐变
			t := clamp01((py + 0.24) / 0.48)
			img.Set(x, y, mix(topFace, bottomFace, t*0.8))

			// ---- 细节：全部压在机身内部 ----
			if drawSticks(img, x, y, px, py) {
				continue
			}
			if drawDPad(img, x, y, px, py) {
				continue
			}
			if drawButtons(img, x, y, px, py) {
				continue
			}
		}
	}
}

// controllerSDF 是手柄的轮廓：一条横着的胶囊 + 两根向下的握把，平滑并集。
func controllerSDF(x, y float64) float64 {
	d := sdCapsule(x, y, -0.205, -0.025, 0.205, -0.025, 0.128)
	d = smin(d, sdCapsule(x, y, -0.205, -0.015, -0.25, 0.155, 0.102), 0.095)
	d = smin(d, sdCapsule(x, y, 0.205, -0.015, 0.25, 0.155, 0.102), 0.095)
	return d
}

// drawSticks 两个摇杆：深色外圈 + 主色内圈，是图标的小尺寸焦点。
func drawSticks(img *image.RGBA, x, y int, px, py float64) bool {
	ink := hex(0x0b0e16)
	for _, cx := range []float64{-0.088, 0.088} {
		d := math.Hypot(px-cx, py-0.028)
		switch {
		case d < 0.036:
			img.Set(x, y, accent) // 摇杆帽
			return true
		case d < 0.058:
			img.Set(x, y, ink) // 外圈
			return true
		}
	}
	return false
}

// drawDPad 左边十字方向键。
func drawDPad(img *image.RGBA, x, y int, px, py float64) bool {
	ink := hex(0x0b0e16)
	const cx, cy = -0.218, 0.02
	if math.Abs(px-cx) < 0.028 && math.Abs(py-cy) < 0.072 {
		img.Set(x, y, ink)
		return true
	}
	if math.Abs(px-cx) < 0.072 && math.Abs(py-cy) < 0.028 {
		img.Set(x, y, ink)
		return true
	}
	return false
}

// drawButtons 右边四颗按键，用 PlayStation 的四个颜色。
func drawButtons(img *image.RGBA, x, y int, px, py float64) bool {
	const cx, cy, ring, r = 0.222, 0.02, 0.048, 0.022
	buttons := []struct {
		dx, dy float64
		c      color.RGBA
	}{
		{0, -ring, hex(0x6ed3a1)}, // △ 绿
		{ring, 0, hex(0xe8695f)},  // ○ 红
		{0, ring, hex(0x7f9bea)},  // ✕ 蓝
		{-ring, 0, hex(0xe884b4)}, // □ 粉
	}
	for _, b := range buttons {
		if math.Hypot(px-(cx+b.dx), py-(cy+b.dy)) < r {
			img.Set(x, y, b.c)
			return true
		}
	}
	return false
}

// ---- SDF 基元 ----

// sdCapsule 点到线段 ab 的距离减去半径。
func sdCapsule(px, py, ax, ay, bx, by, r float64) float64 {
	pax, pay := px-ax, py-ay
	bax, bay := bx-ax, by-ay
	denom := bax*bax + bay*bay
	h := 0.0
	if denom > 0 {
		h = clamp01((pax*bax + pay*bay) / denom)
	}
	return math.Hypot(pax-bax*h, pay-bay*h) - r
}

// smin 平滑并集：让机身和握把之间长出圆润的过渡。
func smin(a, b, k float64) float64 {
	h := clamp01(0.5 + 0.5*(b-a)/k)
	return b*(1-h) + a*h - k*h*(1-h)
}

// roundedRectDistance 返回点到圆角矩形的有符号距离（内侧 ≤ 0）。
func roundedRectDistance(x, y, halfW, halfH, radius float64) float64 {
	r := math.Min(radius, math.Min(halfW, halfH))
	dx := math.Abs(x) - (halfW - r)
	dy := math.Abs(y) - (halfH - r)
	outside := math.Hypot(math.Max(dx, 0), math.Max(dy, 0))
	inside := math.Min(math.Max(dx, dy), 0)
	return outside + inside - r
}

// roundTile 把整个画布裁成圆角方块（外面透明）。
func roundTile(img *image.RGBA, n int, f float64) {
	radius := squircle * f
	half := f / 2
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			lx := float64(x) - half + 0.5
			ly := float64(y) - half + 0.5
			if roundedRectDistance(lx, ly, half, half, radius) > 0 {
				img.Set(x, y, color.RGBA{})
			}
		}
	}
}

func downsample(src *image.RGBA, factor int) *image.RGBA {
	w := src.Bounds().Dx() / factor
	h := src.Bounds().Dy() / factor
	dst := image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b, a uint64
			for dy := 0; dy < factor; dy++ {
				for dx := 0; dx < factor; dx++ {
					c := src.RGBAAt(x*factor+dx, y*factor+dy)
					r += uint64(c.R)
					g += uint64(c.G)
					b += uint64(c.B)
					a += uint64(c.A)
				}
			}
			cnt := uint64(factor * factor)
			dst.Set(x, y, color.RGBA{uint8(r / cnt), uint8(g / cnt), uint8(b / cnt), uint8(a / cnt)})
		}
	}
	return dst
}

// ---- 颜色小工具 ----

func hex(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	return color.RGBA{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		A: 255,
	}
}

// addLight 往底色上叠一层光。
func addLight(base, light color.RGBA, w float64) color.RGBA {
	return color.RGBA{
		R: uint8(math.Min(255, float64(base.R)+float64(light.R)*w*0.85)),
		G: uint8(math.Min(255, float64(base.G)+float64(light.G)*w*0.72)),
		B: uint8(math.Min(255, float64(base.B)+float64(light.B)*w*0.35)),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	return math.Max(0, math.Min(1, v))
}
