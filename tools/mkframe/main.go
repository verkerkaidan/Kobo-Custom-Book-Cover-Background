// Generates assets/frame.png: an example frame, built to the constraints that
// matter on this hardware.
//
//   - 1072x1448, the panel size, so it is used pixel-for-pixel with no resampling
//   - a fully transparent window for the cover, punched at 1x so its edges stay
//     exact and detectWindow() finds precisely this rectangle
//   - window is 52% of the panel, far above the 15% floor that guards against
//     stray erased pixels being mistaken for a window
//   - nothing but flat dark below the window, where the stats are drawn
//   - two colours, high contrast, bold shapes: the Kaleido panel mutes
//     saturation and drops effective resolution, so fine detail would mush
//
// Ornaments are drawn at 2x and downsampled for smooth edges; the window is
// punched afterwards at 1x so it is not softened by the resample.
//
//	go run ./tools/mkframe assets/frame.png
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

const (
	W, H = 1072, 1448
	SS   = 2 // supersampling factor
)

// Matches the suggested window in frame-template.png. Its bottom edge sits
// exactly where the stats block begins.
var window = image.Rect(110, 170, 962, 1120)

var (
	ink   = color.RGBA{22, 26, 38, 255}
	gold  = color.RGBA{198, 164, 92, 255}
	cream = color.RGBA{236, 230, 214, 255}
)

func main() {
	out := "assets/frame.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	big := image.NewRGBA(image.Rect(0, 0, W*SS, H*SS))
	draw.Draw(big, big.Bounds(), &image.Uniform{ink}, image.Point{}, draw.Src)

	win := scaleRect(window, SS)

	// Outer double rule.
	outline(big, inset(big.Bounds(), 26*SS), 3*SS, gold)
	outline(big, inset(big.Bounds(), 40*SS), 1*SS, gold)

	// Double rule hugging the window.
	outline(big, grow(win, 20*SS), 3*SS, gold)
	outline(big, grow(win, 30*SS), 1*SS, cream)

	// Corner blocks: nested squares, the boldest element so they survive the
	// panel's resolution loss. Top two only — the bottom corners would land
	// under the date line, which is exactly what the guide tells you not to do.
	for _, c := range corners(inset(big.Bounds(), 26*SS), 76*SS)[:2] {
		fillRect(big, c, gold)
		fillRect(big, inset(c, 12*SS), ink)
		fillRect(big, inset(c, 24*SS), cream)
		fillRect(big, inset(c, 34*SS), ink)
	}

	// Diamond runs along the top and the two sides — but NOT below the window,
	// where the stats go.
	diamondRun(big, W*SS/2, 98*SS, 62*SS, 5, true) // top, horizontal
	sideY := (win.Min.Y + win.Max.Y) / 2
	diamondRun(big, 68*SS, sideY, 62*SS, 7, false)     // left, vertical
	diamondRun(big, (W-68)*SS, sideY, 62*SS, 7, false) // right, vertical

	// One quiet rule below the text, in the last strip of margin. Sits clear of
	// the stats block, which with text_offset = 32 ends around y=1410.
	fillRect(big, image.Rect(300*SS, 1424*SS, (W-300)*SS, 1427*SS), gold)

	// Downsample for smooth edges.
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	xdraw.CatmullRom.Scale(img, img.Bounds(), big, big.Bounds(), draw.Src, nil)

	// Punch the window at 1x: hard edges, exact rectangle, alpha 0.
	draw.Draw(img, window, &image.Uniform{color.RGBA{}}, image.Point{}, draw.Src)

	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}

	area := 100 * window.Dx() * window.Dy() / (W * H)
	fmt.Printf("wrote %s — window %v (%d%% of panel)\n", out, window, area)
}

// --- drawing helpers --------------------------------------------------------

func fillRect(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func outline(img *image.RGBA, r image.Rectangle, w int, c color.RGBA) {
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), c)
	fillRect(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), c)
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), c)
	fillRect(img, image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), c)
}

func inset(r image.Rectangle, d int) image.Rectangle {
	return image.Rect(r.Min.X+d, r.Min.Y+d, r.Max.X-d, r.Max.Y-d)
}

func grow(r image.Rectangle, d int) image.Rectangle { return inset(r, -d) }

func scaleRect(r image.Rectangle, f int) image.Rectangle {
	return image.Rect(r.Min.X*f, r.Min.Y*f, r.Max.X*f, r.Max.Y*f)
}

func corners(r image.Rectangle, size int) []image.Rectangle {
	return []image.Rectangle{
		image.Rect(r.Min.X, r.Min.Y, r.Min.X+size, r.Min.Y+size),
		image.Rect(r.Max.X-size, r.Min.Y, r.Max.X, r.Min.Y+size),
		image.Rect(r.Min.X, r.Max.Y-size, r.Min.X+size, r.Max.Y),
		image.Rect(r.Max.X-size, r.Max.Y-size, r.Max.X, r.Max.Y),
	}
}

// A diamond: |dx|/r + |dy|/r <= 1.
func diamond(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		if y < 0 || y >= img.Bounds().Max.Y {
			continue
		}
		span := r - abs(y-cy)
		for x := cx - span; x <= cx+span; x++ {
			if x < 0 || x >= img.Bounds().Max.X {
				continue
			}
			img.SetRGBA(x, y, c)
		}
	}
}

// n diamonds centred on (cx,cy), alternating gold and cream, with a small
// hollow centre so they read as ornament rather than blobs.
func diamondRun(img *image.RGBA, cx, cy, step, n int, horizontal bool) {
	for i := 0; i < n; i++ {
		off := (i - n/2) * step
		x, y := cx, cy
		if horizontal {
			x += off
		} else {
			y += off
		}
		size := 22 * SS
		if i%2 == 1 {
			size = 14 * SS
		}
		outer, inner := gold, ink
		if i%2 == 1 {
			outer = cream
		}
		diamond(img, x, y, size, outer)
		if i%2 == 0 {
			diamond(img, x, y, size-9*SS, inner)
		}
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
