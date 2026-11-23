package main

/*
#cgo CFLAGS: -I${SRCDIR}/include
#include "cargadgetd/cg_app.h"
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"image"
	"unsafe"
)

var (
	app       = newCard()
	musicName = C.CString("Music")
	iconC     *C.uint8_t
)

func ensureIconC() {
	if iconC != nil {
		return
	}
	loadIcon()
	if len(iconPix) == 0 {
		return
	}
	n := C.size_t(len(iconPix))
	iconC = (*C.uint8_t)(C.malloc(n))
	if iconC == nil {
		return
	}
	C.memcpy(unsafe.Pointer(iconC), unsafe.Pointer(&iconPix[0]), n)
}

//export cargadgetd_app_music_icon
func cargadgetd_app_music_icon(w, h *C.int) *C.uint8_t {
	ensureIconC()
	if w != nil {
		*w = C.int(iconW)
	}
	if h != nil {
		*h = C.int(iconH)
	}
	return iconC
}

//export cargadgetd_app_music_init
func cargadgetd_app_music_init(_ *C.struct_cargadgetd_host) {}

//export cargadgetd_app_music_draw
func cargadgetd_app_music_draw(rgba *C.uint32_t, w, h, stride C.int) {
	if rgba == nil || w < 1 || h < 1 || stride < w*4 {
		return
	}
	pix := unsafe.Slice((*uint8)(unsafe.Pointer(rgba)), int(h)*int(stride))
	dst := &image.RGBA{
		Pix:    pix,
		Stride: int(stride),
		Rect:   image.Rect(0, 0, int(w), int(h)),
	}
	app.draw(dst)
}

//export cargadgetd_app_music_touch
func cargadgetd_app_music_touch(x, y, action C.int) {
	app.touch(int(x), int(y), int(action))
}

//export cargadgetd_app_music_pause
func cargadgetd_app_music_pause() { app.pause() }

//export cargadgetd_app_music_resume
func cargadgetd_app_music_resume() { app.resume() }

//export cargadgetd_app_music_stop
func cargadgetd_app_music_stop() { app.shutdown() }

type nowHold struct {
	title, artist, album, app *C.char
	png                       []byte
}

var heldNow nowHold

func holdCString(dst **C.char, s string) *C.char {
	if *dst != nil {
		C.free(unsafe.Pointer(*dst))
		*dst = nil
	}
	if s == "" {
		return nil
	}
	*dst = C.CString(s)
	return *dst
}

//export cargadgetd_app_music_nowplaying
func cargadgetd_app_music_nowplaying(out *C.struct_cargadgetd_nowplaying) C.int {
	if out == nil {
		return 0
	}
	*out = C.struct_cargadgetd_nowplaying{}
	tr, ok := app.nowPlaying()
	if !ok {
		return 0
	}
	out.title = holdCString(&heldNow.title, tr.Title)
	out.artist = holdCString(&heldNow.artist, tr.Artist)
	out.album = holdCString(&heldNow.album, tr.Album)
	out.app = holdCString(&heldNow.app, tr.App)
	out.state = C.int(tr.State)
	out.duration_seconds = C.int(tr.Duration)
	out.position_seconds = C.int(tr.Position)
	heldNow.png = append(heldNow.png[:0], tr.PNG...)
	if len(heldNow.png) > 0 {
		out.art_png = (*C.uint8_t)(unsafe.Pointer(&heldNow.png[0]))
		out.art_len = C.int(len(heldNow.png))
	}
	return 1
}

//export cargadgetd_app_music_name
func cargadgetd_app_music_name() *C.char { return musicName }

//export cargadgetd_app_music_huinput_playpause
func cargadgetd_app_music_huinput_playpause(pressed C.int) {
	if pressed != 0 {
		app.control("playpause")
	}
}

//export cargadgetd_app_music_huinput_next
func cargadgetd_app_music_huinput_next(pressed C.int) {
	if pressed != 0 {
		app.control("next")
	}
}

//export cargadgetd_app_music_huinput_prev
func cargadgetd_app_music_huinput_prev(pressed C.int) {
	if pressed != 0 {
		app.control("prev")
	}
}

//export cargadgetd_app_music_huinput_play
func cargadgetd_app_music_huinput_play(pressed C.int) {
	if pressed != 0 {
		app.control("play")
	}
}

//export cargadgetd_app_music_huinput_pause
func cargadgetd_app_music_huinput_pause(pressed C.int) {
	if pressed != 0 {
		app.control("pause")
	}
}

//export cargadgetd_app_music_huinput_stop
func cargadgetd_app_music_huinput_stop(pressed C.int) {
	if pressed != 0 {
		app.control("stop")
	}
}

//export cargadgetd_app_music_settings_count
func cargadgetd_app_music_settings_count() C.int { return C.int(musicSettingsCount()) }

//export cargadgetd_app_music_settings_label
func cargadgetd_app_music_settings_label(i C.int) *C.char {
	return C.CString(musicSettingsLabel(int(i)))
}

//export cargadgetd_app_music_settings_type
func cargadgetd_app_music_settings_type(i C.int) C.int {
	return C.int(musicSettingsType(int(i)))
}

//export cargadgetd_app_music_settings_enum_values
func cargadgetd_app_music_settings_enum_values(i C.int) *C.char {
	return C.CString(musicSettingsEnums(int(i)))
}

//export cargadgetd_app_music_settings_get_bool
func cargadgetd_app_music_settings_get_bool(C.int) C.int { return 0 }

//export cargadgetd_app_music_settings_get_enum
func cargadgetd_app_music_settings_get_enum(i C.int) *C.char {
	return C.CString(musicSettingsGetEnum(int(i)))
}

//export cargadgetd_app_music_settings_get_int
func cargadgetd_app_music_settings_get_int(C.int) C.int { return 0 }

//export cargadgetd_app_music_settings_set_bool
func cargadgetd_app_music_settings_set_bool(C.int, C.int) C.int { return 1 }

//export cargadgetd_app_music_settings_set_enum
func cargadgetd_app_music_settings_set_enum(i C.int, v *C.char) C.int {
	return C.int(musicSettingsSetEnum(int(i), C.GoString(v)))
}

//export cargadgetd_app_music_settings_set_int
func cargadgetd_app_music_settings_set_int(C.int, C.int) C.int { return 1 }

//export cargadgetd_app_music_settings_error
func cargadgetd_app_music_settings_error() *C.char {
	musicSettingsMu.Lock()
	defer musicSettingsMu.Unlock()
	if musicSettingsErr == "" {
		return nil
	}
	return C.CString(musicSettingsErr)
}

func main() {}
