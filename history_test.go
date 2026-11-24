package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAggregateAcrossBoundaryAndGap(t *testing.T) {
	bars := []bucket{{Start: 0, End: 100}, {Start: 100, End: 200}, {Start: 200, End: 300}}
	measurements := []sample{{70, 200}, {130, 260}, {250, 500}, {290, 10}}
	result := aggregate(bars, measurements)
	if result[0].Bytes != 30 || result[1].Bytes != 30 || result[2].Seconds != 0 {
		t.Fatalf("unexpected chart buckets: %+v", result)
	}
}

func TestPeriodsAcrossClockChange(t *testing.T) {
	location, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Fatal(err)
	}
	day, week := periods(time.Date(2026, time.October, 25, 12, 0, 0, 0, location))
	if len(day) != 25 || week[6].End-week[6].Start != 25*3600 {
		t.Fatalf("unexpected clock change: %d hours, Sunday %+v", len(day), week[6])
	}
}

func TestSnapshotAndHistoryPersistTogether(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.November, 24, 15, 0, 0, 0, time.UTC)
	if err := db.save(snapshot{At: now.Unix(), Torrents: []torrent{{Name: "Sample"}}}, 42); err != nil {
		t.Fatal(err)
	}
	_, _, samples, snap, err := db.read(now)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].bytes != 42 || snap.At != now.Unix() ||
		len(snap.Torrents) != 1 || snap.Torrents[0].Name != "Sample" {
		t.Fatalf("unexpected saved data: %+v, %+v", samples, snap)
	}
}
