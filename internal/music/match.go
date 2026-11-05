package music

import (
	"math"
	"sort"
)

// Recording is a song or a music video as matching sees it.
type Recording struct {
	ID      int64
	Title   string
	Artists []string // a song's artist and album artist; a video's artist
	ISRC    string
	MBID    string // MusicBrainz recording
	Seconds float64
}

// Pair is a song and a video that belong together. Sure pairs are tied; the rest are
// suggestions for an admin to look at.
type Pair struct {
	SongID  int64
	VideoID int64
	How     string // isrc | recording | name | close
	Sure    bool
	// SyncMS is where the song's start is in the video, when the two line up.
	SyncMS *int64
}

// The same take, give or take how a file was cut.
const lineUpSeconds = 2.0

// A video this long is a concert or a set, not one song.
const concertSeconds = 20 * 60

// How alike two titles or artists must be to be suggested at all.
const closeEnough = 0.8

// At most this many suggestions for one video.
const maxSuggestions = 3

// Match pairs videos with songs. A shared ISRC or MusicBrainz recording is sure, and so is
// the same artist and title when exactly one song fits and neither is another take (live,
// acoustic, a remix). Everything less certain is a suggestion: nothing fuzzy is ever sure.
// skip leaves out pairs someone has untied.
func Match(songs, videos []Recording, skip func(song, video int64) bool) []Pair {
	byISRC := map[string][]*Recording{}
	byMBID := map[string][]*Recording{}
	byName := map[string][]*Recording{}   // title and artist
	byTitle := map[string][]*Recording{}  // title alone
	byArtist := map[string][]*Recording{} // artist alone
	titleKeys := make(map[int64]string, len(songs))
	for i := range songs {
		s := &songs[i]
		if s.ISRC != "" {
			byISRC[s.ISRC] = append(byISRC[s.ISRC], s)
		}
		if s.MBID != "" {
			byMBID[s.MBID] = append(byMBID[s.MBID], s)
		}
		t := TitleKey(s.Title)
		if t == "" {
			continue
		}
		titleKeys[s.ID] = t
		byTitle[t] = append(byTitle[t], s)
		seen := map[string]bool{}
		for _, a := range s.Artists {
			k := ArtistKey(a)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			byName[t+"\x00"+k] = append(byName[t+"\x00"+k], s)
			byArtist[k] = append(byArtist[k], s)
		}
	}
	var out []Pair
	for i := range videos {
		v := &videos[i]
		if v.Seconds >= concertSeconds {
			continue
		}
		keep := func(list []*Recording) []*Recording {
			var kept []*Recording
			seen := map[int64]bool{}
			for _, s := range list {
				if !seen[s.ID] && (skip == nil || !skip(s.ID, v.ID)) {
					seen[s.ID] = true
					kept = append(kept, s)
				}
			}
			return kept
		}
		// The same recording, by its codes: as sure as it gets. Several files of one
		// recording (an album and a best-of) are one song, so the closest length wins.
		if s := closest(keep(byISRC[v.ISRC]), v); v.ISRC != "" && s != nil {
			out = append(out, pairOf(s, v, "isrc", true))
			continue
		}
		if s := closest(keep(byMBID[v.MBID]), v); v.MBID != "" && s != nil {
			out = append(out, pairOf(s, v, "recording", true))
			continue
		}
		t := TitleKey(v.Title)
		a := ""
		if len(v.Artists) > 0 {
			a = ArtistKey(v.Artists[0])
		}
		if t == "" {
			continue
		}
		vTake := IsTake(v.Title)
		named := keep(byName[t+"\x00"+a])
		if a != "" && len(named) > 0 {
			var same []*Recording
			for _, s := range named {
				if IsTake(s.Title) == vTake {
					same = append(same, s)
				}
			}
			if len(same) == 1 {
				out = append(out, pairOf(same[0], v, "name", true))
				continue
			}
			for _, s := range best(named, v, maxSuggestions) {
				out = append(out, pairOf(s, v, "name", false))
			}
			continue
		}
		// Close: the same title by an artist whose name is nearly the same, or a title
		// nearly the same by the same artist. Only ever suggested.
		var close []*Recording
		for _, s := range keep(byTitle[t]) {
			for _, sa := range s.Artists {
				if a != "" && similar(ArtistKey(sa), a) >= closeEnough {
					close = append(close, s)
					break
				}
			}
		}
		if a != "" {
			for _, s := range keep(byArtist[a]) {
				if st := titleKeys[s.ID]; st != t && (similar(st, t) >= closeEnough || baseKey(s.Title) == baseKey(v.Title)) {
					close = append(close, s)
				}
			}
		}
		for _, s := range best(close, v, maxSuggestions) {
			out = append(out, pairOf(s, v, "close", false))
		}
	}
	return out
}

func pairOf(s, v *Recording, how string, sure bool) Pair {
	p := Pair{SongID: s.ID, VideoID: v.ID, How: how, Sure: sure}
	if s.Seconds > 0 && v.Seconds > 0 && math.Abs(s.Seconds-v.Seconds) <= lineUpSeconds {
		zero := int64(0)
		p.SyncMS = &zero
	}
	return p
}

// closest is the song nearest the video's length, the oldest first when they tie.
func closest(list []*Recording, v *Recording) *Recording {
	b := best(list, v, 1)
	if len(b) == 0 {
		return nil
	}
	return b[0]
}

// best is up to n songs, nearest the video's length first.
func best(list []*Recording, v *Recording, n int) []*Recording {
	seen := map[int64]bool{}
	var uniq []*Recording
	for _, s := range list {
		if !seen[s.ID] {
			seen[s.ID] = true
			uniq = append(uniq, s)
		}
	}
	sort.SliceStable(uniq, func(i, j int) bool {
		di, dj := math.Abs(uniq[i].Seconds-v.Seconds), math.Abs(uniq[j].Seconds-v.Seconds)
		if di != dj {
			return di < dj
		}
		return uniq[i].ID < uniq[j].ID
	})
	if len(uniq) > n {
		uniq = uniq[:n]
	}
	return uniq
}
