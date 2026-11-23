package main

import (
	"bytes"
	"image"
	"image/png"
)

const (
	coverMax    = 256
	coverPNGMax = 48 << 10
)

type nowTrack struct {
	Title, Artist, Album, App string
	State, Duration, Position int
	PNG                       []byte
}

func (c *card) nowPlaying() (nowTrack, bool) {
	tr, _ := c.read()
	if !tr.Live() {
		return nowTrack{}, false
	}
	state := 1
	switch tr.Status {
	case "Playing":
		state = 2
	case "Paused":
		state = 3
	}
	return nowTrack{
		Title:    tr.Title,
		Artist:   tr.Artist,
		Album:    tr.Album,
		App:      "Music",
		State:    state,
		Duration: usToSec(tr.LengthUs),
		Position: usToSec(tr.PositionUs),
		PNG:      c.cachedCover(tr.Art),
	}, true
}

func usToSec(us int64) int {
	if us <= 0 {
		return 0
	}
	return int(us / 1_000_000)
}

func (c *card) cachedCover(src image.Image) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if src == nil {
		c.coverSrc = nil
		c.coverPNG = nil
		return nil
	}
	if src == c.coverSrc && c.coverPNG != nil {
		return c.coverPNG
	}
	pngBytes := encodeCover(src)
	c.coverSrc = src
	c.coverPNG = pngBytes
	return pngBytes
}

func encodeCover(src image.Image) []byte {
	if src == nil {
		return nil
	}
	src = fitCover(src, coverMax)
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil
	}
	if buf.Len() > coverPNGMax {
		return nil
	}
	return buf.Bytes()
}

func fitCover(src image.Image, max int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 || (w <= max && h <= max) {
		return src
	}
	nw, nh := w, h
	if w >= h {
		nw = max
		nh = h * max / w
	} else {
		nh = max
		nw = w * max / h
	}
	if nw < 1 {
		nw = 1
	}
	if nh < 1 {
		nh = 1
	}
	dst := image.NewNRGBA(image.Rect(0, 0, nw, nh))
	for y := 0; y < nh; y++ {
		sy := b.Min.Y + y*h/nh
		for x := 0; x < nw; x++ {
			sx := b.Min.X + x*w/nw
			dst.Set(x, y, src.At(sx, sy))
		}
	}
	return dst
}
