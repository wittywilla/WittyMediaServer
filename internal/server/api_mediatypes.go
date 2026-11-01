package server

import (
	"net/http"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/internal/mediatypes"
)

// mediaFormatView is a format as the Supported formats page shows it, with what the
// FFmpeg on this machine says about it.
type mediaFormatView struct {
	mediatypes.Format
	PlaysText string `json:"playsText"`
	// Available is false when this machine lacks what the format needs: an FFmpeg built
	// without libopenmpt can't play tracker music, for one.
	Available bool `json:"available"`
}

// GET /api/media-types: everything the server knows about file types, and which of them
// work on this machine. Modules and apps read this rather than keep lists of their own.
func (s *Server) handleMediaTypes(w http.ResponseWriter, r *http.Request) {
	all := mediatypes.All()
	out := make([]mediaFormatView, 0, len(all.Formats))
	for _, f := range all.Formats {
		if f.Category == mediatypes.Other {
			continue
		}
		v := mediaFormatView{Format: f, PlaysText: mediatypes.PlaysWords(f.Plays), Available: true}
		switch {
		case f.Demuxer != "" && !s.Tools.HasDemuxer(f.Demuxer):
			v.Available = false
		case f.ID == "djvu":
			v.Available = library.HasDjVu()
		}
		out = append(out, v)
	}
	answer := map[string]any{"version": all.Version, "formats": out, "codecs": all.Codecs}
	if userFrom(r).IsAdmin {
		answer["ffmpeg"] = s.Tools.Version
	}
	writeJSON(w, 200, answer)
}
