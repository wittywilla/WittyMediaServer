// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import "github.com/wittywilla/wittymediaserver/pkg/wittyinfo"

// FromWittyinfo reads the skip segments of a .wittyinfo as markers. Of the kinds a video
// has one of, the first counts, except credits, where the last does (as with chapters); a
// segment without an end runs to the end (an EndSec of 0, see FillEnds).
func FromWittyinfo(x *wittyinfo.Info) Markers {
	var m Markers
	if x == nil {
		return m
	}
	for i := range x.Segments {
		s := &x.Segments[i]
		r := Range{StartSec: float64(s.Start), Label: cleanLabel(s.Label), Action: s.Act()}
		if s.End != nil {
			r.EndSec = float64(*s.End)
		}
		switch s.Type {
		case wittyinfo.SegIntro:
			if m.Intro == nil {
				m.Intro = &r
			}
		case wittyinfo.SegRecap:
			if m.Recap == nil {
				m.Recap = &r
			}
		case wittyinfo.SegCredits:
			m.Credits = &r
		case wittyinfo.SegPreview:
			if m.Preview == nil {
				m.Preview = &r
			}
		case wittyinfo.SegPostCredits:
			if m.PostCredits == nil {
				m.PostCredits = &r
			}
		case wittyinfo.SegCommercial:
			if len(m.Commercials) < maxRanges {
				m.Commercials = append(m.Commercials, r)
			}
		case wittyinfo.SegCustom:
			if len(m.Custom) < maxRanges {
				m.Custom = append(m.Custom, r)
			}
		}
	}
	m.Absent = append([]string(nil), x.Absent...)
	return m
}
