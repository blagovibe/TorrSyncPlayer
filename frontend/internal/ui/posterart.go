// SPDX-License-Identifier: MIT
// Copyright (c) 2025-2026 TorrSyncPlayer contributors
// See LICENSE file for full license text

package ui

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The shelf is the window's own answer to a hard truth about this library:
// torrents arrive as raw file names, with no key art attached. Netflix has
// posters; this player has names — so each poster is drawn: a seeded
// two-hue gradient, the title set in display caps (the title is the poster),
// a whisper of a film-strip emblem, the delivery state, and — when the film
// is playing — an amber ring, the one light this house reserves.
//
// Drawn once per (name, quantised progress, seeding, playing) and cached;
// the rail rebuilds on the two-second poll and the art only changes with
// the numbers on it.

// posterArt is one rendered poster at double density (2x of the tile).
const (
	posterW      = 296
	posterH      = 444
	posterRadius = 26
)

// posterPairs are the house hues: deep, desaturated, cinematic — never two
// accents in one tile; the title and the amber line are the only warmth.
var posterPairs = [][2]color.NRGBA{
	{{0x3b, 0x2f, 0x2a, 0xff}, {0x1f, 0x18, 0x15, 0xff}}, // cedar & shadow
	{{0x17, 0x33, 0x3a, 0xff}, {0x0e, 0x1f, 0x24, 0xff}}, // deep teal
	{{0x33, 0x20, 0x2e, 0xff}, {0x1d, 0x12, 0x19, 0xff}}, // wine cellar
	{{0x1e, 0x33, 0x26, 0xff}, {0x12, 0x1f, 0x17, 0xff}}, // spruce
	{{0x23, 0x2a, 0x38, 0xff}, {0x15, 0x1a, 0x22, 0xff}}, // steel slate
	{{0x24, 0x20, 0x40, 0xff}, {0x14, 0x11, 0x25, 0xff}}, // midnight indigo
	{{0x3a, 0x24, 0x1a, 0xff}, {0x20, 0x13, 0x10, 0xff}}, // ember
	{{0x22, 0x33, 0x27, 0xff}, {0x14, 0x1f, 0x18, 0xff}}, // forest floor
}

var (
	posterExtraBoldOnce = sync.OnceValues(func() (*opentype.Font, error) {
		return opentype.Parse(interExtraBold)
	})
	posterRegularOnce = sync.OnceValues(func() (*opentype.Font, error) {
		return opentype.Parse(interRegular)
	})
	posterCache = struct {
		sync.Mutex
		m map[string]*image.NRGBA
	}{m: map[string]*image.NRGBA{}}
)

// posterFor picks the pair deterministically: same name, same art, forever.
func posterFor(name string) (top, bottom color.NRGBA) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	pair := posterPairs[h.Sum32()%uint32(len(posterPairs))]
	return pair[0], pair[1]
}

func posterFace(size float64, weight *opentype.Font) (font.Face, error) {
	return opentype.NewFace(weight, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

func extraBold() (*opentype.Font, error) {
	return posterExtraBoldOnce()
}

func regular() (*opentype.Font, error) {
	return posterRegularOnce()
}

// posterArtFor renders (or fetches) the poster for one torrent.
func posterArtFor(tr func(string) string, t Torrent, playing bool) (*image.NRGBA, error) {
	key := fmt.Sprintf("%s|%d|%s|%s|%v", t.Name, int(t.Progress*100), t.Status, tr("poster.seeding"), playing)

	posterCache.Lock()
	defer posterCache.Unlock()
	if img, ok := posterCache.m[key]; ok {
		return img, nil
	}
	if len(posterCache.m) > 64 {
		posterCache.m = map[string]*image.NRGBA{}
	}

	img, err := drawPoster(tr, t, playing)
	if err != nil {
		return nil, err
	}
	posterCache.m[key] = img
	return img, nil
}

func drawPoster(tr func(string) string, t Torrent, playing bool) (*image.NRGBA, error) {
	bold, err := extraBold()
	if err != nil {
		return nil, fmt.Errorf("poster font: %w", err)
	}
	top, bottom := posterFor(t.Name)

	img := image.NewNRGBA(image.Rect(0, 0, posterW, posterH))
	mask := roundedMask(posterW, posterH, posterRadius)

	// The art itself: a vertical fall from the lighter hue into the dark,
	// then a vignette so the bottom third holds the type.
	for y := 0; y < posterH; y++ {
		f := float64(y) / float64(posterH)
		f = f * f * (3 - 2*f) // smoothstep: soft shoulder, deep floor
		row := lerpNRGBA(top, bottom, f)
		for x := 0; x < posterW; x++ {
			img.SetNRGBA(x, y, row)
		}
	}
	applyVignette(img)
	drawEmblem(img)
	drawTitle(img, bold, t.Name)
	drawState(tr, img, t, playing)

	// Rounded corners.
	out := image.NewNRGBA(img.Bounds())
	draw.DrawMask(out, img.Bounds(), img, image.Point{}, mask, image.Point{}, draw.Over)
	return out, nil
}

func lerpNRGBA(a, b color.NRGBA, f float64) color.NRGBA {
	mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: 0xff}
}

func applyVignette(img *image.NRGBA) {
	start := posterH * 2 / 5
	for y := start; y < posterH; y++ {
		f := float64(y-start) / float64(posterH-start)
		f = f * f
		alpha := uint8(150 * f)
		for x := 0; x < posterW; x++ {
			c := img.NRGBAAt(x, y)
			c.R = uint8(uint16(c.R) * (0xff - uint16(alpha)) / 0xff)
			c.G = uint8(uint16(c.G) * (0xff - uint16(alpha)) / 0xff)
			c.B = uint8(uint16(c.B) * (0xff - uint16(alpha)) / 0xff)
			img.SetNRGBA(x, y, c)
		}
	}
}

// roundedMask builds the anti-aliased rounded-corner mask by supersampling
// each pixel against the rounded-rect equation — no vector dependency, and
// edges at this radius are indistinguishable from a drawn path.
func roundedMask(w, h, r int) *image.Alpha {
	mask := image.NewAlpha(image.Rect(0, 0, w, h))
	inside := func(x, y float64) bool {
		switch {
		case x < float64(r) && y < float64(r):
			return dist2(x, y, float64(r), float64(r)) <= float64(r*r)
		case x >= float64(w-r) && y < float64(r):
			return dist2(x, y, float64(w-r), float64(r)) <= float64(r*r)
		case x < float64(r) && y >= float64(h-r):
			return dist2(x, y, float64(r), float64(h-r)) <= float64(r*r)
		case x >= float64(w-r) && y >= float64(h-r):
			return dist2(x, y, float64(w-r), float64(h-r)) <= float64(r*r)
		}
		return true
	}
	type offset struct{ dx, dy float64 }
	offsets := []offset{{0.25, 0.25}, {0.75, 0.25}, {0.25, 0.75}, {0.75, 0.75}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			n := 0
			for _, o := range offsets {
				if inside(float64(x)+o.dx, float64(y)+o.dy) {
					n++
				}
			}
			mask.SetAlpha(x, y, color.Alpha{uint8(n * 255 / 4)})
		}
	}
	return mask
}

func dist2(x, y, cx, cy float64) float64 {
	dx, dy := x-cx, y-cy
	return dx*dx + dy*dy
}

// drawEmblem strikes a film strip in the top-left corner, low contrast —
// a watermark of the trade, not a logo in the way.
func drawEmblem(img *image.NRGBA) {
	ink := color.NRGBA{R: 0xf4, G: 0xf2, B: 0xec, A: 0x59} // ~35%
	x, y, w, h := 18, 18, 46, 32
	// The strip outline, then two rows of punched sprockets.
	strokeRect(img, x, y, w, h, 3, ink, true)
	for _, sx := range []int{8, 19, 30} {
		fillRect(img, x+sx, y+7, x+sx+7, y+12, ink)
		fillRect(img, x+sx, y+20, x+sx+7, y+25, ink)
	}
}

func strokeRect(img *image.NRGBA, x, y, w, h, t int, c color.NRGBA, rounded bool) {
	_ = rounded
	for ty := 0; ty < t; ty++ {
		for xx := x; xx < x+w; xx++ {
			img.SetNRGBA(xx, y+ty, c)
			img.SetNRGBA(xx, y+h-1-ty, c)
		}
	}
	for tx := 0; tx < t; tx++ {
		for yy := y; yy < y+h; yy++ {
			img.SetNRGBA(x+tx, yy, c)
			img.SetNRGBA(x+w-1-tx, yy, c)
		}
	}
}

func fillRect(img *image.NRGBA, x0, y0, x1, y1 int, c color.NRGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
}

// drawTitle lays the name in display caps across up to four lines, sitting
// low, wrapped on word boundaries. The title IS the poster.
func drawTitle(img *image.NRGBA, bold *opentype.Font, name string) {
	face, err := posterFace(30, bold)
	if err != nil {
		return
	}
	defer face.Close()
	text := strings.ToUpper(name)
	text = truncateRunes(text, 96)
	lines := wrapToWidth(face, text, posterW-36)
	if len(lines) > 4 {
		lines = lines[:4]
		lines[3] = truncateRunes(lines[3], 34) + "…"
	}
	ink := color.NRGBA{R: 0xf4, G: 0xf2, B: 0xec, A: 0xff}
	gl := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(ink),
		Face: face,
	}
	y := posterH - 58 - (len(lines)-1)*38
	for _, line := range lines {
		gl.Dot = fixed.P(18, y)
		gl.DrawString(line)
		y += 38
	}
}

func wrapToWidth(face font.Face, text string, maxWidth int) []string {
	words := strings.Fields(text)
	var lines []string
	var line string
	for _, word := range words {
		candidate := strings.TrimSpace(line + " " + word)
		if measureString(face, candidate) <= maxWidth || line == "" {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = word
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func measureString(face font.Face, s string) int {
	d := &font.Drawer{Face: face}
	return d.MeasureString(s).Ceil()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// drawState prints the delivery truth in the bottom-left, small and dim —
// never more ink than the film deserves — plus the amber line that says
// how much of it is on this disk, and the ring when it's the one playing.
func drawState(tr func(string) string, img *image.NRGBA, t Torrent, playing bool) {
	var state string
	switch t.Status {
	case "seeding":
		state = tr("poster.seeding")
	case "loading":
		state = tr("poster.loading")
	default:
		state = fmt.Sprintf(tr("poster.downloading"), int(t.Progress*100))
	}
	regularFont, err := regular()
	if err != nil {
		return
	}
	dim := color.NRGBA{R: 0xa9, G: 0xa5, B: 0x9b, A: 0xff}
	face, faceErr := posterFace(15, regularFont)
	if faceErr == nil {
		defer face.Close()
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(dim),
			Face: face,
			Dot:  fixed.P(18, posterH-14),
		}
		d.DrawString(state)
	}

	if t.Status != "seeding" {
		// The amber hairline: how much of the film is here.
		x0, y0, x1, h := 18, posterH-30, posterW-18, 4
		fillRect(img, x0, y0, x1, y0+h, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x1f})
		fill := x0 + int(float64(x1-x0)*t.Progress)
		fillRect(img, x0, y0, fill, y0+h, color.NRGBA{R: 0xe8, G: 0xa3, B: 0x3d, A: 0xff})
	}

	if playing {
		// The ring: this is the film on the screen.
		strokeRing(img, 10, 10, posterW-10, posterH-10, 6, color.NRGBA{R: 0xe8, G: 0xa3, B: 0x3d, A: 0xff})
	}
}

func strokeRing(img *image.NRGBA, x0, y0, x1, y1, t int, c color.NRGBA) {
	for i := 0; i < t; i++ {
		for x := x0; x < x1; x++ {
			img.SetNRGBA(x, y0+i, c)
			img.SetNRGBA(x, y1-1-i, c)
		}
		for y := y0; y < y1; y++ {
			img.SetNRGBA(x0+i, y, c)
			img.SetNRGBA(x1-1-i, y, c)
		}
	}
}
