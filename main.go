// kobo-dynamic-screensaver
//
// Reads Nickel's KoboReader.sqlite, renders reading stats onto a background
// image, and writes the result into the sleep-screen folder Nickel displays.
//
// Single static binary. No interpreter, no libraries, nothing to install.
package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go: cross-compiles without a C toolchain
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	_ "image/jpeg"
)

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

type Config map[string]string

var defaults = Config{
	"db_path":             "/mnt/onboard/.kobo/KoboReader.sqlite",
	"cover_background":    "true",
	"kobo_images":         "/mnt/onboard/.kobo-images",
	"frame":               "",
	"cover_window":        "auto",
	"cover_fit":           "cover",
	"mat_colour":          "18,18,22",
	"text_offset":         "0",
	"background":          "/mnt/onboard/.adds/kobo-screensaver/assets/background.png",
	"output":              "/mnt/onboard/.kobo/screensaver/dynamic.png",
	"width":               "0",
	"height":              "0",
	"font":                "/usr/local/Kobo/fonts/Georgia.ttf",
	"font_bold":           "/usr/local/Kobo/fonts/Georgia-Bold.ttf",
	"title_size":          "44",
	"body_size":           "34",
	"small_size":          "26",
	"margin":              "70",
	"position":            "bottom",
	"text_colour":         "255,255,255",
	"shadow":              "true",
	"scrim":               "true",
	"grayscale":           "false",
	"show_title":          "true",
	"show_progress_bar":   "true",
	"show_finished_count": "true",
	"show_library_count":  "true",
	"show_time_spent":     "false",
	"show_time_left":      "false",
	"show_date":           "true",
	"date_format":         "02 Jan 2006  15:04",
	"poll_interval":       "15",
	"min_regen":           "45",
	"skip_unchanged":      "true",
}

// Minimal INI reader: one flat section, key = value, # or ; comments.
func loadConfig(path string) Config {
	cfg := Config{}
	for k, v := range defaults {
		cfg[k] = v
	}
	f, err := os.Open(path)
	if err != nil {
		return cfg
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, ";") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cfg[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return cfg
}

func (c Config) str(k string) string { return c[k] }

func (c Config) int(k string) int {
	n, err := strconv.Atoi(strings.TrimSpace(c[k]))
	if err != nil {
		n, _ = strconv.Atoi(defaults[k])
	}
	return n
}

func (c Config) bool(k string) bool {
	switch strings.ToLower(strings.TrimSpace(c[k])) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func (c Config) colour(k string) color.RGBA {
	parts := strings.Split(c[k], ",")
	if len(parts) != 3 {
		return color.RGBA{255, 255, 255, 255}
	}
	var v [3]uint8
	for i, p := range parts {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		if n < 0 {
			n = 0
		} else if n > 255 {
			n = 255
		}
		v[i] = uint8(n)
	}
	return color.RGBA{v[0], v[1], v[2], 255}
}

// ---------------------------------------------------------------------------
// screen geometry
// ---------------------------------------------------------------------------

func detectScreenSize(cfg Config) (int, int) {
	if w, h := cfg.int("width"), cfg.int("height"); w > 0 && h > 0 {
		return w, h
	}
	if b, err := os.ReadFile("/sys/class/graphics/fb0/virtual_size"); err == nil {
		if ws, hs, ok := strings.Cut(strings.TrimSpace(string(b)), ","); ok {
			w, e1 := strconv.Atoi(ws)
			h, e2 := strconv.Atoi(hs)
			if e1 == nil && e2 == nil && w > 0 && h > 0 {
				return w, h
			}
		}
	}
	return 1072, 1448 // Clara Colour
}

// ---------------------------------------------------------------------------
// database
// ---------------------------------------------------------------------------

type Stats struct {
	Title, Author     string
	ImageID           string
	Percent           int
	SecondsRead       int
	Finished, Library int
	InProgress        int
}

// What counts as "a book".
//
//   - MimeType LIKE '%epub%' is a whitelist, not a blacklist. Books bought from
//     Kobo are 'application/x-kobo-epub+zip', so the obvious-looking
//     `NOT LIKE '%kobo%'` excludes almost the entire library. It also keeps out
//     the PNGs that KFMon trigger icons and the background image get indexed
//     as, which otherwise count as books and — once tapped — as *finished*
//     ones at 100%.
//   - The .kobo/ path exclusion drops the device's own eLabel leaflet, which is
//     a genuine epub and would otherwise be counted and, worse, shown as the
//     book you are currently reading.
const bookFilter = `
	ContentType = 6
	AND MimeType LIKE '%epub%'
	AND (IsDownloaded = 'true' OR IsDownloaded = 1)
	AND ___UserID IS NOT NULL
	AND ___UserID != ''
	AND ContentID NOT LIKE 'file:///mnt/onboard/.kobo/%'
`

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// Nickel holds a lock on the live database, so work from a snapshot. The -wal
// and -shm sidecars come along so that recent, uncheckpointed writes are seen.
func readStats(dbPath string) (Stats, error) {
	var s Stats

	tmp, err := os.MkdirTemp("", "kss-")
	if err != nil {
		return s, err
	}
	defer os.RemoveAll(tmp)

	snap := filepath.Join(tmp, "KoboReader.sqlite")
	if err := copyFile(dbPath, snap); err != nil {
		return s, err
	}
	for _, ext := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + ext); err == nil {
			_ = copyFile(dbPath+ext, snap+ext)
		}
	}

	db, err := sql.Open("sqlite", "file:"+snap+"?_pragma=query_only(1)")
	if err != nil {
		return s, err
	}
	defer db.Close()

	row := db.QueryRow(`
		SELECT Title, Attribution, ___PercentRead, ImageId,
		       COALESCE(TimeSpentReading, 0)
		FROM content
		WHERE ` + bookFilter + `
		  AND ReadStatus = 1
		  AND DateLastRead IS NOT NULL
		ORDER BY DateLastRead DESC LIMIT 1`)

	var title, author, imageID sql.NullString
	var pct, secsRead sql.NullInt64
	switch err := row.Scan(&title, &author, &pct, &imageID, &secsRead); err {
	case nil:
		s.Title, s.Author, s.Percent = title.String, author.String, int(pct.Int64)
		s.ImageID, s.SecondsRead = imageID.String, int(secsRead.Int64)
	case sql.ErrNoRows:
		// nothing in progress; the counts below still render
	default:
		return s, err
	}

	count := func(extra string) int {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM content WHERE ` + bookFilter + extra).Scan(&n); err != nil {
			return 0
		}
		return n
	}
	s.Finished = count(" AND ReadStatus = 2")
	s.Library = count("")
	s.InProgress = count(" AND ReadStatus = 1")

	return s, nil
}

// ---------------------------------------------------------------------------
// cover art
// ---------------------------------------------------------------------------

// Nickel caches every book's cover under .kobo-images, in a two-level directory
// derived from a hash of the ImageId, as baseline JPEGs with a .parsed suffix.
// Rather than reimplement Kobo's hash (undocumented, and it has changed before)
// we walk the tree looking for the filename. A library of a few thousand books
// is a few thousand direntries — a handful of milliseconds, and this runs at
// most once every min_regen seconds.
//
// Suffixes, largest first. N3_FULL is full-screen artwork — on a Clara Colour
// it is 964x1448 against a 1072x1448 panel. The others are library thumbnails
// and are only worth using if N3_FULL is absent.
var coverSuffixes = []string{
	" - N3_FULL.parsed",
	" - N3_LIBRARY_FULL.parsed",
	" - N3_LIBRARY_GRID.parsed",
}

func findCover(imagesDir, imageID string) string {
	if imagesDir == "" || imageID == "" {
		return ""
	}
	found := make(map[string]string, len(coverSuffixes))

	_ = filepath.WalkDir(imagesDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable subtree just means no cover
		}
		name := d.Name()
		if !strings.HasPrefix(name, imageID) {
			return nil
		}
		for _, suf := range coverSuffixes {
			if name == imageID+suf {
				found[suf] = path
			}
		}
		return nil
	})

	for _, suf := range coverSuffixes {
		if p, ok := found[suf]; ok {
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// fonts
// ---------------------------------------------------------------------------

var fontFallbacks = []string{
	"/usr/local/Kobo/fonts/Georgia.ttf",
	"/usr/local/Kobo/fonts/Avenir.ttf",
	"/usr/local/Kobo/fonts/Caecilia.ttf",
	"/mnt/onboard/.adds/kobo-screensaver/assets/font.ttf",
	"/mnt/onboard/fonts/font.ttf",
}

func loadFace(path string, size float64) (font.Face, error) {
	candidates := append([]string{path}, fontFallbacks...)
	var lastErr error
	for _, p := range candidates {
		if p == "" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			lastErr = err
			continue
		}
		f, err := sfnt.Parse(b) // handles both TTF and OTF outlines
		if err != nil {
			lastErr = err
			continue
		}
		face, err := opentype.NewFace(f, &opentype.FaceOptions{
			Size: size, DPI: 72, Hinting: font.HintingFull,
		})
		if err != nil {
			lastErr = err
			continue
		}
		return face, nil
	}
	return nil, fmt.Errorf("no usable font found (last error: %v)", lastErr)
}

// ---------------------------------------------------------------------------
// rendering
// ---------------------------------------------------------------------------

// Scales the image at path into rectangle r of dst.
//
//	"cover"   — fill r completely, cropping whatever overflows (the default:
//	            inside a hand-drawn window you want no gaps)
//	"contain" — fit entirely inside r, leaving whatever dst already holds
//	            visible around it
//
// Reports whether the file could be read and decoded.
func drawImageInto(dst *image.RGBA, r image.Rectangle, path, mode string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return false
	}

	sb := src.Bounds()
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	rw, rh := float64(r.Dx()), float64(r.Dy())
	if sw <= 0 || sh <= 0 || rw <= 0 || rh <= 0 {
		return false
	}

	sx, sy := rw/sw, rh/sh
	scale := sx
	if mode == "contain" {
		if sy < scale {
			scale = sy
		}
	} else if sy > scale {
		scale = sy
	}
	nw, nh := int(sw*scale+0.5), int(sh*scale+0.5)

	scaled := image.NewRGBA(image.Rect(0, 0, nw, nh))
	xdraw.CatmullRom.Scale(scaled, scaled.Bounds(), src, sb, draw.Src, nil)

	if mode == "contain" {
		dr := image.Rect(0, 0, nw, nh).
			Add(image.Pt(r.Min.X+(r.Dx()-nw)/2, r.Min.Y+(r.Dy()-nh)/2))
		draw.Draw(dst, dr, scaled, image.Point{}, draw.Src)
		return true
	}
	// Cover: take the centre of the scaled image.
	draw.Draw(dst, r, scaled, image.Pt((nw-r.Dx())/2, (nh-r.Dy())/2), draw.Src)
	return true
}

func fitBackground(path string, w, h int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	if !drawImageInto(dst, dst.Bounds(), path, "cover") {
		draw.Draw(dst, dst.Bounds(), &image.Uniform{color.RGBA{30, 30, 34, 255}},
			image.Point{}, draw.Src)
	}
	return dst
}

// ---------------------------------------------------------------------------
// decorative frame
// ---------------------------------------------------------------------------

// Pixels this transparent count as "the window". Not zero, because drawing
// tools leave faint anti-aliased edges around an erased region.
const windowAlphaMax = 8

// The window is wherever the artwork is see-through: the bounding box of the
// transparent pixels. Deriving it from the art means she never has to measure
// anything or edit coordinates — she erases a hole and the cover appears there.
//
// Reports false when there is no transparency at all (a flattened export), or
// when the hole is too small to be deliberate.
func detectWindow(img *image.RGBA) (image.Rectangle, bool) {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if img.RGBAAt(x, y).A > windowAlphaMax {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if !found {
		return image.Rectangle{}, false
	}

	win := image.Rect(minX, minY, maxX+1, maxY+1)
	if win.Dx()*win.Dy()*100 < b.Dx()*b.Dy()*15 {
		return image.Rectangle{}, false // stray erased pixels, not a window
	}
	return win, true
}

// Decodes the frame artwork and works out where the cover goes.
//
// A frame drawn at exactly the panel size is used pixel-for-pixel, so hand-drawn
// lines stay crisp; anything else is scaled to fit inside the panel and centred,
// which never crops her work.
func loadFrame(path string, w, h int) (*image.RGBA, image.Rectangle, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, image.Rectangle{}, false, err
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return nil, image.Rectangle{}, false, err
	}

	canvas := image.NewRGBA(image.Rect(0, 0, w, h)) // transparent
	sb := src.Bounds()
	if sb.Dx() == w && sb.Dy() == h {
		draw.Draw(canvas, canvas.Bounds(), src, sb.Min, draw.Src)
	} else {
		sw, sh := float64(sb.Dx()), float64(sb.Dy())
		scale := float64(w) / sw
		if s := float64(h) / sh; s < scale {
			scale = s
		}
		nw, nh := int(sw*scale+0.5), int(sh*scale+0.5)
		dr := image.Rect(0, 0, nw, nh).Add(image.Pt((w-nw)/2, (h-nh)/2))
		xdraw.CatmullRom.Scale(canvas, dr, src, sb, draw.Src, nil)
	}

	win, ok := detectWindow(canvas)
	return canvas, win, ok, nil
}

// "auto", or an explicit "x,y,w,h" override.
func parseWindow(s string, w, h int) (image.Rectangle, bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, false
	}
	v := make([]int, 4)
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return image.Rectangle{}, false
		}
		v[i] = n
	}
	r := image.Rect(v[0], v[1], v[0]+v[2], v[1]+v[3]).
		Intersect(image.Rect(0, 0, w, h))
	if r.Empty() {
		return image.Rectangle{}, false
	}
	return r, true
}

// Soft dark gradient so light text stays readable over any photograph.
func drawScrim(img *image.RGBA, position string, bandH int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if bandH > h {
		bandH = h
	}
	for i := 0; i < bandH; i++ {
		t := float64(i) / float64(max(1, bandH-1))
		var alpha float64
		var y int
		if position == "bottom" {
			alpha = 200 * math.Pow(t, 1.6)
			y = h - bandH + i
		} else {
			alpha = 200 * math.Pow(1-t, 1.6)
			y = i
		}
		a := uint8(alpha)
		for x := 0; x < w; x++ {
			c := img.RGBAAt(x, y)
			c.R = uint8((int(c.R) * (255 - int(a))) / 255)
			c.G = uint8((int(c.G) * (255 - int(a))) / 255)
			c.B = uint8((int(c.B) * (255 - int(a))) / 255)
			img.SetRGBA(x, y, c)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func measure(face font.Face, s string) int {
	return font.MeasureString(face, s).Round()
}

func ellipsize(face font.Face, s string, maxW int) string {
	if measure(face, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && measure(face, string(r)+"…") > maxW {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "…"
}

func drawText(dst *image.RGBA, face font.Face, x, y int, s string, c color.RGBA, shadow bool) {
	d := &font.Drawer{Dst: dst, Face: face}
	if shadow {
		d.Src = &image.Uniform{color.RGBA{0, 0, 0, 255}}
		d.Dot = fixed.P(x+2, y+2)
		d.DrawString(s)
	}
	d.Src = &image.Uniform{c}
	d.Dot = fixed.P(x, y)
	d.DrawString(s)
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	draw.Draw(img, image.Rect(x0, y0, x1, y1), &image.Uniform{c}, image.Point{}, draw.Src)
}

func strokeRect(img *image.RGBA, x0, y0, x1, y1, w int, c color.RGBA) {
	fillRect(img, x0, y0, x1, y0+w, c)
	fillRect(img, x0, y1-w, x1, y1, c)
	fillRect(img, x0, y0, x0+w, y1, c)
	fillRect(img, x1-w, y0, x1, y1, c)
}

type line struct {
	kind string // title | body | small
	text string
}

// "3h 1m", "45m", "2h". Minute precision: this is a reading stat, not a clock.
func humanDuration(secs int) string {
	m := secs / 60
	h, m := m/60, m%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// Extrapolates time left from the pace so far. Deliberately crude — reading
// speed varies — hence the "~" the caller prefixes. Refuses to guess below 3%,
// where the sample is too small to mean anything.
func estimateRemaining(s Stats) (int, bool) {
	if s.SecondsRead <= 0 || s.Percent < 3 || s.Percent >= 100 {
		return 0, false
	}
	perPercent := float64(s.SecondsRead) / float64(s.Percent)
	return int(perPercent * float64(100-s.Percent)), true
}

// Builds the backdrop: either the plain full-bleed image as before, or, when a
// frame is configured, the cover set into the frame's window with her artwork
// composited over it.
func composeBackground(cfg Config, bgPath string, w, h int) *image.RGBA {
	framePath := cfg.str("frame")
	if framePath == "" {
		return fitBackground(bgPath, w, h)
	}

	frame, win, detected, err := loadFrame(framePath, w, h)
	if err != nil {
		// No file at all simply means she hasn't drawn one: the config ships
		// with the path filled in so that dropping a frame.dat in place just
		// works, and saying so on every render would be noise. A file that
		// exists but won't decode is worth complaining about.
		if !os.IsNotExist(err) {
			log.Printf("frame %s unusable (%v) — drawing without it",
				framePath, err)
		}
		return fitBackground(bgPath, w, h)
	}

	full := image.Rect(0, 0, w, h)
	switch cw := strings.ToLower(strings.TrimSpace(cfg.str("cover_window"))); {
	case cw != "" && cw != "auto":
		if r, ok := parseWindow(cw, w, h); ok {
			win = r
		} else {
			log.Printf("cover_window %q not understood, using auto", cw)
		}
	case !detected:
		// A flattened export with no transparency: the artwork would hide the
		// cover completely, so put the cover behind the full panel instead.
		log.Printf("frame %s has no transparent area — cover will be behind it",
			framePath)
		win = full
	}

	img := image.NewRGBA(full)
	draw.Draw(img, full, &image.Uniform{cfg.colour("mat_colour")},
		image.Point{}, draw.Src)
	drawImageInto(img, win, bgPath, strings.ToLower(cfg.str("cover_fit")))
	draw.Draw(img, full, frame, image.Point{}, draw.Over)
	return img
}

func render(s Stats, cfg Config, w, h int, bgPath string) (*image.RGBA, error) {
	margin := cfg.int("margin")
	col := cfg.colour("text_colour")
	shadow := cfg.bool("shadow")
	position := strings.ToLower(cfg.str("position"))

	fTitle, err := loadFace(cfg.str("font_bold"), float64(cfg.int("title_size")))
	if err != nil {
		return nil, err
	}
	fBody, err := loadFace(cfg.str("font"), float64(cfg.int("body_size")))
	if err != nil {
		return nil, err
	}
	fSmall, err := loadFace(cfg.str("font"), float64(cfg.int("small_size")))
	if err != nil {
		return nil, err
	}
	faces := map[string]font.Face{"title": fTitle, "body": fBody, "small": fSmall}
	sizes := map[string]int{
		"title": cfg.int("title_size"),
		"body":  cfg.int("body_size"),
		"small": cfg.int("small_size"),
	}
	gaps := map[string]int{"title": 14, "body": 16, "small": 10}

	// ---- assemble the lines ------------------------------------------------
	var lines []line
	if cfg.bool("show_title") && s.Title != "" {
		lines = append(lines, line{"title", s.Title})
		if s.Author != "" {
			lines = append(lines, line{"small", s.Author})
		}
	}
	if s.Title != "" {
		// Kobo does not store page numbers: pagination is recomputed on the fly
		// from the current font size and margins, so ___NumPages is -1 for every
		// row. Reading *time* is recorded, though, and serves the same purpose —
		// a sense of progress finer than a percentage.
		prog := fmt.Sprintf("%d%% read", s.Percent)
		if cfg.bool("show_time_spent") && s.SecondsRead > 0 {
			prog += "  ·  " + humanDuration(s.SecondsRead)
		}
		if cfg.bool("show_time_left") {
			if left, ok := estimateRemaining(s); ok {
				prog += "  ·  ~" + humanDuration(left) + " left"
			}
		}
		lines = append(lines, line{"body", prog})
	}

	var tail []string
	if cfg.bool("show_finished_count") {
		plural := "s"
		if s.Finished == 1 {
			plural = ""
		}
		tail = append(tail, fmt.Sprintf("%d book%s finished", s.Finished, plural))
	}
	if cfg.bool("show_library_count") {
		tail = append(tail, fmt.Sprintf("%d on device", s.Library))
	}
	if len(tail) > 0 {
		lines = append(lines, line{"small", strings.Join(tail, "  ·  ")})
	}
	if cfg.bool("show_date") {
		lines = append(lines, line{"small", time.Now().Format(cfg.str("date_format"))})
	}

	barH, barGap := 10, 22
	showBar := cfg.bool("show_progress_bar") && s.Title != ""

	blockH := 0
	for _, l := range lines {
		blockH += sizes[l.kind] + gaps[l.kind]
	}
	if showBar {
		blockH += barH + barGap
	}

	// ---- compose -----------------------------------------------------------
	// Mat colour, then the cover inside the window, then her artwork over the
	// top with alpha. Text and scrim come after, so they are never at the mercy
	// of what she drew.
	img := composeBackground(cfg, bgPath, w, h)
	if cfg.bool("scrim") {
		drawScrim(img, position, blockH+margin*2)
	}

	maxW := w - margin*2
	y := margin
	if position == "bottom" {
		y = h - margin - blockH
	}
	// Nudges the whole block down (or up, if negative). Exists so the text can
	// be moved clear of whatever a hand-drawn frame has near the edges —
	// rules and ornaments will not be in the same place on every drawing.
	y += cfg.int("text_offset")

	for _, l := range lines {
		face := faces[l.kind]
		y += sizes[l.kind] // font.Drawer draws from the baseline
		drawText(img, face, margin, y, ellipsize(face, l.text, maxW), col, shadow)
		y += gaps[l.kind]

		if l.kind == "body" && showBar {
			pct := s.Percent
			if pct < 0 {
				pct = 0
			} else if pct > 100 {
				pct = 100
			}
			x0, x1 := margin, margin+maxW
			strokeRect(img, x0, y, x1, y+barH, 2, col)
			if pct > 0 {
				fw := (maxW - 4) * pct / 100
				fillRect(img, x0+2, y+2, x0+2+fw, y+barH-2, col)
			}
			y += barH + barGap
		}
	}

	if cfg.bool("grayscale") {
		b := img.Bounds()
		for yy := b.Min.Y; yy < b.Max.Y; yy++ {
			for xx := b.Min.X; xx < b.Max.X; xx++ {
				c := img.RGBAAt(xx, yy)
				g := uint8((299*int(c.R) + 587*int(c.G) + 114*int(c.B)) / 1000)
				img.SetRGBA(xx, yy, color.RGBA{g, g, g, 255})
			}
		}
	}

	return img, nil
}

// ---------------------------------------------------------------------------

func generate(cfg Config) error {
	dbPath := cfg.str("db_path")
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("database not found: %s", dbPath)
	}

	w, h := detectScreenSize(cfg)
	stats, err := readStats(dbPath)
	if err != nil {
		return fmt.Errorf("reading database: %w", err)
	}

	// The current book's own cover, when asked for and available. Falls back to
	// the configured image whenever there is no book open, no cached cover, or
	// the feature is off — so the sleep screen never ends up blank.
	bgPath, bgKind := cfg.str("background"), "image"
	if cfg.bool("cover_background") {
		if p := findCover(cfg.str("kobo_images"), stats.ImageID); p != "" {
			bgPath, bgKind = p, "cover"
		}
	}

	// Rendering is the expensive part — decode the cover, scale it, rasterise
	// the text, re-encode a 1072x1448 PNG. About five seconds on this hardware.
	// If nothing that appears on screen has actually changed, skip it: that is
	// what makes a short poll interval affordable in battery terms.
	//
	// The clock is deliberately excluded from this comparison, so the time on
	// screen is when your reading last *changed*, not when it was last checked.
	sig := renderSignature(stats, cfg, bgPath)
	if cfg.bool("skip_unchanged") && sig == lastRenderSig {
		return nil
	}

	img, err := render(stats, cfg, w, h, bgPath)
	if err != nil {
		return fmt.Errorf("rendering: %w", err)
	}

	out := cfg.str("output")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	// Write to a sibling temp file and rename, so Nickel never sees a
	// half-written PNG if it wakes mid-write.
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	if err := os.Rename(tmp, out); err != nil {
		return err
	}

	lastRenderSig = sig
	log.Printf("wrote %s (%dx%d) [%s] %d%% · %d finished · %d on device",
		out, w, h, bgKind, stats.Percent, stats.Finished, stats.Library)
	return nil
}

// What the last written PNG depicted. Empty in one-shot mode, so a manual run
// always redraws.
var lastRenderSig string

// Everything that can appear on screen, and nothing that cannot.
//
// Fields behind a disabled toggle are excluded on purpose: TimeSpentReading
// ticks up continuously while you read, so including it when it isn't being
// drawn would trigger a redraw every time Nickel flushes, which is exactly the
// waste skip_unchanged exists to avoid. Times are compared at the minute
// granularity they are printed at, not in seconds, for the same reason.
//
// The clock is excluded, so it reflects the last real change.
func renderSignature(s Stats, cfg Config, bgPath string) string {
	var sb strings.Builder
	sb.WriteString(bgPath)
	// Contents, not just paths: she overwrites the same filename every time.
	sb.WriteString(fileSig(bgPath, cfg.str("frame")))
	sb.WriteString(cfg.str("cover_window") + cfg.str("cover_fit") +
		cfg.str("text_offset"))
	if s.Title != "" {
		if cfg.bool("show_title") {
			fmt.Fprintf(&sb, "|%s|%s", s.Title, s.Author)
		}
		fmt.Fprintf(&sb, "|%d", s.Percent)
		if cfg.bool("show_time_spent") {
			fmt.Fprintf(&sb, "|%s", humanDuration(s.SecondsRead))
		}
		if cfg.bool("show_time_left") {
			if left, ok := estimateRemaining(s); ok {
				fmt.Fprintf(&sb, "|%s", humanDuration(left))
			}
		}
	}
	if cfg.bool("show_finished_count") {
		fmt.Fprintf(&sb, "|f%d", s.Finished)
	}
	if cfg.bool("show_library_count") {
		fmt.Fprintf(&sb, "|l%d", s.Library)
	}
	return sb.String()
}

// A fingerprint of the database's on-disk state.
//
// The database runs in WAL mode, so Nickel's page-turn writes land in the -wal
// sidecar and the main file's mtime does not move until a checkpoint — which
// can be many minutes, or only at unmount. Watching the main file alone means
// sleeping through almost every reading update, so all three files count.
//
// Sizes are included as well as mtimes because the internal storage is FAT32,
// whose timestamps have two-second granularity; the -wal file grows steadily as
// you read, which catches changes the clock is too coarse to show.
func dbSignature(dbPath string) string {
	return fileSig(dbPath, dbPath+"-wal", dbPath+"-shm")
}

// mtime+size of each path, "-" when absent.
func fileSig(paths ...string) string {
	var sb strings.Builder
	for _, p := range paths {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(p); err == nil {
			fmt.Fprintf(&sb, "%d:%d|", fi.ModTime().UnixNano(), fi.Size())
		} else {
			sb.WriteString("-|")
		}
	}
	return sb.String()
}

// The artwork files. Replacing either has to trigger a redraw on its own —
// otherwise swapping a background or a frame appears to do nothing until the
// reading percentage happens to move.
func assetSignature(cfg Config) string {
	return fileSig(cfg.str("background"), cfg.str("frame"))
}

// Poll the database rather than run on a fixed timer: does essentially nothing
// while you are not reading.
func watch(cfg Config, cfgPath string) {
	dbPath := cfg.str("db_path")
	interval := time.Duration(cfg.int("poll_interval")) * time.Second
	minRegen := time.Duration(cfg.int("min_regen")) * time.Second

	if err := generate(cfg); err != nil {
		log.Printf("initial render failed: %v", err)
	}
	sigOf := func(c Config) string {
		return dbSignature(dbPath) + assetSignature(c) + fileSig(cfgPath)
	}
	lastSig := sigOf(cfg)
	lastRun := time.Now()

	for {
		time.Sleep(interval)

		// While the Kobo is plugged into a computer, Nickel unmounts the
		// partition and the database vanishes. Nothing to do but wait.
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		sig := sigOf(cfg)
		if sig == lastSig || time.Since(lastRun) < minRegen {
			continue
		}
		lastSig = sig
		lastRun = time.Now()

		// Pick up edits to config.ini without a restart, so tuning the layout
		// is eject-and-look rather than eject-reboot-tap. Only the polling
		// timings above stay fixed for the life of the process.
		cfg = loadConfig(cfgPath)

		if err := generate(cfg); err != nil {
			log.Printf("render failed: %v", err)
		}
	}
}

func main() {
	exe, _ := os.Executable()
	defaultCfg := filepath.Join(filepath.Dir(exe), "config.ini")

	cfgPath := flag.String("config", defaultCfg, "path to config.ini")
	doWatch := flag.Bool("watch", false, "stay resident and re-render as reading progresses")
	logPath := flag.String("log", "", "append output to this file instead of stderr")
	flag.Parse()

	if *logPath != "" {
		if f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
			defer f.Close()
			log.SetOutput(f)
		}
	}
	log.SetFlags(log.Ldate | log.Ltime)

	cfg := loadConfig(*cfgPath)

	if *doWatch {
		log.Printf("--- watching %s ---", cfg.str("db_path"))
		watch(cfg, *cfgPath)
		return
	}
	if err := generate(cfg); err != nil {
		log.Printf("error: %v", err)
		os.Exit(1)
	}
}
