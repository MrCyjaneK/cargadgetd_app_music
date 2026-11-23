package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/cargadgetd/music/internal/player"
	"github.com/mrcyjanek/cargadgetd_sdk/cfg"
)

type recSource struct {
	cmds  []string
	gotos []string
	tr    player.Track
	q     []player.QueueTrack
}

func (r *recSource) Snapshot() player.Track {
	if r.tr.Title == "" && r.tr.Status == "" {
		return player.Track{Title: "Night", Artist: "fakemink", Status: "Playing"}
	}
	return r.tr
}

func (*recSource) Close() {}

func (r *recSource) Control(cmd string) { r.cmds = append(r.cmds, cmd) }

func (r *recSource) Queue() []player.QueueTrack { return r.q }

func (r *recSource) GoTo(id string) { r.gotos = append(r.gotos, id) }

func (*recSource) SyncQueue() {}

type idleSource struct{}

func (idleSource) Snapshot() player.Track     { return player.Track{} }
func (idleSource) Queue() []player.QueueTrack { return nil }
func (idleSource) Close()                     {}
func (idleSource) Control(string)             {}
func (idleSource) GoTo(string)                {}
func (idleSource) SyncQueue()                 {}

func TestCardSitsBottomRightAndStaysClearWhenIdle(t *testing.T) {
	c := newCard()
	c.mu.Lock()
	c.src = idleSource{}
	c.mu.Unlock()
	dst := image.NewRGBA(image.Rect(0, 0, 400, 300))
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("idle card painted")
	}
	c.mu.Lock()
	c.src = &recSource{}
	c.mu.Unlock()
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("card covered the top left")
	}
	if dst.RGBAAt(390, 290).A == 0 {
		t.Fatal("card missing from the bottom right")
	}
}

func TestTransportAndBackendSetting(t *testing.T) {
	dir := t.TempDir()
	cfg.SetOwnRootForTest(t, dir)
	if loadBackend() != "mpris" {
		t.Fatalf("default %s", loadBackend())
	}

	rec := &recSource{}
	prevOpen := openSource
	openSource = func(string) player.Source { return rec }
	t.Cleanup(func() { openSource = prevOpen })

	c := newCard()
	c.kind = "mpris"
	if err := c.setBackend("mock"); err != nil {
		t.Fatal(err)
	}
	c.control("playpause")
	c.control("next")
	c.control("prev")
	c.control("play")
	c.control("pause")
	c.control("stop")
	if strings.Join(rec.cmds, ",") != "playpause,next,prev,play,pause,stop" {
		t.Fatalf("cmds %v", rec.cmds)
	}
	bare := newCard()
	bare.kind = "mock"
	bare.control("next")
	if strings.Join(rec.cmds, ",") != "playpause,next,prev,play,pause,stop,next" {
		t.Fatalf("key before paint: %v", rec.cmds)
	}
	plug, err := os.ReadFile(filepath.Join(dir, "music.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(plug), "backend: mock") {
		t.Fatalf("plugin:\n%s", plug)
	}
	if loadBackend() != "mock" {
		t.Fatalf("backend %s", loadBackend())
	}
}

func TestCoverSitsInTheCorner(t *testing.T) {
	art := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(art.Pix); i += 4 {
		art.Pix[i] = 220
		art.Pix[i+1] = 30
		art.Pix[i+2] = 40
		art.Pix[i+3] = 255
	}
	c := newCard()
	c.mu.Lock()
	c.src = &recSource{tr: player.Track{
		Title: "Night", Artist: "fakemink", Status: "Playing", Art: art,
	}}
	c.mu.Unlock()
	dst := image.NewRGBA(image.Rect(0, 0, 400, 300))
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("cover painted the top left")
	}
	pix := dst.RGBAAt(210, 250)
	if pix.R < 180 || pix.B > 80 {
		t.Fatalf("cover pixel %+v", pix)
	}
}

func TestSheetOpensFromTheBodyAndClosesOutside(t *testing.T) {
	rec := &recSource{q: []player.QueueTrack{
		{ID: "/mock/0", Title: "Ghost Signal", Artist: "Mira Vale"},
		{ID: "/mock/1", Title: "Harbor Lights", Artist: "Mira Vale"},
	}}
	c := newCard()
	c.mu.Lock()
	c.src = rec
	c.mu.Unlock()
	dst := image.NewRGBA(image.Rect(0, 0, 800, 480))
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("sheet open before a tap")
	}
	tapObj(t, c, c.pauseBtn)
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("transport opened the sheet")
	}
	if strings.Join(rec.cmds, ",") != "playpause" {
		t.Fatalf("cmds %v", rec.cmds)
	}
	tapObj(t, c, c.names)
	c.draw(dst)
	if dst.RGBAAt(10, 10).A == 0 {
		t.Fatal("body tap left the corner clear")
	}
	if findButton(c.kit.Canvas().Content(), "Tracks") != nil {
		t.Fatal("tracks button still on the sheet")
	}
	nowAt, ok := absPos(c.kit.Canvas().Content(), c.now, fyne.NewPos(0, 0))
	if !ok {
		t.Fatal("now-playing column missing")
	}
	if c.listPane == nil || !c.listPane.Visible() || c.listPane.Size().Width < 40 {
		t.Fatalf("list %v", c.listPane)
	}
	listAt, ok := absPos(c.kit.Canvas().Content(), c.listPane, fyne.NewPos(0, 0))
	if !ok || nowAt.X < listAt.X+40 {
		t.Fatalf("list %v now %v", listAt, nowAt)
	}
	sheetW := c.listPane.Size().Width + c.now.Size().Width
	if sheetW < 590 || sheetW > 610 || listAt.X < 90 || listAt.X > 110 {
		t.Fatalf("sheet width %v at %v", sheetW, listAt)
	}
	tapAt := func(x, y int) {
		c.touch(x, y, 1)
		c.touch(x, y, 0)
	}
	x := int(listAt.X) + 24
	tapAt(x, int(listAt.Y)+12)
	if len(rec.gotos) != 1 || rec.gotos[0] != "/mock/0" {
		t.Fatalf("first goto %v", rec.gotos)
	}
	rec.tr = player.Track{ID: "/mock/0", Title: "Ghost Signal", Artist: "Mira Vale", Status: "Playing"}
	c.draw(dst)
	second := false
	for dy := 28; dy < 220; dy += 8 {
		tapAt(x, int(listAt.Y)+dy)
		if len(rec.gotos) > 0 && rec.gotos[len(rec.gotos)-1] == "/mock/1" {
			second = true
			break
		}
	}
	if !second {
		t.Fatalf("second goto %v", rec.gotos)
	}
	c.touch(8, 8, 1)
	c.touch(8, 8, 0)
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("tap outside left the sheet up")
	}
}

func TestDragOnTheCardDoesNotOpen(t *testing.T) {
	c := newCard()
	c.mu.Lock()
	c.src = &recSource{}
	c.mu.Unlock()
	dst := image.NewRGBA(image.Rect(0, 0, 400, 300))
	c.draw(dst)
	pos, ok := absPos(c.kit.Canvas().Content(), c.names, fyne.NewPos(0, 0))
	if !ok {
		t.Fatal("title missing")
	}
	x, y := int(pos.X+4), int(pos.Y+4)
	c.touch(x, y, 1)
	c.touch(x+40, y, 2)
	c.touch(x+40, y, 0)
	c.draw(dst)
	if dst.RGBAAt(10, 10).A != 0 {
		t.Fatal("drag opened the sheet")
	}
}

func tapObj(t *testing.T, c *card, obj fyne.CanvasObject) {
	t.Helper()
	if obj == nil {
		t.Fatal("nil tap target")
	}
	pos, ok := absPos(c.kit.Canvas().Content(), obj, fyne.NewPos(0, 0))
	if !ok {
		t.Fatalf("object not in canvas: %T", obj)
	}
	sz := obj.Size()
	if sz.Width < 1 || sz.Height < 1 {
		t.Fatalf("zero size %v at %v", sz, pos)
	}
	x := int(pos.X + sz.Width/2)
	y := int(pos.Y + sz.Height/2)
	c.touch(x, y, 1)
	c.touch(x, y, 0)
}

func absPos(o fyne.CanvasObject, target fyne.CanvasObject, acc fyne.Position) (fyne.Position, bool) {
	if o == nil {
		return fyne.Position{}, false
	}
	p := acc.Add(o.Position())
	if o == target {
		return p, true
	}
	if c, ok := o.(*fyne.Container); ok {
		for _, ch := range c.Objects {
			if pos, found := absPos(ch, target, p); found {
				return pos, true
			}
		}
	}
	if d, ok := o.(*dragSink); ok {
		if pos, found := absPos(d.body, target, p); found {
			return pos, true
		}
	}
	if q, ok := o.(*queueScroll); ok {
		if pos, found := absPos(q.list, target, p); found {
			return pos, true
		}
	}
	if sc, ok := o.(*container.Scroll); ok {
		if pos, found := absPos(sc.Content, target, p); found {
			return pos, true
		}
	}
	return fyne.Position{}, false
}

func TestNowPlayingSnapshot(t *testing.T) {
	c := newCard()
	c.mu.Lock()
	c.src = idleSource{}
	c.mu.Unlock()
	if _, ok := c.nowPlaying(); ok {
		t.Fatal("idle snapshot is not a track")
	}

	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	c.mu.Lock()
	c.src = &recSource{tr: player.Track{
		Title: "Night", Artist: "fakemink", Album: "LP", Status: "Playing",
		LengthUs: 180_000_000, PositionUs: 12_000_000, Art: img,
	}}
	c.mu.Unlock()
	got, ok := c.nowPlaying()
	if !ok {
		t.Fatal("playing snapshot")
	}
	if got.Title != "Night" || got.Artist != "fakemink" || got.Album != "LP" || got.App != "Music" {
		t.Fatalf("%+v", got)
	}
	if got.State != 2 || got.Duration != 180 || got.Position != 12 {
		t.Fatalf("state/time %+v", got)
	}
	if !bytes.HasPrefix(got.PNG, []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("png %x", got.PNG)
	}

	wide := image.NewNRGBA(image.Rect(0, 0, 400, 100))
	decoded, err := png.Decode(bytes.NewReader(encodeCover(wide)))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() > coverMax || decoded.Bounds().Dy() > coverMax {
		t.Fatalf("cover %v", decoded.Bounds())
	}
}

func findButton(root fyne.CanvasObject, text string) *widget.Button {
	var found *widget.Button
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if o == nil || found != nil {
			return
		}
		if b, ok := o.(*widget.Button); ok && b.Text == text {
			found = b
			return
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, ch := range c.Objects {
				walk(ch)
			}
		}
		if d, ok := o.(*dragSink); ok {
			walk(d.body)
		}
		if sc, ok := o.(*container.Scroll); ok {
			walk(sc.Content)
		}
	}
	walk(root)
	return found
}
