package application

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
)

// 角色卡头像的取值边界。
//
// 导入时把 PNG 原图整体 base64 塞进卡片 JSON 会连带三处膨胀：建会话请求体
// （几十 MB 的卡直接撞上限）、每个回合的世界快照（SQLite 里按节点复制）与
// 浏览器本地存储。原图在这里只用于生成有界缩略图，之后即丢弃。
const (
	// avatarMaxEdge 是缩略图长边上限。界面里头像最大显示约 60px，立绘灯箱
	// 在 900px 宽内按视口高度缩放，640 足够清晰且能把体积压到百 KB 量级。
	avatarMaxEdge = 640
	// avatarJPEGQuality 是缩略图（不透明图像）的 JPEG 质量。
	avatarJPEGQuality = 85
)

// buildAvatarDataURL 把整幅 PNG 压成有界 data URL。
// 图像无法解码时返回空串，由调用方决定回退策略（此函数不报错）：
// 头像缺失只影响展示，不该让整张角色卡导入失败。
func buildAvatarDataURL(src []byte) string {
	if len(src) == 0 {
		return ""
	}
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return ""
	}
	b := img.Bounds()
	thumb := fitWithin(img, avatarMaxEdge)
	if thumb == nil {
		return ""
	}
	resized := thumb.Bounds().Dx() != b.Dx() || thumb.Bounds().Dy() != b.Dy()

	// 不透明图像用 JPEG（同类素材比 PNG 小数倍）；带透明通道的保留 PNG，
	// 否则透明区域会在深色界面上变成色块。
	var best string
	if isOpaque(thumb) {
		best = encodeDataURL("image/jpeg", func(buf *bytes.Buffer) error {
			return jpeg.Encode(buf, thumb, &jpeg.Options{Quality: avatarJPEGQuality})
		})
	}
	if best == "" {
		best = encodeDataURL("image/png", func(buf *bytes.Buffer) error {
			return png.Encode(buf, thumb)
		})
	}
	if best == "" {
		return ""
	}
	// 原图本来就在尺寸上限内、且比重新编码更小时直接沿用：小尺寸图标重新编码
	// 只会变大（JPEG 不擅长极小图，PNG 头开销也不划算）。
	if !resized {
		original := "data:image/png;base64," + base64.StdEncoding.EncodeToString(src)
		if len(original) < len(best) {
			return original
		}
	}
	return best
}

// encodeDataURL 执行编码并拼成 data URL；编码失败返回空串。
func encodeDataURL(mime string, encode func(*bytes.Buffer) error) string {
	var buf bytes.Buffer
	if err := encode(&buf); err != nil {
		return ""
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// fitWithin 等比缩放到长边不超过 maxEdge；已在范围内时只做像素格式归一。
// 缩放用盒式均值（每个目标像素取源矩形均值），无第三方依赖。
func fitWithin(src image.Image, maxEdge int) *image.RGBA {
	if src == nil {
		return nil
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil
	}
	dw, dh := sw, sh
	if sw > maxEdge || sh > maxEdge {
		if sw >= sh {
			dw = maxEdge
			dh = scaleEdge(sh, maxEdge, sw)
		} else {
			dh = maxEdge
			dw = scaleEdge(sw, maxEdge, sh)
		}
	}
	if dw == sw && dh == sh {
		return toRGBA(src)
	}
	return boxResize(src, dw, dh)
}

// scaleEdge 按等比缩放计算另一条边，至少 1 像素（绝不返回 0 导致空图）。
func scaleEdge(size, target, other int) int {
	v := int(float64(size)*float64(target)/float64(other) + 0.5)
	if v < 1 {
		return 1
	}
	return v
}

// toRGBA 把任意 image.Image 复制成 *image.RGBA（不重采样）。
func toRGBA(src image.Image) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			dst.SetRGBA(x-b.Min.X, y-b.Min.Y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(bl >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

// boxResize 用盒式均值重采样。目标像素与源像素的边界按整数比例切分，
// 保证每个源像素都被恰好一个目标像素覆盖，不产生空洞或重复采样。
func boxResize(src image.Image, dw, dh int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for dy := 0; dy < dh; dy++ {
		y0 := dy * sh / dh
		y1 := (dy + 1) * sh / dh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for dx := 0; dx < dw; dx++ {
			x0 := dx * sw / dw
			x1 := (dx + 1) * sw / dw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var sumR, sumG, sumB, sumA, n uint32
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, bl, a := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
					sumR += r >> 8
					sumG += g >> 8
					sumB += bl >> 8
					sumA += a >> 8
					n++
				}
			}
			if n == 0 {
				continue
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				uint8(sumR / n), uint8(sumG / n), uint8(sumB / n), uint8(sumA / n),
			})
		}
	}
	return dst
}

// isOpaque 检查 alpha 通道是否全为 255。缩放后尺寸有界（≤ maxEdge²），
// 全量扫描比抽样更可靠且开销可忽略。
func isOpaque(img *image.RGBA) bool {
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+img.Rect.Dx()*4]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 0xFF {
				return false
			}
		}
	}
	return true
}
