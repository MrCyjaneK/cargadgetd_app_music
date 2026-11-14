package main

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/cargadgetd/music/internal/player"
)

const nowW = 340

func (c *card) buildSheet() fyne.CanvasObject {
	c.sheetTitle = canvas.NewText("", colFG)
	c.sheetTitle.TextStyle = fyne.TextStyle{Bold: true}
	c.sheetTitle.TextSize = 26
	c.sheetArtist = canvas.NewText("", colMuted)
	c.sheetArtist.TextSize = 20
	c.sheetAlbum = canvas.NewText("", colFG)
	c.sheetAlbum.TextSize = 18
	c.sheetWho = canvas.NewText("", colMuted)
	c.sheetWho.TextSize = 16
	c.sheetTime = canvas.NewText("", colFG)
	c.sheetTime.TextSize = 18
	c.sheetStatus = canvas.NewText("", colMuted)
	c.sheetStatus.TextSize = 16
	c.sheetBG = canvas.NewRectangle(colCover)
	c.sheetBG.SetMinSize(fyne.NewSquareSize(140))
	c.sheetArt = canvas.NewImageFromImage(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	c.sheetArt.FillMode = canvas.ImageFillStretch
	c.sheetArt.SetMinSize(fyne.NewSquareSize(140))
	c.sheetArt.Hide()
	cover := container.NewStack(c.sheetBG, c.sheetArt)
	prev := mediaButton(theme.MediaSkipPreviousIcon(), widget.LowImportance, func() { c.control("prev") })
	c.sheetPause = mediaButton(theme.MediaPauseIcon(), widget.HighImportance, func() { c.control("playpause") })
	next := mediaButton(theme.MediaSkipNextIcon(), widget.LowImportance, func() { c.control("next") })
	transport := container.NewGridWithColumns(3, prev, c.sheetPause, next)
	body := container.NewVBox(
		container.NewCenter(cover),
		c.sheetTitle,
		c.sheetArtist,
		c.sheetAlbum,
		c.sheetWho,
		c.sheetTime,
		c.sheetStatus,
		transport,
	)
	nowScroll := container.NewVScroll(container.NewPadded(body))
	c.now = nowScroll

	c.list = widget.NewList(
		func() int {
			c.mu.Lock()
			n := len(c.queue)
			c.mu.Unlock()
			return n
		},
		func() fyne.CanvasObject { return newQueueRow(c.gotoTrack) },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			row, _ := obj.(*queueRow)
			if row == nil {
				return
			}
			tr, ok := c.trackAt(int(id))
			if !ok {
				return
			}
			row.set(tr, tr.ID != "" && tr.ID == c.currentID())
		},
	)
	c.listPane = newQueueScroll(c.list)
	split := container.New(columnSplit{}, c.listPane, c.now)
	bg := canvas.NewRectangle(colPlate)
	panel := container.NewStack(bg, split)
	dim := canvas.NewRectangle(colDim)
	return container.New(sheetLayout{}, dim, newDismiss(c.closeSheet), panel)
}

type sheetLayout struct{}

func (s sheetLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(nowW, cardH)
}

func (s sheetLayout) Layout(objs []fyne.CanvasObject, sz fyne.Size) {
	if len(objs) < 3 {
		return
	}
	for _, o := range objs[:2] {
		if o == nil {
			continue
		}
		o.Move(fyne.NewPos(0, 0))
		o.Resize(sz)
	}
	panel := objs[2]
	if panel == nil {
		return
	}
	w := sz.Width * 3 / 4
	h := sz.Height - 32
	if w < 1 {
		w = sz.Width
	}
	if h < 1 || h > sz.Height {
		h = sz.Height
	}
	panel.Resize(fyne.NewSize(w, h))
	panel.Move(fyne.NewPos((sz.Width-w)/2, (sz.Height-h)/2))
}

// columnSplit keeps the queue on the left and the now-playing column on the right.
type columnSplit struct{}

func (s columnSplit) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(nowW, cardH)
}

func (s columnSplit) Layout(objs []fyne.CanvasObject, sz fyne.Size) {
	if len(objs) < 2 {
		return
	}
	list, now := objs[0], objs[1]
	want := float32(nowW)
	const minList = 120
	if sz.Width-want < minList {
		want = sz.Width - minList
	}
	if want < 1 {
		want = sz.Width / 2
	}
	if want > sz.Width {
		want = sz.Width
	}
	listW := sz.Width - want
	if list != nil {
		list.Move(fyne.NewPos(0, 0))
		list.Resize(fyne.NewSize(listW, sz.Height))
		list.Show()
	}
	if now != nil {
		now.Move(fyne.NewPos(listW, 0))
		now.Resize(fyne.NewSize(want, sz.Height))
	}
}

type queueRow struct {
	widget.BaseWidget
	title  *canvas.Text
	artist *canvas.Text
	bg     *canvas.Rectangle
	id     string
	onTap  func(string)
}

func newQueueRow(onTap func(string)) *queueRow {
	r := &queueRow{
		title:  canvas.NewText(" ", colFG),
		artist: canvas.NewText(" ", colMuted),
		bg:     canvas.NewRectangle(color.NRGBA{}),
		onTap:  onTap,
	}
	r.title.TextStyle = fyne.TextStyle{Bold: true}
	r.title.TextSize = 18
	r.artist.TextSize = 14
	r.ExtendBaseWidget(r)
	return r
}

func (r *queueRow) Tapped(*fyne.PointEvent) {
	if r.onTap != nil && r.id != "" {
		r.onTap(r.id)
	}
}

func (r *queueRow) set(tr player.QueueTrack, current bool) {
	r.id = tr.ID
	title := tr.Title
	if title == "" {
		title = " "
	}
	artist := tr.Artist
	if artist == "" {
		artist = " "
	}
	r.title.Text = title
	r.artist.Text = artist
	if current {
		r.bg.FillColor = colMark
	} else {
		r.bg.FillColor = color.NRGBA{}
	}
	r.title.Refresh()
	r.artist.Refresh()
	r.bg.Refresh()
}

func (r *queueRow) CreateRenderer() fyne.WidgetRenderer {
	text := container.NewVBox(r.title, r.artist)
	return widget.NewSimpleRenderer(container.NewStack(r.bg, container.NewPadded(text)))
}

// queueScroll swallows a drag and moves the list, because the software
// canvas does not report a mobile device and Fyne would otherwise ignore it.
type queueScroll struct {
	widget.BaseWidget
	list *widget.List
}

func newQueueScroll(list *widget.List) *queueScroll {
	q := &queueScroll{list: list}
	q.ExtendBaseWidget(q)
	return q
}

func (q *queueScroll) Dragged(e *fyne.DragEvent) {
	if q.list == nil {
		return
	}
	q.list.ScrollToOffset(q.list.GetScrollOffset() - e.Dragged.DY)
}

func (q *queueScroll) DragEnd() {}

func (q *queueScroll) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(q.list)
}

type tapPlate struct {
	widget.BaseWidget
	title, artist *canvas.Text
	onTap         func()
}

func newTapPlate(title, artist *canvas.Text, fn func()) *tapPlate {
	t := &tapPlate{title: title, artist: artist, onTap: fn}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapPlate) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapPlate) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewVBox(t.title, t.artist))
}

type tapFill struct {
	widget.BaseWidget
	onTap func()
}

func newTapFill(fn func()) *tapFill {
	t := &tapFill{onTap: fn}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapFill) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapFill) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.NRGBA{}))
}

// dismissLayer takes taps and drags that miss the panel.
type dismissLayer struct {
	widget.BaseWidget
	onTap func()
}

func newDismiss(fn func()) *dismissLayer {
	d := &dismissLayer{onTap: fn}
	d.ExtendBaseWidget(d)
	return d
}

func (d *dismissLayer) Tapped(*fyne.PointEvent) {
	if d.onTap != nil {
		d.onTap()
	}
}

func (d *dismissLayer) Dragged(*fyne.DragEvent) {}

func (d *dismissLayer) DragEnd() {}

func (d *dismissLayer) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.NRGBA{}))
}

// dragSink swallows a drag that starts on the corner card. A tap still
// reaches the buttons and the title inside it.
type dragSink struct {
	widget.BaseWidget
	body fyne.CanvasObject
}

func newDragSink(body fyne.CanvasObject) *dragSink {
	d := &dragSink{body: body}
	d.ExtendBaseWidget(d)
	return d
}

func (d *dragSink) MinSize() fyne.Size {
	if d.body == nil {
		return fyne.NewSize(0, 0)
	}
	return d.body.MinSize()
}

func (d *dragSink) Dragged(*fyne.DragEvent) {}

func (d *dragSink) DragEnd() {}

func (d *dragSink) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(d.body)
}
