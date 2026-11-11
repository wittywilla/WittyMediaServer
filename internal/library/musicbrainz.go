// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"github.com/wittywilla/wittymediaserver/internal/cdlookup"
)

// MusicBrainz identifies audio CDs by their table of contents, and the Cover Art
// Archive has their covers. The client is in the WittyCD module, so that other programs
// can use it; this is the server's way of calling it.

// MusicBrainzAttribution is what is shown wherever their data is used.
const MusicBrainzAttribution = cdlookup.Attribution

const mbUA = "WittyMediaServer/0.5 ( https://github.com/wittywilla/WittyMediaServer )"

type (
	// CDMatch is one release a disc could be.
	CDMatch = cdlookup.Match
	// CDTrackName is a track's name from any source.
	CDTrackName = cdlookup.TrackName
)

func (s *Scanner) mbClient() *cdlookup.Client {
	return &cdlookup.Client{HTTP: SafeClient, UserAgent: mbUA, Base: s.MusicBrainzBase, CoverBase: s.CoverArtBase}
}
