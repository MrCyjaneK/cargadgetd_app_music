package main

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/cargadgetd/music/internal/player"
	"github.com/mrcyjanek/cargadgetd_sdk/fynekit"
)

const cardH = 110

var (
	colFG    = color.NRGBA{R: 232, G: 236, B: 240, A: 255}
	colMuted = color.NRGBA{R: 154, G: 163, B: 175, A: 255}
	colPlate = color.NRGBA{R: 16, G: 20, B: 24, A: 230}
	colDim   = color.NRGBA{R: 0, G: 0, B: 0, A: 160}
	colCover = color.NRGBA{R: 10, G: 12, B: 16, A: 255}
	colMark  = color.NRGBA{R: 40, G: 56, B: 48, A: 255}
)

// openSource is the music backend factory. Tests replace it.
var openSource = player.Open

// card is the drive music plate: half the buffer wide, bottom-right, and
// clear everywhere else so a tap misses it and falls through. A tap on the
// cover or the title opens a full-frame sheet.
type card struct {
	mu   sync.Mutex
	kit  *fynekit.Kit
	kind string
	src  player.Source

	root     fyne.CanvasObject
	title    *canvas.Text
	artist   *canvas.Text
	names    *tapPlate
	coverBG  *canvas.Rectangle
	coverArt *canvas.Image
	pauseBtn *widget.Button
	art      image.Image
	status   string
	shownID  string
	queue    []player.QueueTrack
	queueSig string
	sheet    fyne.CanvasObject
	now      fyne.CanvasObject
	list     *widget.List
	listPane fyne.CanvasObject
	sheetOn  bool

	sheetTitle  *canvas.Text
	sheetArtist *canvas.Text
	sheetAlbum  *canvas.Text
	sheetWho    *canvas.Text
	sheetTime   *canvas.Text
	sheetStatus *canvas.Text
	sheetArt    *canvas.Image
	sheetBG     *canvas.Rectangle
	sheetPause  *widget.Button
}

func newCard() *card {
	return &card{
		kit:  fynekit.NewTransparent(),
		kind: player.Normalize(loadBackend()),
	}
}

func (c *card) build() fyne.CanvasObject {
	c.title = canvas.NewText("", colFG)
	c.title.TextStyle = fyne.TextStyle{Bold: true}
	c.title.TextSize = 20
	c.artist = canvas.NewText("", colMuted)
	c.artist.TextSize = 16
	prev := mediaButton(theme.MediaSkipPreviousIcon(), widget.LowImportance, func() { c.control("prev") })
	c.pauseBtn = mediaButton(theme.MediaPauseIcon(), widget.HighImportance, func() { c.control("playpause") })
	next := mediaButton(theme.MediaSkipNextIcon(), widget.LowImportance, func() { c.control("next") })
	transport := container.NewGridWithColumns(3, prev, c.pauseBtn, next)
	c.names = newTapPlate(c.title, c.artist, c.openSheet)
	c.coverBG = canvas.NewRectangle(colCover)
	c.coverBG.SetMinSize(fyne.NewSquareSize(cardH))
	c.coverArt = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	c.coverArt.FillMode = canvas.ImageFillStretch
	c.coverArt.SetMinSize(fyne.NewSquareSize(cardH))
	c.coverArt.Hide()
	cover := container.NewStack(c.coverBG, c.coverArt, newTapFill(c.openSheet))
	body := container.New(&cardBox{}, cover, c.names, transport)
	plate := canvas.NewRectangle(color.NRGBA{R: 16, G: 20, B: 24, A: 168})
	c.root = newDragSink(container.NewStack(plate, body))
	c.root.Hide()
	c.sheet = c.buildSheet()
	c.sheet.Hide()
	return container.New(&stage{}, c.root, c.sheet)
}

func mediaButton(icon fyne.Resource, imp widget.Importance, fn func()) *widget.Button {
	b := widget.NewButtonWithIcon("", icon, fn)
	b.Importance = imp
	return b
}

type stage struct{}

func (stage) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 || objs[0] == nil {
		return fyne.NewSize(cardH, cardH)
	}
	return objs[0].MinSize()
}

func (stage) Layout(objs []fyne.CanvasObject, sz fyne.Size) {
	if len(objs) == 0 {
		return
	}
	if objs[0] != nil {
		cornerCard{}.Layout(objs[:1], sz)
	}
	if len(objs) > 1 && objs[1] != nil {
		objs[1].Move(fyne.NewPos(0, 0))
		objs[1].Resize(sz)
	}
}

type cornerCard struct{}

func (cornerCard) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 || objs[0] == nil {
		return fyne.NewSize(cardH, cardH)
	}
	return objs[0].MinSize()
}

func (cornerCard) Layout(objs []fyne.CanvasObject, sz fyne.Size) {
	if len(objs) == 0 || objs[0] == nil {
		return
	}
	w := sz.Width / 2
	if w < cardH {
		w = sz.Width
	}
	h := float32(cardH)
	if h > sz.Height {
		h = sz.Height
	}
	objs[0].Resize(fyne.NewSize(w, h))
	objs[0].Move(fyne.NewPos(sz.Width-w, sz.Height-h))
}

// cardBox pins the cover to the left edge and stacks the title over the
// transport in the remaining column, scaled to the cover height.
type cardBox struct{}

func (cardBox) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(cardH*2, cardH)
}

func (cardBox) Layout(objs []fyne.CanvasObject, sz fyne.Size) {
	if len(objs) < 3 {
		return
	}
	h := sz.Height
	if h < 1 {
		h = cardH
	}
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSquareSize(h))
	gap := float32(4)
	x := h + gap
	w := sz.Width - x
	if w < 1 {
		w = 1
	}
	rows := objs[1:]
	sum := float32(0)
	heights := make([]float32, len(rows))
	for i, o := range rows {
		if o == nil {
			continue
		}
		heights[i] = o.MinSize().Height
		sum += heights[i]
	}
	if sum > h && sum > 0 {
		scale := h / sum
		for i := range heights {
			heights[i] *= scale
		}
		sum = h
	}
	pad := (h - sum) / float32(len(rows)+1)
	if pad < 0 {
		pad = 0
	}
	y := pad
	for i, o := range rows {
		if o == nil {
			continue
		}
		o.Move(fyne.NewPos(x, y))
		o.Resize(fyne.NewSize(w, heights[i]))
		y += heights[i] + pad
	}
}

func (c *card) draw(dst *image.RGBA) {
	if dst == nil {
		return
	}
	c.kit.Ensure(c.build)
	if cv := c.kit.Canvas(); cv != nil {
		cv.SetPadded(false)
	}
	tr, q := c.read()
	c.apply(tr, q)
	c.kit.Draw(dst)
}

func (c *card) control(cmd string) {
	c.mu.Lock()
	src := c.src
	if src == nil {
		src = openSource(c.kind)
		c.src = src
	}
	c.mu.Unlock()
	if src != nil {
		src.Control(cmd)
	}
}

func (c *card) gotoTrack(id string) {
	c.mu.Lock()
	src := c.src
	c.mu.Unlock()
	if src != nil && id != "" {
		src.GoTo(id)
	}
}

func (c *card) syncQueue() {
	c.mu.Lock()
	src := c.src
	c.mu.Unlock()
	if src != nil {
		src.SyncQueue()
	}
}

func (c *card) read() (player.Track, []player.QueueTrack) {
	c.mu.Lock()
	if c.src == nil {
		c.src = openSource(c.kind)
	}
	src := c.src
	c.mu.Unlock()
	if src == nil {
		return player.Track{}, nil
	}
	return src.Snapshot(), src.Queue()
}

func (c *card) apply(tr player.Track, q []player.QueueTrack) {
	if c.root == nil {
		return
	}
	changed := false
	if !tr.Live() {
		if c.root.Visible() {
			c.root.Hide()
			changed = true
		}
		if c.sheetOn {
			c.closeSheet()
			changed = true
		}
	} else {
		if !c.root.Visible() {
			c.root.Show()
			changed = true
		}
		changed = c.setText(c.title, tr.Title) || changed
		changed = c.setText(c.artist, tr.Artist) || changed
		if c.status != tr.Status {
			c.status = tr.Status
			setPlayIcon(c.pauseBtn, tr.Status)
			setPlayIcon(c.sheetPause, tr.Status)
			changed = true
		}
		if c.art != tr.Art {
			c.art = tr.Art
			applyArt(c.coverArt, c.coverBG, tr.Art)
			applyArt(c.sheetArt, c.sheetBG, tr.Art)
			changed = true
		}
		changed = c.applySheet(tr) || changed
	}
	if c.noteQueue(q) {
		changed = true
	}
	c.mu.Lock()
	prevID := c.shownID
	idChanged := prevID != tr.ID
	if idChanged {
		c.shownID = tr.ID
	}
	qNow := append([]player.QueueTrack(nil), c.queue...)
	c.mu.Unlock()
	if idChanged && c.list != nil {
		for i, row := range qNow {
			if row.ID == prevID || row.ID == tr.ID {
				c.list.RefreshItem(i)
			}
		}
		changed = true
	}
	if changed {
		c.kit.MarkDirty()
	}
}

func (c *card) setText(tx *canvas.Text, s string) bool {
	if tx == nil || tx.Text == s {
		return false
	}
	tx.Text = s
	tx.Refresh()
	return true
}

func (c *card) applySheet(tr player.Track) bool {
	changed := false
	changed = c.setText(c.sheetTitle, tr.Title) || changed
	changed = c.setText(c.sheetArtist, tr.Artist) || changed
	changed = c.setLine(c.sheetAlbum, tr.Album) || changed
	who := ""
	if tr.AlbumArtist != "" && tr.AlbumArtist != tr.Artist {
		who = tr.AlbumArtist
	}
	changed = c.setLine(c.sheetWho, who) || changed
	changed = c.setLine(c.sheetTime, timeLine(tr)) || changed
	changed = c.setLine(c.sheetStatus, tr.Status) || changed
	return changed
}

func (c *card) setLine(tx *canvas.Text, s string) bool {
	if tx == nil {
		return false
	}
	s = strings.TrimSpace(s)
	show := s != ""
	changed := tx.Text != s || tx.Visible() != show
	tx.Text = s
	if show {
		tx.Show()
	} else {
		tx.Hide()
	}
	tx.Refresh()
	return changed
}

func (c *card) noteQueue(q []player.QueueTrack) bool {
	sig := queueSig(q)
	c.mu.Lock()
	same := sig == c.queueSig
	if !same {
		c.queue = append([]player.QueueTrack(nil), q...)
		c.queueSig = sig
	}
	c.mu.Unlock()
	if same || c.list == nil {
		return false
	}
	c.list.Refresh()
	return true
}

func queueSig(q []player.QueueTrack) string {
	var b strings.Builder
	for _, t := range q {
		b.WriteString(t.ID)
		b.WriteByte(0)
		b.WriteString(t.Title)
		b.WriteByte(0)
		b.WriteString(t.Artist)
		b.WriteByte(0)
	}
	return b.String()
}

func (c *card) trackAt(i int) (player.QueueTrack, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if i < 0 || i >= len(c.queue) {
		return player.QueueTrack{}, false
	}
	return c.queue[i], true
}

func (c *card) currentID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.shownID
}

func applyArt(img *canvas.Image, bg *canvas.Rectangle, art image.Image) {
	if img == nil {
		return
	}
	if art == nil {
		img.Hide()
		if bg != nil {
			bg.Show()
		}
		return
	}
	img.Image = art
	img.Refresh()
	img.Show()
	if bg != nil {
		bg.Hide()
	}
}

func setPlayIcon(b *widget.Button, status string) {
	if b == nil {
		return
	}
	if status == "Playing" {
		b.SetIcon(theme.MediaPauseIcon())
	} else {
		b.SetIcon(theme.MediaPlayIcon())
	}
}

func timeLine(tr player.Track) string {
	if tr.LengthUs <= 0 {
		if tr.PositionUs <= 0 {
			return ""
		}
		return clockUs(tr.PositionUs)
	}
	return clockUs(tr.PositionUs) + " / " + clockUs(tr.LengthUs)
}

func clockUs(us int64) string {
	if us < 0 {
		us = 0
	}
	sec := us / 1_000_000
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func (c *card) openSheet() {
	if c.sheet == nil || c.sheetOn {
		return
	}
	c.sheetOn = true
	c.sheet.Show()
	c.syncQueue()
	c.kit.MarkDirty()
}

func (c *card) closeSheet() {
	if c.sheet == nil || !c.sheetOn {
		return
	}
	c.sheetOn = false
	c.sheet.Hide()
	c.kit.MarkDirty()
}

func (c *card) touch(x, y, action int) {
	c.kit.Ensure(c.build)
	c.kit.Touch(x, y, action)
}

func (c *card) pause() { c.closeSrc() }

func (c *card) resume() {
	kind := player.Normalize(loadBackend())
	c.mu.Lock()
	opened := c.src != nil
	c.mu.Unlock()
	if !opened {
		c.mu.Lock()
		c.kind = kind
		c.mu.Unlock()
		return
	}
	_ = c.setBackend(kind)
}

func (c *card) shutdown() { c.closeSrc() }

func (c *card) closeSrc() {
	c.mu.Lock()
	src := c.src
	c.src = nil
	c.mu.Unlock()
	if src != nil {
		src.Close()
	}
}

func (c *card) setBackend(kind string) error {
	kind = player.Normalize(kind)
	c.mu.Lock()
	same := c.kind == kind && c.src != nil
	c.mu.Unlock()
	if same {
		return nil
	}
	if err := patchBackend(kind); err != nil {
		return err
	}
	next := openSource(kind)
	c.mu.Lock()
	old := c.src
	c.src = next
	c.kind = kind
	c.mu.Unlock()
	if old != nil {
		old.Close()
	}
	return nil
}
