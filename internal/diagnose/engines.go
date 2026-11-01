package diagnose

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/wittywilla/wittymodule"
)

// The parts of a disc check that understand the disc run in its module, in a helper, and report
// as they go. Reading every byte for damage stays here, since any disc is files and sectors.

const (
	// moduleCheckLimit is how long a helper may spend on a check: it presses every button
	// of every menu, and a crawl has its own budget inside this.
	moduleCheckLimit = 30 * time.Minute
	maxModuleFinding = 8 << 10
	maxModuleLines   = 64
	maxModuleReports = 2000
)

// ModuleHelper is a helper for one job: the part of a modules.Attached a check uses.
type ModuleHelper interface {
	Call(method string, params, out any) ([]byte, error)
	OnNotify(fn func(method string, params json.RawMessage))
	Close()
}

// heard takes what a helper reports. It's untrusted like anything from a module: only words,
// kept to a sensible size, which go through the same scrubbing as the server's own.
func heard(j *Job, method string, raw json.RawMessage, count *int) {
	if *count++; *count > maxModuleReports {
		return
	}
	switch method {
	case wittymodule.NotifyDiagStep:
		var s wittymodule.DiagStep
		if json.Unmarshal(raw, &s) == nil && s.Fraction >= 0 && s.Fraction <= 1 {
			j.Step(clipText(s.Phase), s.Fraction)
		}
	case wittymodule.NotifyDiagProgress:
		var f float64
		if json.Unmarshal(raw, &f) == nil && f >= 0 && f <= 1 {
			j.Progress(f)
		}
	case wittymodule.NotifyDiagLabel:
		var l string
		if json.Unmarshal(raw, &l) == nil {
			j.SetLabel(clipText(l))
		}
	case wittymodule.NotifyDiagFinding:
		var f wittymodule.DiagFinding
		if json.Unmarshal(raw, &f) != nil {
			return
		}
		sev := SevOK
		switch f.Severity {
		case wittymodule.DiagInfo:
			sev = SevInfo
		case wittymodule.DiagWarn:
			sev = SevWarn
		case wittymodule.DiagProblem:
			sev = SevProblem
		}
		details, more := clip(f.Details, len(f.Details), maxModuleLines)
		names, moreNames := clip(f.Names, len(f.Names), maxModuleLines)
		j.Add(Finding{Severity: sev, Title: clipText(f.Title), What: clipText(f.What), Fix: clipText(f.Fix), Details: clipAll(details), Names: clipAll(names), More: max(f.More, 0) + more + moreNames})
	}
}

func clipText(s string) string {
	if len(s) > maxModuleFinding {
		return s[:maxModuleFinding]
	}
	return s
}

func clipAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = clipText(s)
	}
	return out
}

// checkWith has a disc's module check it, and then scans it for damage.
func checkWith(ctx context.Context, j *Job, id, name string, src *discSource, o DiscOptions, dmg damageOptions) {
	from, to, dmgFrom := barSpans(o)
	dmg.from = dmgFrom
	damage := func() {
		if o.Damage && ctx.Err() == nil {
			j.Engine("scanning for damage", func() { scanDamage(ctx, j, src, dmg) })
		}
	}
	var h ModuleHelper
	if j.env.Helper != nil {
		h = j.env.Helper(id, moduleCheckLimit)
	}
	if h == nil {
		j.Add(Finding{Severity: SevWarn, Title: name + " isn't on this server", What: "This disc's titles and menus can't be checked without it.",
			Fix: "Add " + name + " in Settings, Modules, and run this check again."})
		damage()
		return
	}
	defer h.Close()
	stop := context.AfterFunc(ctx, h.Close)
	defer stop()
	count := 0
	h.OnNotify(func(method string, raw json.RawMessage) { heard(j, method, raw, &count) })
	var r struct {
		Scrambled bool `json:"scrambled"`
	}
	_, err := h.Call("diagnose", struct {
		Path  string  `json:"path"`
		Crawl bool    `json:"crawl,omitempty"`
		From  float64 `json:"from,omitempty"`
		To    float64 `json:"to,omitempty"`
		BDJ   bool    `json:"bdj,omitempty"`
		Java  string  `json:"java,omitempty"`
	}{src.Path, o.Crawl, from, to, j.env.setting("bdjEnabled", "0") == "1", j.env.setting("javaPath", "")}, &r)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		j.Add(Finding{Severity: SevProblem, Title: name + " couldn't finish checking this disc", What: clipText(err.Error()),
			Fix: "If the damage scan was on, it follows below. Otherwise make a new copy of the disc."})
		damage()
		return
	}
	dmg.scrambled = r.Scrambled
	dmg.where = func(file string, off int64) string {
		var w struct {
			Where string `json:"where"`
		}
		if _, err := h.Call("where", struct {
			Name string `json:"name"`
			Off  int64  `json:"off"`
		}{file, off}, &w); err != nil {
			return ""
		}
		return clipText(w.Where)
	}
	damage()
}

func checkDVD(ctx context.Context, j *Job, src *discSource, o DiscOptions) {
	checkWith(ctx, j, "wittydvd", "WittyDVD", src, o, damageOptions{packs: true})
}

func checkBluray(ctx context.Context, j *Job, src *discSource, o DiscOptions) {
	checkWith(ctx, j, "wittybluray", "WittyBluray", src, o, damageOptions{ts: true})
}

// checkCD checks an audio CD image, and says whether this was one. A sheet or a container that
// holds no audio goes on to the checks for other discs, as does a file that isn't there, so
// that it's reported once and the same way.
func checkCD(ctx context.Context, j *Job, path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	if j.env.Helper == nil {
		return false
	}
	h := j.env.Helper("wittycd", moduleCheckLimit)
	if h == nil {
		j.Add(Finding{Severity: SevWarn, Title: "WittyCD isn't on this server", What: "A CD image can't be checked without it.",
			Fix: "Add WittyCD in Settings, Modules, and run this check again."})
		return true
	}
	defer h.Close()
	stop := context.AfterFunc(ctx, h.Close)
	defer stop()
	count := 0
	h.OnNotify(func(method string, raw json.RawMessage) { heard(j, method, raw, &count) })
	var ce *wittymodule.CallError
	if _, err := h.Call("open", struct {
		Path string `json:"path"`
	}{path}, nil); err != nil {
		if errors.As(err, &ce) && (ce.Code == "not-cd" || ce.Code == "no-audio") {
			return false
		}
		j.Add(Finding{Severity: SevProblem, Title: "This CD image can't be read", What: clipText(err.Error()),
			Fix: "A cue sheet needs the audio file it names beside it, with the same name. Otherwise, make the image again from the disc."})
		return true
	}
	if _, err := h.Call("diagnose", nil, nil); err != nil && ctx.Err() == nil {
		j.Add(Finding{Severity: SevProblem, Title: "WittyCD couldn't finish checking this image", What: clipText(err.Error())})
	}
	return true
}

// kindSentence words what a disc is, for the findings the server adds itself.
func kindSentence(src *discSource) string {
	if src.IsDir {
		return "A folder"
	}
	return "A " + formatName(src.Format) + " image"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}
