// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"testing"
)

func FuzzSubtitleText(f *testing.F) {
	f.Add([]byte("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nHello\n"), int64(90000), 1.5)
	f.Add([]byte("WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:1,LOCAL:00:00:00.000\n\n99:99:99.999 --> -1\n"), int64(-1), -1.0)
	f.Fuzz(func(t *testing.T, b []byte, pts int64, start float64) { timestampedVTT(b, pts, start) })
}

// Pictures that came from anyone with an account: a bezel, a background.
func FuzzPictures(f *testing.F) {
	f.Add([]byte("\x89PNG\r\n\x1a\n"))
	f.Add([]byte("\xff\xd8\xff"))
	f.Add([]byte("GIF89a"))
	f.Add([]byte("RIFF\x00\x00\x00\x00WEBP"))
	f.Add([]byte("<svg xmlns='http://www.w3.org/2000/svg'><script>x</script></svg>"))
	f.Fuzz(func(t *testing.T, b []byte) {
		imageType(bytes.NewReader(b), "x.png")
		_, _, _, _ = reencodeImage(b)
		_, _ = cleanBackground(b)
	})
}
