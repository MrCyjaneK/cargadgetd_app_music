package main

import (
	"strings"

	"github.com/mrcyjanek/cargadgetd_sdk/cfg"
)

// loadBackend is plugins/music.yaml key backend. Missing means mpris.
func loadBackend() string {
	if v, ok := cfg.OwnGet("music", "backend"); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return "mpris"
}

func patchBackend(kind string) error {
	return cfg.OwnSet("music", "backend", kind)
}
