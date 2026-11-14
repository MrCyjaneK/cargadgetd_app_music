package player

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MPRIS is read with gdbus, the same way apps/music does it: list session
// names, prefer the player whose PlaybackStatus is Playing, then Metadata.

const mprisTimeout = 1500 * time.Millisecond

const pollEvery = time.Second

var runGdbus = gdbus

type mprisSource struct {
	mu         sync.Mutex
	pollMu     sync.Mutex
	tr         Track
	artURL     string
	title      string
	cached     string
	wantQueue  bool
	queue      []QueueTrack
	queueKey   string
	queueFresh bool

	once sync.Once
	stop chan struct{}
	done chan struct{}
}

func openMPRIS() Source {
	s := &mprisSource{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	go s.loop()
	return s
}

func (s *mprisSource) loop() {
	defer close(s.done)
	s.poll()
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-tick.C:
			s.poll()
		}
	}
}

func (s *mprisSource) poll() {
	if s.stop != nil {
		select {
		case <-s.stop:
			return
		default:
		}
	}
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	s.pollTrack()
	s.mu.Lock()
	want := s.wantQueue
	s.mu.Unlock()
	if want {
		s.refreshQueue()
	}
}

// pollTrack reads the current player and leaves the queue alone.
func (s *mprisSource) pollTrack() {
	tr := fetch(s)
	s.mu.Lock()
	s.tr = tr
	s.mu.Unlock()
}

// Control runs Play, Pause, Next, or Previous on the current player.
// playpause follows the last snapshot. play and pause are the separate
// head-unit keys. stop is Pause: MPRIS Stop drops the bus name.
func (s *mprisSource) Control(cmd string) {
	if s.stop != nil {
		select {
		case <-s.stop:
			return
		default:
		}
	}
	go s.control(cmd)
}

func (s *mprisSource) control(cmd string) {
	if s.stop != nil {
		select {
		case <-s.stop:
			return
		default:
		}
	}
	s.mu.Lock()
	playing := s.tr.Status == "Playing"
	s.mu.Unlock()
	method := ""
	switch cmd {
	case "next":
		method = "Next"
	case "prev":
		method = "Previous"
	case "play":
		method = "Play"
	case "pause", "stop":
		// Stop drops the MPRIS bus name until the app is opened again.
		method = "Pause"
	case "playpause":
		if playing {
			method = "Pause"
		} else {
			method = "Play"
		}
	default:
		return
	}
	dest := s.dest()
	if dest == "" {
		return
	}
	if method == "Play" || method == "Pause" {
		s.mu.Lock()
		if method == "Pause" {
			s.tr.Status = "Paused"
		} else {
			s.tr.Status = "Playing"
		}
		s.mu.Unlock()
	}
	if _, err := callPlayer(dest, method); err != nil {
		s.mu.Lock()
		s.tr.Err = method + ": " + err.Error()
		s.mu.Unlock()
		return
	}
	s.afterChange()
}

func (s *mprisSource) Snapshot() Track {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tr
}

func (s *mprisSource) Queue() []QueueTrack {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]QueueTrack, len(s.queue))
	copy(out, s.queue)
	return out
}

// SyncQueue starts a track-list refresh. Later polls read Tracks again.
// GetTracksMetadata runs when that id list changes, and also when the same
// paths now describe a different song. This bridge keeps q0, q1, q2 and
// slides the current song into q0. An empty or short reply leaves the queue
// already on screen alone.
func (s *mprisSource) SyncQueue() {
	if !s.alive() {
		return
	}
	s.mu.Lock()
	s.wantQueue = true
	s.mu.Unlock()
	go func() {
		if !s.alive() {
			return
		}
		s.pollMu.Lock()
		defer s.pollMu.Unlock()
		if !s.alive() {
			return
		}
		s.refreshQueue()
	}()
}

// GoTo plays the track id on TrackList. It returns before the bus call.
func (s *mprisSource) GoTo(id string) {
	if id == "" || !s.alive() {
		return
	}
	go s.goTo(id)
}

func (s *mprisSource) goTo(id string) {
	if !s.alive() {
		return
	}
	dest := s.dest()
	if dest == "" {
		return
	}
	if _, err := callTrackList(dest, "GoTo", id); err != nil {
		s.mu.Lock()
		s.tr.Err = "GoTo: " + err.Error()
		s.mu.Unlock()
		return
	}
	s.afterChange()
}

// afterChange reads the player again and, when the queue is open, reloads
// it if the current song is no longer the row we cached for that path.
func (s *mprisSource) afterChange() {
	if !s.alive() {
		return
	}
	s.pollMu.Lock()
	defer s.pollMu.Unlock()
	if !s.alive() {
		return
	}
	s.pollTrack()
	s.mu.Lock()
	want := s.wantQueue
	s.mu.Unlock()
	if want {
		s.refreshQueue()
	}
}

func (s *mprisSource) alive() bool {
	if s.stop == nil {
		return true
	}
	select {
	case <-s.stop:
		return false
	default:
		return true
	}
}

func (s *mprisSource) refreshQueue() {
	dest := s.dest()
	if dest == "" {
		return
	}
	raw, err := getProp(dest, "org.mpris.MediaPlayer2.TrackList", "Tracks")
	if err != nil {
		s.mu.Lock()
		if s.tr.Err == "" {
			s.tr.Err = "tracks: " + err.Error()
		}
		s.mu.Unlock()
		return
	}
	ids := parseObjectPaths(raw)
	if len(ids) == 0 {
		return
	}
	key := strings.Join(ids, "\n")
	s.mu.Lock()
	same := key == s.queueKey && s.queueFresh && !s.queueMovedLocked()
	s.mu.Unlock()
	if same {
		return
	}
	meta, err := callTrackList(dest, "GetTracksMetadata", formatTrackIDs(ids))
	if err != nil {
		s.mu.Lock()
		s.tr.Err = "GetTracksMetadata: " + err.Error()
		s.queueFresh = false
		s.mu.Unlock()
		return
	}
	if len(splitDicts(meta)) < len(ids) {
		s.mu.Lock()
		s.queueFresh = false
		s.mu.Unlock()
		return
	}
	items := parseQueue(meta, ids)
	s.mu.Lock()
	s.queue = items
	s.queueKey = key
	s.queueFresh = true
	s.mu.Unlock()
}

// queueMovedLocked reports that the cached row for the current track id
// is a different song. The caller holds s.mu. Paths on this bridge stay
// q0, q1, q2 while the songs slide forward.
func (s *mprisSource) queueMovedLocked() bool {
	if s.tr.ID == "" || s.tr.Title == "" {
		return false
	}
	for _, item := range s.queue {
		if item.ID != s.tr.ID {
			continue
		}
		return item.Title != s.tr.Title || item.Artist != s.tr.Artist
	}
	return false
}

func (s *mprisSource) Close() {
	s.once.Do(func() { close(s.stop) })
	<-s.done
}

func fetch(s *mprisSource) Track {
	dest := s.dest()
	if dest == "" {
		return Track{Err: "no player"}
	}
	status, err := getProp(dest, "org.mpris.MediaPlayer2.Player", "PlaybackStatus")
	if err != nil {
		s.mu.Lock()
		s.cached = ""
		s.mu.Unlock()
		return Track{Err: "player unavailable"}
	}
	tr := Track{Status: parseVariantString(status)}
	if pos, err := getProp(dest, "org.mpris.MediaPlayer2.Player", "Position"); err == nil {
		tr.PositionUs = parseVariantInt(pos)
	}
	meta, err := getProp(dest, "org.mpris.MediaPlayer2.Player", "Metadata")
	if err != nil {
		return tr
	}
	tr.Title = parseMetaString(meta, "xesam:title")
	tr.Artist = parseMetaArtist(meta)
	tr.Album = parseMetaString(meta, "xesam:album")
	tr.AlbumArtist = parseMetaList(meta, "xesam:albumArtist")
	tr.ID = parseMetaPath(meta, "mpris:trackid")
	tr.LengthUs = parseMetaInt(meta, "mpris:length")
	artURL := parseMetaString(meta, "mpris:artUrl")
	s.mu.Lock()
	sameArt := artURL != "" && artURL == s.artURL && tr.Title == s.title && s.tr.Art != nil
	prev := s.tr.Art
	s.mu.Unlock()
	if sameArt {
		tr.Art = prev
		return tr
	}
	if artURL != "" {
		tr.Art = loadArt(artURL)
	}
	s.mu.Lock()
	s.artURL = artURL
	s.title = tr.Title
	s.mu.Unlock()
	return tr
}

func (s *mprisSource) dest() string {
	s.mu.Lock()
	cached := s.cached
	s.mu.Unlock()
	if cached != "" {
		return cached
	}
	picked := pick()
	s.mu.Lock()
	s.cached = picked
	s.mu.Unlock()
	return picked
}

func pick() string {
	names := listPlayers()
	var first string
	for _, n := range names {
		if first == "" {
			first = n
		}
		out, err := getProp(n, "org.mpris.MediaPlayer2.Player", "PlaybackStatus")
		if err == nil && parseVariantString(out) == "Playing" {
			return n
		}
	}
	return first
}

func listPlayers() []string {
	out, err := runGdbus("call", "--session",
		"--dest", "org.freedesktop.DBus",
		"--object-path", "/org/freedesktop/DBus",
		"--method", "org.freedesktop.DBus.ListNames")
	if err != nil {
		return nil
	}
	return mprisNameRe.FindAllString(out, -1)
}

func callPlayer(dest, method string) (string, error) {
	return runGdbus("call", "--session",
		"--dest", dest,
		"--object-path", "/org/mpris/MediaPlayer2",
		"--method", "org.mpris.MediaPlayer2.Player."+method)
}

func callTrackList(dest, method string, args ...string) (string, error) {
	cmd := []string{"call", "--session",
		"--dest", dest,
		"--object-path", "/org/mpris/MediaPlayer2",
		"--method", "org.mpris.MediaPlayer2.TrackList." + method}
	cmd = append(cmd, args...)
	return runGdbus(cmd...)
}

func formatTrackIDs(ids []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, id := range ids {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('\'')
		b.WriteString(id)
		b.WriteByte('\'')
	}
	b.WriteByte(']')
	return b.String()
}

func getProp(dest, iface, prop string) (string, error) {
	return runGdbus("call", "--session",
		"--dest", dest,
		"--object-path", "/org/mpris/MediaPlayer2",
		"--method", "org.freedesktop.DBus.Properties.Get",
		iface, prop)
}

func gdbus(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mprisTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gdbus", args...)
	cmd.Env = sessionBusEnv()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("gdbus timed out")
	}
	if err != nil {
		return out, err
	}
	return out, nil
}

func sessionBusEnv() []string {
	env := os.Environ()
	if strings.TrimSpace(os.Getenv("DBUS_SESSION_BUS_ADDRESS")) != "" {
		return env
	}
	candidates := []string{
		fmt.Sprintf("/run/user/%d/dbus/user_bus_socket", os.Getuid()),
		"/run/user/100000/dbus/user_bus_socket",
	}
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		candidates = append([]string{xdg + "/dbus/user_bus_socket"}, candidates...)
	}
	for _, sock := range candidates {
		if _, err := os.Stat(sock); err == nil {
			return append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+sock)
		}
	}
	return env
}

var mprisNameRe = regexp.MustCompile(`org\.mpris\.MediaPlayer2\.[A-Za-z0-9._-]+`)

func parseVariantString(s string) string {
	re := regexp.MustCompile(`<'([^']*)'>`)
	m := re.FindStringSubmatch(s)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func parseMetaString(meta, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `': <'([^']*)'>`)
	m := re.FindStringSubmatch(meta)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func parseMetaArtist(meta string) string {
	if s := parseMetaList(meta, "xesam:artist"); s != "" {
		return s
	}
	return parseMetaList(meta, "xesam:albumArtist")
}

func parseMetaList(meta, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `': <\[([^\]]*)\]>`)
	m := re.FindStringSubmatch(meta)
	if len(m) == 2 {
		a := strings.ReplaceAll(m[1], "'", "")
		a = strings.TrimSpace(a)
		if i := strings.IndexByte(a, ','); i >= 0 {
			a = strings.TrimSpace(a[:i])
		}
		return a
	}
	return parseMetaString(meta, key)
}

func parseMetaPath(meta, key string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `': <objectpath '([^']*)'>`)
	m := re.FindStringSubmatch(meta)
	if len(m) == 2 {
		return m[1]
	}
	return parseMetaString(meta, key)
}

func parseMetaInt(meta, key string) int64 {
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `': <int64 (-?\d+)>`)
	m := re.FindStringSubmatch(meta)
	if len(m) != 2 {
		return 0
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func parseVariantInt(s string) int64 {
	re := regexp.MustCompile(`<int64 (-?\d+)>`)
	m := re.FindStringSubmatch(s)
	if len(m) != 2 {
		return 0
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// gdbus prints an object-path array as
// [objectpath '/a', '/b', '/c']: only the first element carries the type tag.
var quotedPathRe = regexp.MustCompile(`'(/[^']*)'`)

func parseObjectPaths(s string) []string {
	ms := quotedPathRe.FindAllStringSubmatch(s, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if m[1] != "" {
			out = append(out, m[1])
		}
	}
	return out
}

func splitDicts(s string) []string {
	var out []string
	depth := 0
	start := -1
	for i, r := range s {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, s[start:i+len(string(r))])
				start = -1
			}
		}
	}
	return out
}

func parseQueue(meta string, ids []string) []QueueTrack {
	dicts := splitDicts(meta)
	out := make([]QueueTrack, 0, len(ids))
	for i, id := range ids {
		// GoTo takes the Tracks path. Metadata track ids are a different
		// spelling on some rows and are not a second name for that path.
		item := QueueTrack{ID: id}
		if i < len(dicts) {
			d := dicts[i]
			item.Title = parseMetaString(d, "xesam:title")
			item.Artist = parseMetaList(d, "xesam:artist")
			item.Album = parseMetaString(d, "xesam:album")
		}
		out = append(out, item)
	}
	return out
}
