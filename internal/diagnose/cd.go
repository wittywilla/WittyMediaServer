package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/wittywilla/wittycd"
)

// checkCD checks an audio CD image, and says whether this was one. A sheet or a
// container that holds no audio goes on to the checks for other discs, as does a
// file that isn't there, so that it's reported once and the same way.
func checkCD(ctx context.Context, j *Job, path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	var d *wittycd.Disc
	var err error
	if !j.Engine("reading the CD's table of contents", func() { d, err = wittycd.Open(path) }) {
		return true
	}
	if errors.Is(err, wittycd.ErrNotCD) || errors.Is(err, wittycd.ErrNoAudio) {
		return false
	}
	if err != nil {
		j.Add(Finding{Severity: SevProblem, Title: "This CD image can't be read", What: err.Error(),
			Fix: "A cue sheet needs the audio file it names beside it, with the same name. Otherwise, make the image again from the disc."})
		return true
	}
	defer d.Close()
	j.SetLabel("Audio CD")
	j.Step("Reading every track", 0.05)
	r := d.Check(func(done float64) bool {
		j.Progress(0.05 + done*0.9)
		return ctx.Err() == nil
	})
	if ctx.Err() != nil {
		return true
	}
	j.Add(Finding{Severity: SevOK, Title: "This is an audio CD image",
		What: fmt.Sprintf("%d audio tracks, %s long, in %s form. It belongs in a Music library.", len(d.AudioTracks()), hms(r.Facts.DurationSec), r.Facts.Format)})
	for _, f := range r.Findings {
		sev := SevOK
		switch f.Severity {
		case wittycd.SeverityNote:
			sev = SevInfo
		case wittycd.SeverityProblem:
			sev = SevProblem
		}
		title := "The disc"
		if f.Track > 0 {
			title = fmt.Sprintf("Track %d", f.Track)
		}
		j.Add(Finding{Severity: sev, Title: title, What: f.Text})
	}
	return true
}
