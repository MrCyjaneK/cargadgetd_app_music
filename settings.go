package main

import (
	"strings"
	"sync"

	"github.com/cargadgetd/music/internal/player"
)

const (
	musicSetBackend = iota
	musicSetCount
)

var (
	musicSettingsMu  sync.Mutex
	musicSettingsErr string
)

func musicSettingsCount() int { return musicSetCount }

func musicSettingsLabel(i int) string {
	if i == musicSetBackend {
		return "Music"
	}
	return ""
}

func musicSettingsType(i int) int {
	if i == musicSetBackend {
		return 1
	}
	return -1
}

func musicSettingsEnums(i int) string {
	if i != musicSetBackend {
		return ""
	}
	bs := player.Backends()
	parts := make([]string, len(bs))
	for i, b := range bs {
		parts[i] = b.Label
	}
	return strings.Join(parts, ",")
}

func musicSettingsGetEnum(i int) string {
	if i != musicSetBackend {
		return ""
	}
	app.mu.Lock()
	kind := app.kind
	app.mu.Unlock()
	return player.Label(kind)
}

func musicSettingsSetEnum(i int, v string) int {
	musicClear()
	if i != musicSetBackend {
		return musicFail("not an enum")
	}
	ok := false
	for _, t := range strings.Split(musicSettingsEnums(i), ",") {
		if t == v {
			ok = true
		}
	}
	if !ok {
		return musicFail("unknown value")
	}
	if err := app.setBackend(v); err != nil {
		return musicFail(err.Error())
	}
	return 0
}

func musicClear() {
	musicSettingsMu.Lock()
	musicSettingsErr = ""
	musicSettingsMu.Unlock()
}

func musicFail(msg string) int {
	musicSettingsMu.Lock()
	musicSettingsErr = msg
	musicSettingsMu.Unlock()
	return 1
}
