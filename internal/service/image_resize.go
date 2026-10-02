package service

import (
	"image"
	"image/color"
)

// fitInside уменьшает картинку так, чтобы она вписалась в квадрат side×side:
// длинная сторона становится равна side, пропорции сохраняются. Поэтому панорама
// и вертикальный снимок с телефона дают миниатюры одного размера.
func fitInside(src image.Image, side int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= side && h <= side {
		return src
	}
	tw, th := side, h*side/w
	if h > w {
		tw, th = w*side/h, side
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	return resizeBox(src, tw, th)
}

// resizeBox собирает результат из средних значений по областям источника:
// простое прореживание пикселей давало бы рваные края на сильном уменьшении.
// Результат непрозрачный — прозрачные пиксели подмешиваются к белому, иначе PNG
// с прозрачностью превращался бы в JPEG-миниатюре в чёрные пятна.
func resizeBox(src image.Image, tw, th int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		y0 := sb.Min.Y + y*sh/th
		y1 := sb.Min.Y + (y+1)*sh/th
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < tw; x++ {
			x0 := sb.Min.X + x*sw/tw
			x1 := sb.Min.X + (x+1)*sw/tw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, b, a, n = r+uint64(cr), g+uint64(cg), b+uint64(cb), a+uint64(ca), n+1
				}
			}
			if n == 0 {
				continue
			}
			// Значения источника уже умножены на альфу, поэтому белый фон под
			// прозрачным просто добавляется к ним.
			r, g, b, a = r/n, g/n, b/n, a/n
			dst.SetRGBA(x, y, color.RGBA{
				R: to8(r + (0xffff - a)),
				G: to8(g + (0xffff - a)),
				B: to8(b + (0xffff - a)),
				A: 255,
			})
		}
	}
	return dst
}

func to8(v uint64) uint8 {
	v >>= 8
	if v > 255 {
		return 255
	}
	return uint8(v)
}
