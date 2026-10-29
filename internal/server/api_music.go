package server

import (
	"net/http"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Music and audiobooks: albums (or books), artists (or authors), the
// tracks of one album, and streaming one audio file.

// GET /api/music/albums?library=&artist=&search=&sort=
func (s *Server) handleAlbums(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	al, err := s.Store.Albums(r.Context(), int64(qInt(r, "library", 0)), q.Get("artist"), q.Get("search"), q.Get("sort"), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, al)
}

// GET /api/music/artists?library=
func (s *Server) handleArtists(w http.ResponseWriter, r *http.Request) {
	ar, err := s.Store.Artists(r.Context(), int64(qInt(r, "library", 0)), s.access(r))
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, ar)
}

// GET /api/music/tracks?library=&artist=&album= (album and artist may be
// empty strings, meaning "no album" / "unknown artist").
func (s *Server) handleTracks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	iq := library.ItemQuery{LibraryID: int64(qInt(r, "library", 0)), Kind: "audio", Sort: "track", Access: s.access(r)}
	if q.Has("album") {
		v := q.Get("album")
		iq.Album = &v
	}
	if q.Has("artist") {
		v := q.Get("artist")
		iq.AlbumArtist = &v
	}
	if iq.Album == nil && iq.AlbumArtist == nil {
		iq.Sort, iq.Limit = "title", 2000 // all songs of the library
	}
	items, _, err := s.Store.Items(r.Context(), iq)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	_ = s.Store.AttachProgress(r.Context(), userFrom(r).ID, items)
	writeJSON(w, 200, items)
}

// GET /api/items/{id}/stream?start=SEC: plays an audio file. Formats every
// browser understands are sent as they are (with seeking); others are
// converted to AAC while streaming, starting at start (the client reloads
// with a new start to seek). The conversion stops when the listener does.
func (s *Server) handleAudioStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "audio" {
		writeErr(w, 404, "not found")
		return
	}
	if library.DirectAudio[it.VideoCodec] && !qBool(r, "transcode") {
		w.Header().Set("Cache-Control", "private, max-age=3600")
		http.ServeFile(w, r, it.Path)
		return
	}
	start, _ := strconv.ParseFloat(r.URL.Query().Get("start"), 64)
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if start > 0 {
		args = append(args, "-ss", strconv.FormatFloat(start, 'f', 3, 64))
	}
	args = append(args, "-i", it.Path, "-map", "0:a:0", "-vn", "-sn", "-c:a", "aac", "-b:a", "192k", "-f", "adts", "pipe:1")
	cmd := ffmpeg.Command(r.Context(), s.Tools.FFmpeg, args...)
	cmd.Stdout = w
	w.Header().Set("Content-Type", "audio/aac")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-WMS-Transcoded", "1")
	s.Log.Info("audio transcode", "cat", "playback", "item", it.ID, "codec", it.VideoCodec, "start", start)
	_ = cmd.Run() // ends when the file is done or the listener disconnects
}
