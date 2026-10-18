package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"

	nativeui "github.com/egoist/mygo/ui"
)

const (
	brandBlue  = "#2563EB"
	brandBlueR = 37
	brandBlueG = 99
	brandBlueB = 235
)

// brandMark is the vector form of the product mark. Keep the viewBox at 64
// units so the same proportions remain legible in a 16px navigation mark and
// in a larger product lockup.
var brandMark = nativeui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none">
  <rect x="2" y="2" width="60" height="60" rx="15" fill="#2563EB"/>
  <path d="M17 24h30" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
  <path d="m38 17 9 7-9 7" stroke="#fff" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M47 40H17" stroke="#fff" stroke-width="5" stroke-linecap="round"/>
  <path d="m26 33-9 7 9 7" stroke="#fff" stroke-width="5" stroke-linecap="round" stroke-linejoin="round"/>
  <circle cx="16" cy="32" r="5" fill="#2563EB" stroke="#fff" stroke-width="3"/>
  <circle cx="48" cy="32" r="5" fill="#2563EB" stroke="#fff" stroke-width="3"/>
</svg>`))

var (
	brandIconOnce sync.Once
	brandIconData []byte
)

// brandIconPNG returns the shared raster asset used by the native window and
// notification tray. A 32px image is the native tray size on high-DPI
// Windows (16 logical pixels); the OS can downsample it for the taskbar.
func brandIconPNG() []byte {
	brandIconOnce.Do(func() {
		brandIconData = renderBrandPNG(32)
	})
	return brandIconData
}

// brandBadge places the color mark used by the sidebar product lockup. The
// SVG is rendered at its native square size so its rounded corners and white
// route remain crisp on both light and dark themes.
func (m *model) brandBadge(c *nativeui.Context) {
	nativeui.Image(c, brandMark).Size(32, 32).Fit(nativeui.Contain).Label("Proxy Start")
}

func renderBrandPNG(size int) []byte {
	if size < 1 {
		return nil
	}
	const supersample = 4
	// Render the 64-unit source canvas at a fixed supersampled resolution, then
	// downsample to the requested output size. Using size*supersample here
	// would clip the fixed 64-unit geometry for 16px and 32px outputs.
	largeSize := 64 * supersample
	img := image.NewNRGBA(image.Rect(0, 0, largeSize, largeSize))
	blue := color.NRGBA{R: brandBlueR, G: brandBlueG, B: brandBlueB, A: 255}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	// The SVG's 64-unit geometry is mirrored here so the window/taskbar/tray
	// raster and the in-app vector mark keep the same silhouette.
	fillRoundedRect(img, 2, 2, 62, 62, 15, blue, supersample)
	fillLine(img, 17, 24, 47, 24, 5, white, supersample)
	fillLine(img, 38, 17, 47, 24, 5, white, supersample)
	fillLine(img, 47, 24, 38, 31, 5, white, supersample)
	fillLine(img, 47, 40, 17, 40, 5, white, supersample)
	fillLine(img, 26, 33, 17, 40, 5, white, supersample)
	fillLine(img, 17, 40, 26, 47, 5, white, supersample)
	fillCircle(img, 16, 32, 5, blue, supersample)
	fillCircle(img, 16, 32, 6.5, white, supersample)
	fillCircle(img, 16, 32, 5, blue, supersample)
	fillCircle(img, 48, 32, 6.5, white, supersample)
	fillCircle(img, 48, 32, 5, blue, supersample)

	img = downsampleBrand(img, size)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

func fillRoundedRect(img *image.NRGBA, x0, y0, x1, y1, radius float64, fill color.NRGBA, scale int) {
	minX := int(math.Floor(x0 * float64(scale)))
	minY := int(math.Floor(y0 * float64(scale)))
	maxX := int(math.Ceil(x1 * float64(scale)))
	maxY := int(math.Ceil(y1 * float64(scale)))
	r := radius * float64(scale)
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			cx := clamp(px, float64(minX)+r, float64(maxX)-r)
			cy := clamp(py, float64(minY)+r, float64(maxY)-r)
			dx, dy := px-cx, py-cy
			if dx*dx+dy*dy <= r*r {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
}

func fillCircle(img *image.NRGBA, cx, cy, radius float64, fill color.NRGBA, scale int) {
	cx *= float64(scale)
	cy *= float64(scale)
	radius *= float64(scale)
	minX := int(math.Floor(cx - radius))
	minY := int(math.Floor(cy - radius))
	maxX := int(math.Ceil(cx + radius))
	maxY := int(math.Ceil(cy + radius))
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= radius*radius {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
}

func fillLine(img *image.NRGBA, x0, y0, x1, y1, width float64, fill color.NRGBA, scale int) {
	x0 *= float64(scale)
	y0 *= float64(scale)
	x1 *= float64(scale)
	y1 *= float64(scale)
	width *= float64(scale)
	minX := int(math.Floor(math.Min(x0, x1) - width/2))
	minY := int(math.Floor(math.Min(y0, y1) - width/2))
	maxX := int(math.Ceil(math.Max(x0, x1) + width/2))
	maxY := int(math.Ceil(math.Max(y0, y1) + width/2))
	dx, dy := x1-x0, y1-y0
	lengthSquared := dx*dx + dy*dy
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			var t float64
			if lengthSquared > 0 {
				t = ((px-x0)*dx + (py-y0)*dy) / lengthSquared
				t = clamp(t, 0, 1)
			}
			qx, qy := x0+t*dx, y0+t*dy
			ddx, ddy := px-qx, py-qy
			if ddx*ddx+ddy*ddy <= (width/2)*(width/2) {
				img.SetNRGBA(x, y, fill)
			}
		}
	}
}

func downsampleBrand(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for sy := y * sh / size; sy < (y+1)*sh/size; sy++ {
				for sx := x * sw / size; sx < (x+1)*sw/size; sx++ {
					c := src.NRGBAAt(sx, sy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			count := uint32((sw / size) * (sh / size))
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / count), G: uint8(g / count), B: uint8(b / count), A: uint8(a / count)})
		}
	}
	return dst
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
