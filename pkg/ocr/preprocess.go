package ocr

import (
	"image"
	"image/color"

	"github.com/disintegration/imaging"
)

// binarize performs a simple global threshold on a grayscale image.
func binarize(img image.Image, threshold uint8) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bb, _ := img.At(x, y).RGBA()
			gray := uint8((r + g + bb) / 3 >> 8)
			var v uint8 = 255
			if gray <= threshold {
				v = 0
			}
			out.Set(x, y, color.NRGBA{R: v, G: v, B: v, A: 255})
		}
	}
	return out
}

// adaptiveThreshold performs a simple mean adaptive threshold.
func adaptiveThreshold(img image.Image, window int, bias int) *image.NRGBA {
	if window < 3 {
		window = 3
	}
	if window%2 == 0 {
		window++
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	out := imaging.New(w, h, color.NRGBA{255, 255, 255, 255})
	half := window / 2
	ints := make([]int, w*h)
	for y := 0; y < h; y++ {
		rowSum := 0
		for x := 0; x < w; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			v := int((r + g + b) / 3 >> 8)
			rowSum += v
			idx := y*w + x
			if y == 0 {
				ints[idx] = rowSum
			} else {
				ints[idx] = ints[(y-1)*w+x] + rowSum
			}
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			x0, y0 := x-half, y-half
			x1, y1 := x+half, y+half
			if x0 < 0 {
				x0 = 0
			}
			if y0 < 0 {
				y0 = 0
			}
			if x1 >= w {
				x1 = w - 1
			}
			if y1 >= h {
				y1 = h - 1
			}
			A := ints[y0*w+x0]
			B := ints[y0*w+x1]
			C := ints[y1*w+x0]
			D := ints[y1*w+x1]
			sum := D - B - C + A
			mean := sum / ((x1 - x0 + 1) * (y1 - y0 + 1))
			rv, gv, bv, _ := img.At(x, y).RGBA()
			pix := int((rv + gv + bv) / 3 >> 8)
			th := mean - bias
			if th < 0 {
				th = 0
			}
			var c color.NRGBA
			if pix < th {
				c = color.NRGBA{0, 0, 0, 255}
			} else {
				c = color.NRGBA{255, 255, 255, 255}
			}
			out.Set(x, y, c)
		}
	}
	return out
}

// dilate performs a simple 4-neighborhood dilation radius times.
func dilate(img *image.NRGBA, radius int) *image.NRGBA {
	if radius <= 0 {
		return img
	}
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	cur := img
	for r := 0; r < radius; r++ {
		next := imaging.New(w, h, color.NRGBA{255, 255, 255, 255})
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				black := false
				for _, d := range [][2]int{{0, 0}, {1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
					x2 := x + d[0]
					y2 := y + d[1]
					if x2 < 0 || y2 < 0 || x2 >= w || y2 >= h {
						continue
					}
					rv, gv, bv, _ := cur.At(x2, y2).RGBA()
					if rv+gv+bv == 0 {
						black = true
						break
					}
				}
				if black {
					next.Set(x, y, color.NRGBA{0, 0, 0, 255})
				}
			}
		}
		cur = next
	}
	return cur
}
