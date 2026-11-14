// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
package library

import (
	"os"
	"path/filepath"
	"testing"
)

// Release file names written as an NFO's title are not titles; real titles with dots are.
func TestReleaseName(t *testing.T) {
	for _, s := range []string{
		"Invasion.of.the.Body.Snat ... VP.WEB-DL.DD.5.1.H.265-PiRaTeS",
		"Invasion.of.the.Body.Snatchers.1978.1080p.BluRay.x264-GROUP",
		"The.Matrix.1999.2160p.UHD.BluRay.REMUX.HDR.HEVC",
		"Alien.1979.Directors.Cut.720p.WEBRip.AAC",
		"Heat_1995_1080p_BluRay",
		"How_to_Train_Your_Dragon_ ... _WEB-DL_DDP5_1_H_264-SARVO.mkv",
		"How_to_Train_Your_Dragon_ ... B-DL_DDP_5_1_H_265-PiRaTeS.mkv",
		"How.the.Grinch.Stole.Chri ... 160p.AAC.2.0.HDR.x265-hallowed",
	} {
		if !releaseName(s) {
			t.Errorf("%q is a release name", s)
		}
	}
	for _, s := range []string{"Dr. No", "Mr. & Mrs. Smith", "Invasion of the Body Snatchers", "Star Wars: Episode IV – A New Hope", "Dial M for Murder", "The DVD Collection", "U.S. Marshals"} {
		if releaseName(s) {
			t.Errorf("%q is a title", s)
		}
	}
}

func TestNFOReleaseTitleGivesWay(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		p := filepath.Join(dir, "movie.nfo")
		if err := os.WriteFile(p, []byte(`<?xml version="1.0"?><movie>`+body+`<year>1978</year></movie>`), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	// The original title is a real one: it's the title.
	it := &Item{Title: "Invasion Of The Body Snatchers"}
	applyNFO(it, write(`<title>Invasion.of.the.Body.Snat ... VP.WEB-DL.DD.5.1.H.265-PiRaTeS</title><originaltitle>Invasion of the Body Snatchers</originaltitle>`))
	if it.Title != "Invasion of the Body Snatchers" {
		t.Fatalf("title %q", it.Title)
	}
	// No original title: the title from the folder stays.
	it = &Item{Title: "Invasion Of The Body Snatchers"}
	applyNFO(it, write(`<title>Invasion.of.the.Body.Snatchers.1978.1080p.BluRay.x264-GROUP</title>`))
	if it.Title != "Invasion Of The Body Snatchers" {
		t.Fatalf("title %q", it.Title)
	}
	// A real title is still taken.
	it = &Item{Title: "Folder Name"}
	applyNFO(it, write(`<title>Dr. No</title>`))
	if it.Title != "Dr. No" {
		t.Fatalf("title %q", it.Title)
	}
}

// Shortened release names (" ... " where the middle was) are cut there: the title is what came before.
func TestParseNameShortenedRelease(t *testing.T) {
	for in, want := range map[string]string{
		"How_to_Train_Your_Dragon_ ... _WEB-DL_DDP5_1_H_264-SARVO.mkv": "How to Train Your Dragon",
		"How_to_Train_Your_Dragon_ ... B-DL_DDP_5_1_H_265-PiRaTeS.mkv": "How to Train Your Dragon",
		"Invasion.of.the.Body.Snat ... VP.WEB-DL.DD.5.1.H.265-PiRaTeS": "Invasion of the Body Snat",
		"Some Film…1080p": "Some Film",
	} {
		if got, _ := ParseName(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
