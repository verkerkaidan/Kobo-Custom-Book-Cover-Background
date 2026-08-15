// Generates assets/frame-template.png: a guide to draw a frame on top of.
//
// It is a guide, not a frame — it marks the panel size, a suggested window for
// the cover, and the strip along the bottom where the stats get drawn. Draw on
// a layer above it, delete this layer, erase the window to transparent, export.
//
//	go run ./tools/mktemplate assets/frame-template.png
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// Kobo Clara Colour. Other models differ; see config.ini for the list.
const (
	W, H   = 1072, 1448
	margin = 70
)

// Suggested window: full width less a comfortable border, stopping exactly
// where the stats block begins.
var window = image.Rect(120, 180, 952, 1130)

// Where render() draws the title, percentage, bar and counts.
var textArea = image.Rect(0, 1130, W, 1378)

func main() {
	out := "assets/frame-template.png"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, img.Bounds(), color.RGBA{250, 249, 246, 255})

	// The stats strip, shaded so it is obvious what will be covered.
	fill(img, textArea, color.RGBA{226, 232, 240, 255})

	// The suggested window, checkerboarded the way art tools show transparency.
	for y := window.Min.Y; y < window.Max.Y; y++ {
		for x := window.Min.X; x < window.Max.X; x++ {
			if ((x/24)+(y/24))%2 == 0 {
				img.SetRGBA(x, y, color.RGBA{214, 214, 214, 255})
			} else {
				img.SetRGBA(x, y, color.RGBA{235, 235, 235, 255})
			}
		}
	}
	dashed(img, window, color.RGBA{40, 90, 200, 255}, 3)
	dashed(img, image.Rect(margin, margin, W-margin, H-margin),
		color.RGBA{170, 170, 170, 255}, 1)

	face := mustFace(26)
	label(img, face, window.Min.X+16, window.Min.Y+40,
		"COVER GOES HERE — erase this area to transparent",
		color.RGBA{40, 90, 200, 255})
	label(img, face, margin, textArea.Min.Y+40,
		"stats are drawn over this strip — keep detail out",
		color.RGBA{70, 90, 120, 255})
	label(img, mustFace(22), margin, margin+30,
		fmt.Sprintf("%dx%d — draw your pattern here, then delete this guide layer", W, H),
		color.RGBA{150, 150, 150, 255})

	f, err := os.Create(out)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		panic(err)
	}
	fmt.Println("wrote", out)
}

func fill(img *image.RGBA, r image.Rectangle, c color.RGBA) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func dashed(img *image.RGBA, r image.Rectangle, c color.RGBA, weight int) {
	on := func(i int) bool { return (i/12)%2 == 0 }
	for x := r.Min.X; x < r.Max.X; x++ {
		if !on(x) {
			continue
		}
		for t := 0; t < weight; t++ {
			img.SetRGBA(x, r.Min.Y+t, c)
			img.SetRGBA(x, r.Max.Y-1-t, c)
		}
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		if !on(y) {
			continue
		}
		for t := 0; t < weight; t++ {
			img.SetRGBA(r.Min.X+t, y, c)
			img.SetRGBA(r.Max.X-1-t, y, c)
		}
	}
}

func mustFace(size float64) font.Face {
	f, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		panic(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		panic(err)
	}
	return face
}

func label(img *image.RGBA, face font.Face, x, y int, s string, c color.RGBA) {
	d := &font.Drawer{Dst: img, Src: &image.Uniform{c}, Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}
