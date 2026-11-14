package player

type mockSource struct{}

func (mockSource) Snapshot() Track {
	return Track{
		ID:          "/mock/0",
		Title:       "Ghost Signal",
		Artist:      "Mira Vale",
		Album:       "Night Drive",
		AlbumArtist: "Mira Vale",
		Status:      "Playing",
		LengthUs:    180_000_000,
		PositionUs:  12_000_000,
		Demo:        true,
	}
}

func (mockSource) Queue() []QueueTrack {
	return []QueueTrack{
		{ID: "/mock/0", Title: "Ghost Signal", Artist: "Mira Vale", Album: "Night Drive"},
		{ID: "/mock/1", Title: "Harbor Lights", Artist: "Mira Vale", Album: "Night Drive"},
		{ID: "/mock/2", Title: "Low Tide", Artist: "Mira Vale", Album: "Night Drive"},
	}
}

func (mockSource) Close() {}

func (mockSource) Control(string) {}

func (mockSource) GoTo(string) {}

func (mockSource) SyncQueue() {}
