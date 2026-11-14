package player

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	artFetchTimeout = 2 * time.Second
	artMaxBytes     = 4 << 20
)

// loadArt reads an mpris:artUrl. Same shapes as apps/music: file, http, data.
func loadArt(raw string) image.Image {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	switch {
	case strings.HasPrefix(raw, "file://"):
		u, err := url.Parse(raw)
		if err != nil || u.Path == "" {
			return nil
		}
		return decodeArtFile(u.Path)
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return decodeArtHTTP(raw)
	case strings.HasPrefix(raw, "data:image/"):
		return decodeArtData(raw)
	case strings.HasPrefix(raw, "/"):
		return decodeArtFile(raw)
	default:
		return nil
	}
}

func decodeArtFile(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return decodeArt(io.LimitReader(f, artMaxBytes))
}

func decodeArtHTTP(raw string) image.Image {
	ctx, cancel := context.WithTimeout(context.Background(), artFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil
	}
	return decodeArt(io.LimitReader(res.Body, artMaxBytes))
}

func decodeArtData(raw string) image.Image {
	i := strings.Index(raw, ",")
	if i < 0 {
		return nil
	}
	payload := raw[i+1:]
	var r io.Reader = strings.NewReader(payload)
	if strings.Contains(raw[:i], "base64") {
		r = base64.NewDecoder(base64.StdEncoding, r)
	}
	return decodeArt(io.LimitReader(r, artMaxBytes))
}

func decodeArt(r io.Reader) image.Image {
	b, err := io.ReadAll(r)
	if err != nil || len(b) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return img
}
