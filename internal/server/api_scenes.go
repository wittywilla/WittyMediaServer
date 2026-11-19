// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// sceneThumbWidth: the scene picker's tiles and the seek bar's preview are at most 10rem wide,
// so twice that covers a sharp screen and keeps each picture near 15 KB.
const sceneThumbWidth = 320

// GET /api/items/{id}/scene/{sec}: a small picture of a video file at a whole second, for its
// scene picker and the preview over the seek bar (discs have /disc/thumb). One key frame is
// read with no other decoding, so it costs about as much as opening the file; pictures are kept
// and at most two are made at once (thumbSlots, shared with discs), so a scene list of fifty
// doesn't start fifty FFmpegs on a small server. The player asks only for the tiles on screen.
func (s *Server) handleSceneThumb(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 400, "bad id")
		return
	}
	sec, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("sec"), ".jpg"))
	if err != nil || sec < 0 {
		writeErr(w, 400, "bad time")
		return
	}
	it, err := s.visibleItem(r, id)
	if err != nil || it.Kind != "video" {
		writeErr(w, 404, "video not found")
		return
	}
	if it.RuntimeSec > 0 && float64(sec) > it.RuntimeSec {
		sec = int(it.RuntimeSec)
	}
	h := sha1.Sum([]byte(it.Path))
	out := filepath.Join(s.Cfg.ImagesDir, "scenes", fmt.Sprintf("%s-%d.jpg", hex.EncodeToString(h[:8]), sec))
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		select {
		case thumbSlots <- struct{}{}:
		case <-r.Context().Done():
			return
		}
		err := s.Tools.Poster(r.Context(), ffmpeg.FileInput(it.Path), float64(sec), sceneThumbWidth, out)
		<-thumbSlots
		if r.Context().Err() != nil {
			_ = os.Remove(out) // the tile scrolled away before the picture was made
			return
		}
		if err != nil {
			s.Log.DebugContext(r.Context(), "no scene picture", "cat", "library", "item", it.ID, "sec", sec, "err", err)
			writeErr(w, 404, errNoPicture.Error())
			return
		}
	}
	w.Header().Set("Cache-Control", "private, max-age=604800")
	http.ServeFile(w, r, out)
}
