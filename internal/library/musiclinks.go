package library

import (
	"context"

	"github.com/wittywilla/wittymediaserver/internal/music"
)

// recordingIDs reads the ISRC and MusicBrainz recording a file's tags carry, for tying
// songs to their music videos (internal/music).
func recordingIDs(tags []map[string]string) *[2]string {
	return &[2]string{
		music.CleanISRC(tag(tags, "isrc", "tsrc")),
		music.CleanMBID(tag(tags, "musicbrainz_trackid", "musicbrainz track id", "musicbrainz_recordingid", "musicbrainz recording id")),
	}
}

// saveRecordingIDs keeps them, for songs of music libraries and music videos.
func (s *Scanner) saveRecordingIDs(ctx context.Context, lib *Library, it *Item) error {
	if it.recIDs == nil || (lib.Type != "music" && lib.Type != "musicvideos") {
		return nil
	}
	links := music.Links{DB: s.Store.DB.DB}
	return links.SaveIDs(ctx, it.ID, it.recIDs[0], it.recIDs[1])
}
