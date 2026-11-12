// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// What a file and the NFO beside it say of a title's people, read again outside a scan: for a
// refresh, and for titles scanned before credits existed.

// nfoCredits reads the people of the NFO beside a title: a film's, an episode's (with pictures
// from its show's .actors folder) or a show's tvshow.nfo. An NFO that isn't there, or can't be
// read, names nobody.
func (s *Scanner) nfoCredits(ctx context.Context, t *Title) []CreditIn {
	var scratch Item
	var show Show
	switch {
	case t.Item != nil && t.Item.ShowID != nil:
		it := t.Item
		dir, base := filepath.Dir(it.Path), strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
		applyEpisodeNFO(&scratch, filepath.Join(dir, base+".nfo"))
		if sh, err := s.Store.Show(ctx, *it.ShowID, 0); err == nil {
			addActorPhotos(scratch.credits[CreditsNFO], newActorPhotos(sh.Path))
		}
		return scratch.credits[CreditsNFO]
	case t.Item != nil:
		it := t.Item
		dir, base := filepath.Dir(it.Path), strings.TrimSuffix(filepath.Base(it.Path), filepath.Ext(it.Path))
		if it.Kind == "dvd-folder" || it.Kind == "bluray-folder" {
			dir, base = it.Path, discFolderName(it.Path)
		}
		return filmNFOPeople(filmNFOs(it.Path, dir, base))
	case t.Show != nil:
		applyShowNFO(&show, filepath.Join(t.Show.Path, "tvshow.nfo"))
		return show.credits[CreditsNFO]
	}
	return nil
}

// tagCredits reads the people a video file's own tags name, when the file can be read.
func (s *Scanner) tagCredits(ctx context.Context, it *Item) ([]CreditIn, bool) {
	if it == nil || it.Kind != "video" || s.Tools == nil {
		return nil, false
	}
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	p, err := s.Tools.ProbeFile(pctx, it.Path)
	if err != nil {
		return nil, false
	}
	return creditsFromTags(p.Format.Tags), true
}

// rereadLocalCredits reads a title's NFO and file tags again, as a refresh does.
func (s *Scanner) rereadLocalCredits(ctx context.Context, t *Title) {
	c := t.pendingCredits()
	if c == nil {
		return
	}
	c.set(CreditsNFO, s.nfoCredits(ctx, t))
	if list, ok := s.tagCredits(ctx, t.Item); ok {
		c.set(CreditsTags, list)
	}
}

// The tags a video's container may name people in (Matroska's names, which ffprobe passes on
// as they are written, and what MP4's ©dir becomes), with what each says they did. A file's
// "artist" is who made the music, or nobody in particular, never its director.
var tagJobs = map[string]struct{ role, job string }{
	"director":                {RoleCrew, "Director"},
	"©dir":                    {RoleCrew, "Director"},
	"directed_by":             {RoleCrew, "Director"},
	"written_by":              {RoleCrew, "Writer"},
	"writer":                  {RoleCrew, "Writer"},
	"screenplay_by":           {RoleCrew, "Screenplay"},
	"producer":                {RoleCrew, "Producer"},
	"executive_producer":      {RoleCrew, "Executive Producer"},
	"coproducer":              {RoleCrew, "Co-Producer"},
	"composer":                {RoleCrew, "Composer"},
	"director_of_photography": {RoleCrew, "Director of Photography"},
	"edited_by":               {RoleCrew, "Editor"},
	"production_designer":     {RoleCrew, "Production Design"},
	"costume_designer":        {RoleCrew, "Costume Design"},
	"actor":                   {RoleCast, ""},
}

// creditsFromTags turns a file's tags into credits, in a fixed order whatever order the tags
// came in.
func creditsFromTags(tags map[string]string) []CreditIn {
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sortStrings(keys)
	out := []CreditIn{}
	for _, k := range keys {
		j, ok := tagJobs[strings.ToLower(strings.TrimSpace(k))]
		if !ok {
			continue
		}
		list := creditsFromText(tags[k], j.role, "", j.job)
		if j.role == RoleCast {
			castOrder(list, nil)
		}
		out = append(out, list...)
	}
	return out
}
