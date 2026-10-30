// Package imgcheck 判断一张图是不是「视觉上的空白图」。
//
// 为什么需要：有些游戏的 logo.png 是一整片纯白，或者全透明。
// 前者画到界面上就是一条白条（比没有 logo 还难看），后者什么都不显示
// 却占着位置。这两种情况都该当成「没有 logo」，让界面退回文字标题。
//
// 为什么不在前端做：页面加载图片走的是 art:// 自定义协议（Windows 上是
// http://art.localhost），属于独立源，画进 canvas 会把 canvas 标记为污染，
// getImageData 直接抛异常。所以这个判断只能在 Go 这边做。
package imgcheck

import (
	"image"
	"image/color"
	_ "image/gif"  // 注册解码器
	_ "image/jpeg" //
	_ "image/png"  //
	"os"
)

// maxSamples 是采样上限：整张图最多看这么多像素。
// logo 通常几千像素宽，逐像素扫没必要，均匀抽样足够判断「是不是一片纯色」。
const maxSamples = 4096

// 判定阈值。
const (
	alphaVisible = 12 // 低于这个 alpha 算看不见
	solidSpread  = 12 // 亮度跨度小于这个值就算「纯色一片」
	// fullCoverage 是「铺满整张图」的阈值。铺满 + 很亮/很平 = 白条；
	// 只占一小块 = 透明底上的白字 logo，那是正常素材，不能杀。
	fullCoverage = 0.95
	// washedLum 是「整体亮到发白」的均值阈值。铺满整幅又这么亮，
	// 不管有没有细微渐变，画到深色界面上都是一条白带。
	washedLum = 0.88
)

// Blank 报告这张图是不是视觉空白（读不出来也算「不空白」，
// 交给浏览器自己报错降级，免得解码器的小毛病把正常图干掉）。
func Blank(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}

	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return false
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w == 0 || h == 0 {
		return true
	}

	// 均匀取样：按面积开方算步长，大图多跳几格，小图逐像素
	stride := 1
	for (w/stride)*(h/stride) > maxSamples {
		stride++
	}

	var (
		opaque int
		total  int
		lumMin = 1.0
		lumMax = -1.0
		lumSum float64
	)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stride {
		for x := bounds.Min.X; x < bounds.Max.X; x += stride {
			r, g, b, a := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).RGBA()
			total++

			if a>>8 < alphaVisible {
				continue // 透明的地方不参与「纯色」判断
			}
			opaque++

			// 感知亮度（0..1），用整数近似避免浮点噪音
			lum := (0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(b>>8)) / 255
			if lum < lumMin {
				lumMin = lum
			}
			if lum > lumMax {
				lumMax = lum
			}
			lumSum += lum
		}
	}

	// 整张图都透明：画出来什么都没有
	if opaque == 0 || total == 0 {
		return true
	}

	// 只占画面一小块（透明底上的白字 logo）→ 是正常素材，留着
	if float64(opaque)/float64(total) < fullCoverage {
		return false
	}

	// 铺满整幅了，再看它是「一片平坦」还是「亮得发白」：
	// - 纯色（哪怕有压缩噪点）→ 白条/黑条
	// - 均值很亮 → 白底带
	// 白底黑字的牌子不算：它亮度跨度大、均值也没那么高，会留在下面这条。
	if lumMax-lumMin <= float64(solidSpread)/255 {
		return true
	}
	return lumSum/float64(opaque) >= washedLum
}

// BlankPath 对本地文件路径做判定；http(s) 地址一律当不空白——
// 那是浏览器的事，我们不在下载前多花一次网络请求。
func BlankPath(path string) bool {
	if path == "" {
		return false
	}
	if len(path) > 7 && (path[:7] == "http://" || path[:8] == "https://") {
		return false
	}
	return Blank(path)
}
