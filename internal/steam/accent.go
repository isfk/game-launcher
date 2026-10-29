package steam

import (
	"image"
	_ "image/gif"  // 注册解码器
	_ "image/jpeg" // Steam 封面都是 jpeg
	_ "image/png"  // logo 是 png
	"math"
	"os"
)

// AccentFromImage 从一张封面图里取主色，返回 "#rrggbb"。
//
// 做法：按网格采样、丢掉接近黑白的像素、用饱和度当权重求平均，
// 最后把结果推到「饱和度高一点、亮度居中」，这样背景光晕才有灯箱感，
// 而不是把封面糊成一团泥。
func AccentFromImage(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	img, _, err := image.Decode(f)
	if err != nil {
		return "", err
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return "", os.ErrInvalid
	}

	// 最多采 48×48 个点，跟原图大小无关，够稳也够快。
	const grid = 48
	stepX := max(1, width/grid)
	stepY := max(1, height/grid)

	var sumR, sumG, sumB, weight float64
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, _ := img.At(x, y).RGBA()
			rf := float64(r>>8) / 255
			gf := float64(g>>8) / 255
			bf := float64(b>>8) / 255

			h, s, l := rgbToHSL(rf, gf, bf)
			_ = h
			// 纯黑、纯白、高光不参与：它们只会把颜色冲淡。
			if l < 0.06 || l > 0.94 {
				continue
			}
			w := s*s + 0.02 // 无彩色的像素几乎不投票
			sumR += rf * w
			sumG += gf * w
			sumB += bf * w
			weight += w
		}
	}

	if weight == 0 {
		return "#3a4a6a", nil
	}

	h, _, _ := rgbToHSL(sumR/weight, sumG/weight, sumB/weight)
	return hslToHex(h, 0.62, 0.46), nil
}

// rgbToHSL 返回 h∈[0,1)、s、l。
func rgbToHSL(r, g, b float64) (h, s, l float64) {
	mx := math.Max(r, math.Max(g, b))
	mn := math.Min(r, math.Min(g, b))
	l = (mx + mn) / 2
	d := mx - mn
	if d == 0 {
		return 0, 0, l
	}
	if l > 0.5 {
		s = d / (2 - mx - mn)
	} else {
		s = d / (mx + mn)
	}
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6) / 6
	case g:
		h = ((b-r)/d + 2) / 6
	default:
		h = ((r-g)/d + 4) / 6
	}
	if h < 0 {
		h += 1
	}
	return h, s, l
}

// hslToHex 把 HSL 转成 #rrggbb。
func hslToHex(h, s, l float64) string {
	h = math.Mod(h+1, 1)
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q

	channel := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(math.Min(1, math.Max(0, v)) * 255))
	}

	const hex = "0123456789abcdef"
	out := make([]byte, 0, 7)
	out = append(out, '#')
	for _, c := range []uint8{channel(h + 1.0/3), channel(h), channel(h - 1.0/3)} {
		out = append(out, hex[c>>4], hex[c&0xf])
	}
	return string(out)
}
