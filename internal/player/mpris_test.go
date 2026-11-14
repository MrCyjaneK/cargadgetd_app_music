package player

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	if Normalize("mpris") != "mpris" || Normalize("MPRIS") != "mpris" || Normalize("") != "mpris" {
		t.Fatal(Normalize(""))
	}
	if Normalize("mock") != "mock" || Normalize("Mock") != "mock" {
		t.Fatal(Normalize("Mock"))
	}
	if Normalize("nope") != "mpris" {
		t.Fatal(Normalize("nope"))
	}
	if Open("mock").Snapshot().Title != "Ghost Signal" {
		t.Fatal("mock backend")
	}
}

func TestFetchAppleMusic(t *testing.T) {
	meta := `({'mpris:artUrl': <'file:///tmp/no-such-cover.png'>, 'mpris:length': <int64 178932000>, 'mpris:trackid': <objectpath '/com/apple/android/music/_0036804863659'>, 'xesam:album': <'Terrified .'>, 'xesam:albumArtist': <['fakemink']>, 'xesam:artist': <['fakemink']>, 'xesam:title': <'Night , Blooming Jasmine .'>},)`
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "ListNames"):
			return `(['org.freedesktop.DBus', 'org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		case strings.Contains(joined, "PlaybackStatus"):
			return `(<'Playing'>,)`, nil
		case strings.Contains(joined, "Metadata"):
			return `(<` + meta + `>,)`, nil
		case strings.Contains(joined, "Position"):
			return `(<int64 5103000>,)`, nil
		default:
			t.Fatalf("unexpected gdbus %s", joined)
			return "", nil
		}
	}
	tr := fetch(&mprisSource{})
	if tr.Title != "Night , Blooming Jasmine ." || tr.Artist != "fakemink" || tr.Status != "Playing" {
		t.Fatalf("%+v", tr)
	}
	if tr.Album != "Terrified ." || tr.AlbumArtist != "fakemink" {
		t.Fatalf("album %+v", tr)
	}
	if tr.ID != "/com/apple/android/music/_0036804863659" || tr.LengthUs != 178932000 || tr.PositionUs != 5103000 {
		t.Fatalf("id %q len %d pos %d", tr.ID, tr.LengthUs, tr.PositionUs)
	}
	if !tr.Live() {
		t.Fatal("expected live")
	}
}

func TestControlPlayPauseNextPrev(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	status := "Playing"
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(joined, "Player.Pause"):
			calls = append(calls, "Pause")
			status = "Paused"
			return "()", nil
		case strings.Contains(joined, "Player.Next"):
			calls = append(calls, "Next")
			return "()", nil
		case strings.Contains(joined, "Player.Previous"):
			calls = append(calls, "Previous")
			return "()", nil
		case strings.Contains(joined, "Player.Play"):
			calls = append(calls, "Play")
			status = "Playing"
			return "()", nil
		case strings.Contains(joined, "ListNames"):
			return `(['org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		case strings.Contains(joined, "PlaybackStatus"):
			return `(<'` + status + `'>,)`, nil
		case strings.Contains(joined, "Metadata"):
			return `(<{'xesam:title': <'Night'>, 'xesam:artist': <['fakemink']>}>,)`, nil
		case strings.Contains(joined, "Position"):
			return `(<int64 0>,)`, nil
		default:
			t.Fatalf("unexpected gdbus %s", joined)
			return "", nil
		}
	}
	s := &mprisSource{}
	s.poll()
	if s.Snapshot().Status != "Playing" {
		t.Fatalf("before %+v", s.Snapshot())
	}
	s.control("playpause")
	s.control("next")
	s.control("prev")
	s.control("playpause")
	s.control("pause")
	s.control("play")
	s.control("stop")
	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	want := []string{"Pause", "Next", "Previous", "Play", "Pause", "Play", "Pause"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("calls %v", got)
	}
	if s.Snapshot().Status != "Paused" || s.Snapshot().Title != "Night" {
		t.Fatalf("%+v", s.Snapshot())
	}
}

func TestFetchNoPlayer(t *testing.T) {
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		return `(['org.freedesktop.DBus'],)`, nil
	}
	tr := fetch(&mprisSource{})
	if tr.Live() || tr.Err == "" {
		t.Fatalf("%+v", tr)
	}
}

func TestQueueAndGoTo(t *testing.T) {
	tracks := `(<[objectpath '/net/mrcyjanek/mprisam/q0', '/net/mrcyjanek/mprisam/q1']>,)`
	meta := `([{'xesam:artist': <['payton']>, 'xesam:album': <'Rich Boy - Single'>, 'mpris:length': <int64 111966000>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>, 'xesam:title': <'Rich Boy'>}, {'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q1'>, 'xesam:title': <'bitcrush!'>, 'xesam:artist': <['Devika']>}],)`
	var mu sync.Mutex
	var metaCalls int
	var gotoArg string
	tracksOut := tracks
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(joined, "TrackList.GoTo"):
			if len(args) > 0 {
				gotoArg = args[len(args)-1]
			}
			return "()", nil
		case strings.Contains(joined, "GetTracksMetadata"):
			metaCalls++
			if !strings.Contains(joined, "'/net/mrcyjanek/mprisam/q0'") {
				t.Fatalf("metadata args %s", joined)
			}
			return meta, nil
		case strings.Contains(joined, "TrackList") && strings.Contains(joined, "Tracks"):
			return tracksOut, nil
		case strings.Contains(joined, "ListNames"):
			return `(['org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		case strings.Contains(joined, "PlaybackStatus"):
			return `(<'Paused'>,)`, nil
		case strings.Contains(joined, "Metadata"):
			return `(<{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}>,)`, nil
		case strings.Contains(joined, "Position"):
			return `(<int64 5103000>,)`, nil
		default:
			t.Fatalf("unexpected gdbus %s", joined)
			return "", nil
		}
	}
	s := &mprisSource{}
	s.mu.Lock()
	s.wantQueue = true
	s.mu.Unlock()
	s.poll()
	s.poll()
	mu.Lock()
	n := metaCalls
	mu.Unlock()
	if n != 1 {
		t.Fatalf("metadata calls %d", n)
	}
	q := s.Queue()
	if len(q) != 2 || q[0].Title != "Rich Boy" || q[0].Artist != "payton" || q[0].Album != "Rich Boy - Single" || q[0].ID != "/net/mrcyjanek/mprisam/q0" {
		t.Fatalf("first %+v", q)
	}
	if q[1].Title != "bitcrush!" || q[1].Artist != "Devika" || q[1].Album != "" || q[1].ID != "/net/mrcyjanek/mprisam/q1" {
		t.Fatalf("second %+v", q[1])
	}
	mu.Lock()
	tracksOut = `(<[objectpath '/net/mrcyjanek/mprisam/q0']>,)`
	mu.Unlock()
	s.poll()
	mu.Lock()
	n = metaCalls
	mu.Unlock()
	if n != 2 {
		t.Fatalf("metadata calls after id change %d", n)
	}
	if len(s.Queue()) != 1 {
		t.Fatalf("queue %v", s.Queue())
	}

	s.goTo("/net/mrcyjanek/mprisam/q1")
	if len(s.Queue()) != 1 {
		t.Fatalf("goto rebuilt the queue: %v", s.Queue())
	}
	mu.Lock()
	got := gotoArg
	mu.Unlock()
	if got != "/net/mrcyjanek/mprisam/q1" {
		t.Fatalf("goto %q", got)
	}
}

func TestQueueKeepsRowsWhenReloadIsShort(t *testing.T) {
	full := `(<[objectpath '/net/mrcyjanek/mprisam/q0', '/net/mrcyjanek/mprisam/q1']>,)`
	meta := `([{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>, 'mpris:trackid': <objectpath '/com/apple/android/music/other'>}, {'xesam:title': <'bitcrush!'>, 'xesam:artist': <['Devika']>}],)`
	short := `([{'xesam:title': <'Only'>, 'xesam:artist': <['One']>}],)`
	tracksOut := full
	metaOut := meta
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "GetTracksMetadata"):
			return metaOut, nil
		case strings.Contains(joined, "TrackList") && strings.Contains(joined, "Tracks"):
			return tracksOut, nil
		case strings.Contains(joined, "ListNames"):
			return `(['org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		case strings.Contains(joined, "PlaybackStatus"):
			return `(<'Paused'>,)`, nil
		case strings.Contains(joined, "Metadata"):
			return `(<{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}>,)`, nil
		case strings.Contains(joined, "Position"):
			return `(<int64 0>,)`, nil
		default:
			t.Fatalf("unexpected gdbus %s", joined)
			return "", nil
		}
	}
	s := &mprisSource{}
	s.mu.Lock()
	s.wantQueue = true
	s.mu.Unlock()
	s.poll()
	q := s.Queue()
	if len(q) != 2 || q[0].ID != "/net/mrcyjanek/mprisam/q0" || q[1].ID != "/net/mrcyjanek/mprisam/q1" || q[0].Title != "Rich Boy" {
		t.Fatalf("queue %+v", q)
	}
	metaOut = short
	s.poll()
	if len(s.Queue()) != 2 || s.Queue()[1].Title != "bitcrush!" {
		t.Fatalf("short reload replaced the queue: %+v", s.Queue())
	}
	tracksOut = `(<[]>,)`
	s.poll()
	if len(s.Queue()) != 2 {
		t.Fatalf("empty tracks cleared the queue: %+v", s.Queue())
	}
}

func TestQueueRefreshesWhenSongSlidesToFront(t *testing.T) {
	tracks := `(<[objectpath '/net/mrcyjanek/mprisam/q0', '/net/mrcyjanek/mprisam/q1']>,)`
	first := `([{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}, {'xesam:title': <'bitcrush!'>, 'xesam:artist': <['Devika']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q1'>}],)`
	shifted := `([{'xesam:title': <'bitcrush!'>, 'xesam:artist': <['Devika']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}, {'xesam:title': <'Next'>, 'xesam:artist': <['Other']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q1'>}],)`
	player := `(<{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}>,)`
	metaOut := first
	var mu sync.Mutex
	var metaCalls int
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.Contains(joined, "GetTracksMetadata"):
			metaCalls++
			return metaOut, nil
		case strings.Contains(joined, "TrackList") && strings.Contains(joined, "Tracks"):
			return tracks, nil
		case strings.Contains(joined, "ListNames"):
			return `(['org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		case strings.Contains(joined, "PlaybackStatus"):
			return `(<'Playing'>,)`, nil
		case strings.Contains(joined, "Metadata"):
			return player, nil
		case strings.Contains(joined, "Position"):
			return `(<int64 0>,)`, nil
		default:
			t.Fatalf("unexpected gdbus %s", joined)
			return "", nil
		}
	}
	s := &mprisSource{}
	s.mu.Lock()
	s.wantQueue = true
	s.mu.Unlock()
	s.poll()
	s.poll()
	mu.Lock()
	n := metaCalls
	mu.Unlock()
	if n != 1 || s.Queue()[0].Title != "Rich Boy" || s.Queue()[1].ID != "/net/mrcyjanek/mprisam/q1" {
		t.Fatalf("calls %d queue %+v", n, s.Queue())
	}
	mu.Lock()
	player = `(<{'xesam:title': <'bitcrush!'>, 'xesam:artist': <['Devika']>, 'mpris:trackid': <objectpath '/net/mrcyjanek/mprisam/q0'>}>,)`
	metaOut = shifted
	mu.Unlock()
	s.poll()
	q := s.Queue()
	mu.Lock()
	n = metaCalls
	mu.Unlock()
	if n != 2 || len(q) != 2 || q[0].Title != "bitcrush!" || q[0].ID != "/net/mrcyjanek/mprisam/q0" || q[1].Title != "Next" {
		t.Fatalf("calls %d queue %+v", n, q)
	}
}

func TestGoToDoesNotBlock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	prev := runGdbus
	t.Cleanup(func() { runGdbus = prev })
	runGdbus = func(args ...string) (string, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "TrackList.GoTo") {
			select {
			case <-started:
			default:
				close(started)
			}
			<-release
			return "()", nil
		}
		if strings.Contains(joined, "ListNames") {
			return `(['org.mpris.MediaPlayer2.AppSupport.com.apple.android.music'],)`, nil
		}
		if strings.Contains(joined, "PlaybackStatus") {
			return `(<'Playing'>,)`, nil
		}
		if strings.Contains(joined, "Metadata") {
			return `(<{'xesam:title': <'Rich Boy'>, 'xesam:artist': <['payton']>}>,)`, nil
		}
		if strings.Contains(joined, "Position") {
			return `(<int64 0>,)`, nil
		}
		if strings.Contains(joined, "Tracks") {
			return `(<[]>,)`, nil
		}
		t.Fatalf("unexpected gdbus %s", joined)
		return "", nil
	}
	s := &mprisSource{}
	done := make(chan struct{})
	go func() {
		s.GoTo("/net/mrcyjanek/mprisam/q0")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GoTo blocked")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("GoTo never called the bus")
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.Snapshot().Title == "Rich Boy" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("poll did not finish")
}

func TestLoadArtFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cover.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	img := loadArt("file://" + path)
	if img == nil || img.Bounds().Dx() != 2 {
		t.Fatalf("%v", img)
	}
}
