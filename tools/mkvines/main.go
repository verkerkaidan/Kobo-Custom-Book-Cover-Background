// Generates assets/frame.png: a semi-transparent vine border.
//
// Unlike mkframe this draws no hole. The whole canvas starts fully
// transparent and the vines are laid over it with partial alpha, so with
// cover_window = full the cover fills the panel and shows through
// the leaves. A soft dark band along the edges keeps the vines legible over
// any cover, light or dark.
//
//   - 1072x1448, the panel size, used pixel-for-pixel
//   - vines along the top, left and right; the sides stop above the stats
//     block; one thin run along the very bottom, below the text
//   - three colours, bold leaf shapes: the Kaleido panel mutes saturation and
//     drops effective resolution, so fine tendrils would mush
//
// Everything is drawn at 2x and downsampled for smooth edges.
//
//	go run ./tools/mkvines assets/frame.png
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"

	xdraw "golang.org/x/image/draw"
)

const (
	W, H = 1072, 1448
	SS   = 2 // supersampling factor
)

// Alpha is the "semi" in semi-transparent: 255 would hide the cover.
var (
	ink   = color.RGBA{24, 44, 30, 215}  // stems, midribs
	leaf  = color.RGBA{72, 128, 78, 190} // leaves
	light = color.RGBA{132, 176, 110, 170}
	berry = color.RGBA{172, 62, 74, 200}
	band  = color.RGBA{18, 26, 20, 255} // edge vignette, alpha set per pixel
)

const (
	bandWidth = 140 // px at 1x the vignette reaches in from each edge
	bandAlpha = 135 // alpha at the very edge
)

func main() {
	out := "assets/frame.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	big := image.NewRGBA(image.Rect(0, 0, W*SS, H*SS))

	vignette(big)

	// The stats block starts around y=1260 with title and date off; the side
	// vines curl to an end well above it, and the bottom run sits under the
	// last text line (~1380).
	//
	// Each vine: start, end, wave amplitude, number of leaves.
	vine(big, pt(40, 96), pt(W-40, 96), 16, 17)      // top
	vine(big, pt(64, 60), pt(64, 1180), 15, 19)      // left
	vine(big, pt(W-64, 60), pt(W-64, 1180), 15, 19)  // right
	vine(big, pt(150, 1420), pt(W-150, 1420), 8, 11) // bottom, thin

	// Corner curls tie the runs together.
	curl(big, pt(70, 70), 38, 0)
	curl(big, pt(W-70, 70), 38, 1)
	curl(big, pt(64, 1215), 30, 2)
	curl(big, pt(W-64, 1215), 30, 3)

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	xdraw.CatmullRom.Scale(img, img.Bounds(), big, big.Bounds(), draw.Src, nil)

	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	fmt.Printf("wrote %s — semi-transparent overlay, no window (set cover_window = full)\n", out)
}

// --- geometry ----------------------------------------------------------------

type p2 struct{ x, y float64 }

func pt(x, y float64) p2 { return p2{x * SS, y * SS} }

// A soft dark band along all four edges so pale covers don't swallow the
// leaves. Alpha falls off with a gentle curve to nothing at bandWidth.
func vignette(img *image.RGBA) {
	b := img.Bounds()
	bw := float64(bandWidth * SS)
	for y := 0; y < b.Max.Y; y++ {
		for x := 0; x < b.Max.X; x++ {
			d := math.Min(math.Min(float64(x), float64(b.Max.X-1-x)),
				math.Min(float64(y), float64(b.Max.Y-1-y)))
			if d >= bw {
				continue
			}
			t := 1 - d/bw
			a := uint8(bandAlpha * t * t)
			c := band
			c.A = a
			blend(img, x, y, c)
		}
	}
}

// A wavy stem from a to b with leaves alternating sides, a berry cluster
// every few nodes.
func vine(img *image.RGBA, a, b p2, amp float64, leaves int) {
	dx, dy := b.x-a.x, b.y-a.y
	length := math.Hypot(dx, dy)
	ux, uy := dx/length, dy/length // along
	nx, ny := -uy, ux              // across
	amp *= SS
	waves := float64(leaves) / 2

	stemW := 5.0 * SS
	prev := a
	steps := int(length / 2)
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		off := amp * math.Sin(t*waves*2*math.Pi)
		q := p2{a.x + dx*t + nx*off, a.y + dy*t + ny*off}
		segment(img, prev, q, stemW, ink)
		prev = q
	}

	for i := 0; i < leaves; i++ {
		t := (float64(i) + 0.5) / float64(leaves)
		off := amp * math.Sin(t*waves*2*math.Pi)
		base := p2{a.x + dx*t + nx*off, a.y + dy*t + ny*off}
		side := 1.0
		if i%2 == 1 {
			side = -1
		}
		// Leaves lean forward along the stem, sticking out to alternate
		// sides. A little deterministic jitter in angle and size keeps the
		// run from reading as a string of fairy lights.
		j := jitter(i, a)
		ang := math.Atan2(uy, ux) + side*(0.8+0.5*j)
		leafLen := (32 + 14*jitter(i+7, a)) * SS
		leafW := leafLen * (0.42 + 0.08*j)
		col := leaf
		if i%3 == 2 {
			col = light
		}
		// A short stalk, then the leaf.
		stalk := 7.0 * SS
		tip := p2{base.x + math.Cos(ang)*stalk, base.y + math.Sin(ang)*stalk}
		segment(img, base, tip, 2.4*SS, ink)
		drawLeaf(img, tip, ang, leafLen, leafW, col)
		if i%5 == 3 {
			berries(img, base, -side, math.Atan2(uy, ux))
		}
	}
}

// A repeatable pseudo-random value in [0,1) — no rand, so the output is
// identical on every run.
func jitter(i int, seed p2) float64 {
	v := math.Sin(float64(i)*12.9898+seed.x*0.013+seed.y*0.007) * 43758.5453
	return v - math.Floor(v)
}

// Three overlapping berries on the opposite side to the nearest leaf.
func berries(img *image.RGBA, base p2, side, ang float64) {
	nx, ny := -math.Sin(ang), math.Cos(ang)
	r := 6.0 * SS
	for i, o := range []p2{{0, 9}, {-8, 17}, {8, 18}} {
		c := p2{base.x + nx*side*o.y*SS + math.Cos(ang)*o.x*SS,
			base.y + ny*side*o.y*SS + math.Sin(ang)*o.x*SS}
		col := berry
		if i == 1 {
			col.R, col.G, col.B = 196, 84, 92
		}
		disc(img, c, r, col)
	}
	// A short stalk joining them to the stem.
	segment(img, base, p2{base.x + nx*side*9*SS, base.y + ny*side*9*SS},
		2.5*SS, ink)
}

// A spiral of ~1.6 turns, opening in one of four directions.
func curl(img *image.RGBA, c p2, r float64, quadrant int) {
	r *= SS
	start := []float64{math.Pi, 0, math.Pi, 0}[quadrant]
	dir := []float64{1, -1, -1, 1}[quadrant]
	prev := p2{}
	n := 140
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		ang := start + dir*t*1.6*2*math.Pi
		rad := r * (1 - 0.85*t)
		q := p2{c.x + rad*math.Cos(ang), c.y + rad*math.Sin(ang)}
		if i > 0 {
			segment(img, prev, q, (4.5-2.5*t)*SS, ink)
		}
		prev = q
	}
	disc(img, prev, 4*SS, ink)
}

// A pointed leaf: base at p, tip at p + len along ang, half-width following
// a sine so it swells in the middle and comes to a point both ends; a midrib.
func drawLeaf(img *image.RGBA, base p2, ang, length, width float64, col color.RGBA) {
	cx, sx := math.Cos(ang), math.Sin(ang)
	// bounding box
	x0 := int(math.Min(base.x, base.x+cx*length) - width - 2)
	x1 := int(math.Max(base.x, base.x+cx*length) + width + 2)
	y0 := int(math.Min(base.y, base.y+sx*length) - width - 2)
	y1 := int(math.Max(base.y, base.y+sx*length) + width + 2)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			// local coords: u along the leaf, v across
			dx, dy := float64(x)-base.x, float64(y)-base.y
			u := dx*cx + dy*sx
			v := -dx*sx + dy*cx
			if u < 0 || u > length {
				continue
			}
			hw := width * math.Pow(math.Sin(math.Pi*u/length), 0.8)
			if math.Abs(v) <= hw {
				blend(img, x, y, col)
			}
		}
	}
	// midrib, stopping short of the tip
	segment(img, base, p2{base.x + cx*length*0.85, base.y + sx*length*0.85},
		1.6*SS, ink)
}

// --- rasterising -------------------------------------------------------------

func disc(img *image.RGBA, c p2, r float64, col color.RGBA) {
	for y := int(c.y - r - 1); y <= int(c.y+r+1); y++ {
		for x := int(c.x - r - 1); x <= int(c.x+r+1); x++ {
			if math.Hypot(float64(x)-c.x, float64(y)-c.y) <= r {
				blend(img, x, y, col)
			}
		}
	}
}

// A thick line, stamped as a run of discs — crude but fine at 2x.
func segment(img *image.RGBA, a, b p2, w float64, col color.RGBA) {
	n := int(math.Hypot(b.x-a.x, b.y-a.y)/(w/3)) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		// Stamping discs would multiply alpha where they overlap, so write
		// them with Src semantics: a stroke has one flat alpha.
		stamp(img, p2{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t}, w/2, col)
	}
}

func stamp(img *image.RGBA, c p2, r float64, col color.RGBA) {
	for y := int(c.y - r - 1); y <= int(c.y+r+1); y++ {
		for x := int(c.x - r - 1); x <= int(c.x+r+1); x++ {
			if math.Hypot(float64(x)-c.x, float64(y)-c.y) <= r {
				set(img, x, y, col)
			}
		}
	}
}

// Writes col over whatever is there ("over" compositing). Used for leaves
// and the vignette, which are single passes and never self-overlap.
func blend(img *image.RGBA, x, y int, col color.RGBA) {
	if !(image.Point{x, y}).In(img.Bounds()) {
		return
	}
	draw.Draw(img, image.Rect(x, y, x+1, y+1), &image.Uniform{col},
		image.Point{}, draw.Over)
}

// Writes col outright (Src), so overlapping stamps of the same stroke stay
// one flat colour instead of darkening, and a stroke always lands on top of
// whatever is under it — a midrib over a leaf, a stem over the vignette.
func set(img *image.RGBA, x, y int, col color.RGBA) {
	if !(image.Point{x, y}).In(img.Bounds()) {
		return
	}
	draw.Draw(img, image.Rect(x, y, x+1, y+1), &image.Uniform{col},
		image.Point{}, draw.Src)
}
