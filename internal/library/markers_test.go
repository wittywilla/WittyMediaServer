package library

import (
	"context"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/mediaextras"
)

func TestChapterMarkersAndIntro(t *testing.T) {
	var p ffmpeg.Probe
	add := func(start, end, title string) {
		p.Chapters = append(p.Chapters, struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		}{start, end, map[string]string{"title": title}})
	}
	add("0", "40", "Previously on")
	add("40", "100", "Opening")
	add("100", "1300", "Act One")
	add("1300", "1380", "Ending")
	m := chapterMarkers(&p)
	if m.Recap == nil || m.Recap.EndSec != 40 || m.Intro == nil || m.Intro.StartSec != 40 || m.Credits == nil || m.Credits.StartSec != 1300 {
		t.Errorf("%+v %+v %+v", m.Recap, m.Intro, m.Credits)
	}
	for title, want := range map[string]bool{"Intro": true, "Opening Credits": true, "OP": true, "Outro": false, "Ending Theme": false, "Previously on": false, "Vorspann": true} {
		if introChapter(title) != want {
			t.Errorf("introChapter(%q) = %v", title, !want)
		}
	}
}

func TestScannedMarkersLeaveManualAlone(t *testing.T) {
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	s := &Store{DB: d}
	ctx := context.Background()
	lib, err := s.CreateLibrary(ctx, "movies", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	it := &Item{LibraryID: lib.ID, Kind: "video", Path: "/x/film.mkv", Title: "Film", SortTitle: "Film",
		markers: &mediaextras.Markers{Credits: &mediaextras.Range{StartSec: 100, EndSec: 200}}}
	if err := s.upsertItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	if st := s.Markers(ctx, it.ID); st.Scanned == nil || st.Scanned.Credits.StartSec != 100 {
		t.Fatalf("scanned on insert: %+v", st)
	}
	if err := s.SetManualMarkers(ctx, it.ID, &mediaextras.Markers{Intro: &mediaextras.Range{StartSec: 1, EndSec: 9}}); err != nil {
		t.Fatal(err)
	}
	it.markers = &mediaextras.Markers{} // a scan after the chapters were taken out
	if err := s.upsertItem(ctx, it); err != nil {
		t.Fatal(err)
	}
	st := s.Markers(ctx, it.ID)
	if st.Scanned != nil {
		t.Errorf("old chapters kept: %+v", st.Scanned)
	}
	if st.Manual == nil || st.Manual.Intro.EndSec != 9 {
		t.Errorf("a scan touched the manual markers: %+v", st.Manual)
	}
	if err := s.SetManualMarkers(ctx, 99999, nil); err == nil {
		t.Error("markers for no item")
	}
}
