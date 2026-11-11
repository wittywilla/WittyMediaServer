// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package libtranscode

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/ffmpeg"
)

// Checking a new file before anything is done with the old one. The questions: can it be
// opened, is it as long as the old one, are the tracks there that the plan says, and does
// it really decode? A wrong answer to any of them throws the new file away.

// expected counts what the plan says the new file holds.
func (pl *Plan) expected() (video, audio, subs int) {
	for _, st := range pl.Steps {
		switch st.Action {
		case ActKeep, ActConvert, ActAdd:
			switch st.Kind {
			case "video":
				video++
			case "audio":
				audio++
			case "subtitle":
				subs++
			}
		case ActEmbed:
			subs++
		}
	}
	return
}

// VerifyProblem is a plain sentence about why a new file was thrown away.
type VerifyProblem struct{ Why string }

func (v *VerifyProblem) Error() string { return v.Why }

// lengthOK allows what muxing and rounding move: two seconds, or one percent of a long film.
func lengthOK(before, after float64) bool {
	if before <= 0 {
		return after > 0
	}
	return math.Abs(before-after) <= math.Max(2, before*0.01)
}

func clock(sec float64) string {
	d := time.Duration(sec) * time.Second
	return fmt.Sprintf("%d:%02d:%02d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}

// Verify checks a new file against its source and plan.
func Verify(ctx context.Context, tools *ffmpeg.Tools, src *Source, pl *Plan, path string) error {
	pctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	p, err := tools.ProbeFile(pctx, path)
	if err != nil {
		return &VerifyProblem{"The new file couldn't be opened, so the old one stays."}
	}
	out := Inspect(path, p)
	wantV, wantA, wantS := pl.expected()
	if len(out.Video) != wantV {
		return &VerifyProblem{fmt.Sprintf("The new file has %d picture streams and should have %d, so the old one stays.", len(out.Video), wantV)}
	}
	if len(out.Audio) != wantA {
		return &VerifyProblem{fmt.Sprintf("The new file has %d sound tracks and should have %d, so the old one stays.", len(out.Audio), wantA)}
	}
	if len(out.Subs) != wantS {
		return &VerifyProblem{fmt.Sprintf("The new file has %d subtitle tracks and should have %d, so the old one stays.", len(out.Subs), wantS)}
	}
	if len(src.Audio) > 0 && len(out.Audio) == 0 {
		return &VerifyProblem{"The new file has no sound and the old one has, so the old one stays."}
	}
	wantCovers := 0
	for _, st := range pl.Steps {
		if st.Kind == "cover" && st.Action == ActKeep {
			wantCovers++
		}
	}
	if len(out.Covers) < wantCovers {
		return &VerifyProblem{"The new file lost its cover picture, so the old one stays."}
	}
	if src.Chapters > 0 && out.Chapters == 0 {
		return &VerifyProblem{"The new file lost its chapters, so the old one stays."}
	}
	if !lengthOK(src.Duration, out.Duration) {
		// A file with broken times can say the wrong length. What's really in it decides.
		mctx, done := context.WithTimeout(ctx, 10*time.Minute)
		real := tools.MeasureDuration(mctx, src.Path)
		done()
		if real <= 0 || !lengthOK(real, out.Duration) {
			return &VerifyProblem{fmt.Sprintf("The new file is %s long and the old one is %s, so the old one stays.", clock(out.Duration), clock(src.Duration))}
		}
	}
	return decodes(ctx, tools, path, out.Duration, len(out.Audio) > 0)
}

// decodes plays a few seconds from the start, middle and end, really decoding them.
func decodes(ctx context.Context, tools *ffmpeg.Tools, path string, length float64, hasAudio bool) error {
	points := []float64{0}
	if length > 12 {
		points = append(points, length/2, math.Max(length-6, 0))
	}
	for _, at := range points {
		args := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-xerror", "-threads", "2", "-ss", strconv.FormatFloat(at, 'f', 3, 64)}
		args = append(args, ffmpeg.FileInput(path)...)
		args = append(args, "-t", "3", "-map", "0:v:0")
		if hasAudio {
			args = append(args, "-map", "0:a:0")
		}
		args = append(args, "-f", "null", "-")
		dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		_, _, err := ffmpeg.Run(dctx, tools.FFmpeg, args, "job", "check a converted file")
		cancel()
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return ctx.Err()
			}
			return &VerifyProblem{fmt.Sprintf("The new file wouldn't play from %s, so the old one stays.", clock(at))}
		}
	}
	return nil
}
