// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
)

// A disc's sections on its page (Design 2: the "discpage", "whytitle" and "discrefuse"
// boards): the feature with its scenes, the episodes of a TV disc, the film's other versions,
// the bonus material, shortcuts into the disc's own menus, why this title is the film, what
// was wrong with the copy, and the plain refusal of an encrypted disc. WittyDVD and WittyBluray
// declare them in their own sdui.json (sources and an insert into the title page); the server
// answers them, as it does WittyPlay's, because what they show is this server's: the disc's
// kept ranking, an administrator's choice and the names given to its titles. Each module's
// sources answer for its own kind of disc only, so on any other title the sections are left
// out. The player stays the app's own.

const (
	wittyDVDID    = "wittydvd"
	wittyBlurayID = "wittybluray"
)

// discSourceNames are each disc module's sources, after its "<module>.".
var discSourceNames = []string{"feature", "scenes", "episodes", "versions", "bonus", "menus", "why", "copy", "refusal"}

// discSources answers one of a disc module's sources. ok is false for any other source.
func (rv *sduiResolver) discSources(mod string, req sdui.SourceRequest) (sdui.SourceResult, bool, error) {
	name, found := strings.CutPrefix(req.Source.Type(), mod+".")
	if (mod != wittyDVDID && mod != wittyBlurayID) || !found || !slices.Contains(discSourceNames, name) {
		return sdui.SourceResult{}, false, nil
	}
	id, err := strconv.ParseInt(req.Source.Str("item"), 10, 64)
	if err != nil {
		return sdui.SourceResult{}, true, nil
	}
	it, err := rv.s.visibleItem(rv.r, id)
	// Each module answers for its own kind of disc: the other's sections are left out.
	if err != nil || it.Disc == nil || (mod == wittyBlurayID) != isBluray(it) || !isDiscKind(it) {
		return sdui.SourceResult{}, true, nil
	}
	var all []sdui.Item
	switch name {
	case "refusal":
		all = rv.discRefusal(it)
	case "why":
		all = rv.discWhy(it)
	case "copy":
		all = rv.discCopy(it)
	case "episodes":
		all = rv.discEpisodes(it)
	case "versions":
		all = rv.discVersions(it)
	default:
		if it.Disc.Scrambled {
			break // the refusal says why there is nothing to play
		}
		v, err := rv.s.discPage(rv.ctx, it)
		if errors.Is(err, errDiscEncrypted) {
			break
		}
		if err != nil {
			return sdui.SourceResult{}, true, err
		}
		switch name {
		case "feature":
			all = rv.discFeature(it, v)
		case "scenes":
			all = rv.discScenes(it, v)
		case "bonus":
			all = rv.discBonus(it, v)
		case "menus":
			all = rv.discMenus(it, v)
		}
	}
	if lim := req.Source.Int("limit"); lim > 0 && lim < req.Limit {
		req.Limit = lim
	}
	return page(all, req), true, nil
}

// unit is what a disc calls its titles.
func unitOf(it *library.Item) string {
	if isBluray(it) {
		return "Playlist"
	}
	return "Title"
}

// titleName is a title as a person reads it: its own name when it was given one, else
// "Title 3" or "Playlist 00800".
func titleName(it *library.Item, labels library.DiscLabels, ttn int) string {
	if l := strings.TrimSpace(labels.Titles[strconv.Itoa(ttn)]); l != "" {
		return l
	}
	if isBluray(it) {
		return fmt.Sprintf("Playlist %05d", ttn)
	}
	return fmt.Sprintf("Title %d", ttn)
}

// lengthWords is a running time as the boards write it: "1 hr 52 min", "9 min", "40 sec".
func lengthWords(sec float64) string {
	switch {
	case sec <= 0:
		return ""
	case sec < 60:
		return fmt.Sprintf("%d sec", int(math.Round(sec)))
	}
	m := int(math.Round(sec / 60))
	if h := m / 60; h > 0 {
		if m%60 == 0 {
			return fmt.Sprintf("%d hr", h)
		}
		return fmt.Sprintf("%d hr %d min", h, m%60)
	}
	return fmt.Sprintf("%d min", m)
}

func joinWords(parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

func playRoute(it *library.Item, q string) string {
	return fmt.Sprintf("/play/%d?mode=%s", it.ID, q)
}

// discFeature: Feature Presentation. Play the film, and how many scenes it has.
func (rv *sduiResolver) discFeature(it *library.Item, v *discPageView) []sdui.Item {
	if v.Main == nil || rv.noPlay {
		return nil
	}
	m := v.Main
	length := m.DurationSec
	if f := it.Disc.FeaturePlays(m.TTN); f != nil && f.DurationSec > 0 {
		length = f.DurationSec
	}
	out := []sdui.Item{{ID: "play", Kind: "view", Title: "Play the film", Subtitle: lengthWords(length), Icon: "play", Route: playRoute(it, "title")}}
	if n := len(m.Chapters); n > 1 {
		out = append(out, sdui.Item{ID: "scenes", Kind: "view", Title: "Scenes", Subtitle: fmt.Sprintf("%d chapters", n), Icon: "list",
			Route: playRoute(it, fmt.Sprintf("title&ttn=%d&chapter=1", m.TTN))})
	}
	return out
}

// discScenes: the film's chapters as pictures, each playing from its start.
func (rv *sduiResolver) discScenes(it *library.Item, v *discPageView) []sdui.Item {
	if v.Main == nil || len(v.Main.Chapters) < 2 || rv.noPlay {
		return nil
	}
	m := v.Main
	out := make([]sdui.Item, 0, len(m.Chapters))
	for _, c := range m.Chapters {
		name := strings.TrimSpace(c.Label)
		if name == "" {
			name = fmt.Sprintf("Scene %d", c.N)
		}
		out = append(out, sdui.Item{ID: fmt.Sprintf("scene-%d", c.N), Kind: "view", Title: name, Subtitle: fmtClock(c.Start),
			Route:  playRoute(it, fmt.Sprintf("title&ttn=%d&chapter=%d", m.TTN, c.N)),
			Images: map[string]string{"backdrop": fmt.Sprintf("/api/items/%d/disc/thumb/%d/%d", it.ID, m.TTN, c.N)}})
	}
	return out
}

// fmtClock is a place in a film: "1:04:00", "4:00".
func fmtClock(sec float64) string {
	t := int(sec)
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t/60%60, t%60)
	}
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

// discBonus: the other titles worth watching, longest first, each played once.
func (rv *sduiResolver) discBonus(it *library.Item, v *discPageView) []sdui.Item {
	if rv.noPlay {
		return nil
	}
	out := make([]sdui.Item, 0, len(v.Extras))
	for i, x := range v.Extras {
		name := strings.TrimSpace(x.Label)
		if name == "" {
			name = fmt.Sprintf("Bonus feature %d", i+1)
		}
		out = append(out, sdui.Item{ID: fmt.Sprintf("bonus-%d", x.TTN), Kind: "view", Title: name, Icon: "video",
			Subtitle: joinWords(lengthWords(x.DurationSec), fmt.Sprintf("%s %d", strings.ToLower(unitOf(it)), x.TTN)),
			Route:    playRoute(it, fmt.Sprintf("title&ttn=%d", x.TTN))})
	}
	return out
}

// discMenus: shortcuts straight into the disc's own menus.
func (rv *sduiResolver) discMenus(it *library.Item, v *discPageView) []sdui.Item {
	if rv.noPlay {
		return nil
	}
	out := make([]sdui.Item, 0, len(v.Menus))
	for _, m := range v.Menus {
		q := "menu&menu=" + m.Kind
		if m.VTS > 0 {
			q += fmt.Sprintf("&menuVts=%d", m.VTS)
		}
		out = append(out, sdui.Item{ID: "menu-" + m.Kind, Kind: "view", Title: m.Label, Icon: "disc", Route: playRoute(it, q)})
	}
	return out
}

// discEpisodes: a TV disc's episodes in viewing order.
func (rv *sduiResolver) discEpisodes(it *library.Item) []sdui.Item {
	rk := it.Disc.Rank
	if rk == nil || it.Disc.Scrambled || rv.noPlay {
		return nil
	}
	out := make([]sdui.Item, 0, len(rk.Episodes))
	for _, e := range rk.Episodes {
		where := fmt.Sprintf("%s %d", strings.ToLower(unitOf(it)), e.TTN)
		if e.Part {
			where = fmt.Sprintf("from %s of %s", fmtClock(e.StartSec), where)
		}
		out = append(out, sdui.Item{ID: fmt.Sprintf("episode-%d", e.Index), Kind: "view", Title: fmt.Sprintf("Episode %d", e.Index), Icon: "show",
			Subtitle: joinWords(lengthWords(e.DurationSec), where), Route: playRoute(it, fmt.Sprintf("title&episode=%d", e.Index))})
	}
	return out
}

// discVersions: every other way to see the film than what Play plays now. With a chosen
// title, the engine's own pick is one of them.
func (rv *sduiResolver) discVersions(it *library.Item) []sdui.Item {
	rk := it.Disc.Rank
	if rk == nil || it.Disc.Scrambled || rv.noPlay {
		return nil
	}
	main := it.Disc.MainTitle
	var out []sdui.Item
	for i, v := range rk.Versions {
		if v.Kind == "angle" && v.Main && v.TTN == main || v.Kind != "angle" && v.TTN == main {
			continue
		}
		label := strings.TrimSpace(v.Label)
		if label == "" {
			label = fmt.Sprintf("%s %d", unitOf(it), v.TTN)
		}
		label = strings.ToUpper(label[:1]) + label[1:]
		q := fmt.Sprintf("title&ttn=%d", v.TTN)
		if v.Kind == "angle" && v.Angle > 0 {
			q += fmt.Sprintf("&angle=%d", v.Angle)
		}
		out = append(out, sdui.Item{ID: fmt.Sprintf("version-%d", i), Kind: "view", Title: label, Icon: "movie", Subtitle: lengthWords(v.DurationSec), Route: playRoute(it, q)})
	}
	return out
}

// classWords say what the engine took a title for, as the board writes it.
var classWords = map[string]string{
	"main": "The film", "part": "Part of the film", "alternate": "Another cut", "featurette": "A featurette", "trailer": "A trailer",
	"intro": "An intro", "episode": "An episode", "playAll": "Plays every episode", "duplicate": "A copy of another title", "menu": "A menu's background",
	"extra": "An extra", "short": "A short", "stub": "Too short to matter",
}

var levelWords = map[string]string{"high": "High", "medium": "Medium", "low": "Low"}

// discWhy: why this title is the film, how sure the engine is, and the other candidates,
// each to play once. With a module from before rankings there is nothing to say.
func (rv *sduiResolver) discWhy(it *library.Item) []sdui.Item {
	d := it.Disc
	rk := d.Rank
	if rk == nil {
		return nil
	}
	labels := rv.s.Store.DiscLabels(rv.ctx, it.ID)
	sure := fmt.Sprintf("%d%% sure", int(math.Round(rk.Confidence*100)))
	head := sdui.Item{ID: "why", Kind: "view", Icon: "info"}
	if d.Chosen != nil {
		head.Title = titleName(it, labels, d.MainTitle) + " was chosen as the film"
		head.Subtitle = "An administrator chose it."
		if rk.Main >= 0 && rk.Main != d.MainTitle {
			head.Subtitle = fmt.Sprintf("An administrator chose it; the engine picks %s (%s).", titleName(it, labels, rk.Main), sure)
		}
		head.Badges = []string{"Chosen"}
	} else {
		head.Title = titleName(it, labels, rk.Main) + " is the film"
		if rk.Main < 0 {
			head.Title = "No title looks like the film"
		}
		head.Subtitle = joinWords(rk.Why, sure)
		if w := levelWords[rk.Level]; w != "" {
			head.Badges = []string{w}
		}
	}
	admin := rv.u.IsAdmin
	if admin && d.Chosen != nil {
		head.Subtitle = joinWords(head.Subtitle, "Choose to use the engine's pick again.")
		head.Action = &sdui.Action{Type: sdui.ActCommand, Op: "disc.clearChoice", Item: strconv.FormatInt(it.ID, 10)}
	}
	out := []sdui.Item{head}
	for _, c := range rk.Candidates {
		badge := "Extra"
		switch {
		case c.TTN == d.MainTitle:
			badge = "Film"
			if d.Chosen != nil {
				badge = "Chosen"
			}
		case c.TTN == rk.Main:
			badge = "Pick"
		case c.Class == "part":
			badge = "Part"
		case c.Class == "alternate" || isVersion(rk, c.TTN):
			badge = "Cut"
		case c.Class == "duplicate":
			badge = "Copy"
		}
		chapters := ""
		if c.Chapters > 0 {
			chapters = fmt.Sprintf("%d chapters", c.Chapters)
		}
		length := c.DurationSec
		if f := d.FeaturePlays(c.TTN); f != nil && f.DurationSec > 0 {
			length = f.DurationSec // the film as it plays, through every title it runs through
		}
		row := sdui.Item{ID: fmt.Sprintf("candidate-%d", c.TTN), Kind: "view", Title: joinWords(titleName(it, labels, c.TTN), lengthWords(length)),
			Subtitle: joinWords(classWords[c.Class], chapters), Badges: []string{badge}}
		if c.TTN == rk.Main && c.TTN != d.MainTitle {
			row.Subtitle = joinWords("The engine's pick", chapters)
		}
		switch {
		case admin && c.TTN != d.MainTitle && !d.Scrambled && badge != "Part":
			// A part of the film is no film to choose.
			// An administrator makes it the film, for everyone; others play it once.
			row.Subtitle = joinWords("Choose to make it the film", row.Subtitle)
			row.Action = &sdui.Action{Type: sdui.ActCommand, Op: "disc.choose", Item: strconv.FormatInt(it.ID, 10), Params: map[string]any{"ttn": c.TTN}}
		case !d.Scrambled && !rv.noPlay:
			row.Route = playRoute(it, fmt.Sprintf("title&ttn=%d", c.TTN))
		}
		out = append(out, row)
	}
	if admin {
		out = append(out, sdui.Item{ID: "rank", Kind: "view", Title: "Rank again", Subtitle: "Work out which " + strings.ToLower(unitOf(it)) + " is the film again", Icon: "sparkles",
			Action: &sdui.Action{Type: sdui.ActCommand, Op: "disc.rank", Item: strconv.FormatInt(it.ID, 10)}})
		if isBluray(it) && !d.Scrambled {
			if st := rv.s.secondOpinionState(rv.ctx); st.Enabled && st.Available {
				out = append(out, sdui.Item{ID: "compare", Kind: "view", Title: "Compare with libbluray", Subtitle: joinWords("libbluray "+st.Version, "on this server"), Icon: "check",
					Action: &sdui.Action{Type: sdui.ActCommand, Op: "disc.compare", Item: strconv.FormatInt(it.ID, 10)}})
			}
		}
	}
	if rk.Decoys > 0 {
		out = append(out, sdui.Item{ID: "decoys", Kind: "view", Title: fmt.Sprintf("%d decoy %ss left out", rk.Decoys, strings.ToLower(unitOf(it))), Icon: "warning", Badges: []string{"Decoy"},
			Subtitle: "They repeat or scramble the film's scenes, or point at video that isn't there."})
	}
	return out
}

// discCopy: what was wrong with this copy of the disc and what was mended, and for an
// administrator the way to check it.
func (rv *sduiResolver) discCopy(it *library.Item) []sdui.Item {
	var out []sdui.Item
	if rk := it.Disc.Rank; rk != nil {
		for i, r := range rk.Repairs {
			out = append(out, sdui.Item{ID: fmt.Sprintf("repair-%d", i), Kind: "view", Title: r, Icon: "check", Badges: []string{"Mended"}})
		}
		if more := rk.RepairCount - len(rk.Repairs); more > 0 {
			out = append(out, sdui.Item{ID: "repairs-more", Kind: "view", Title: fmt.Sprintf("%d more mended", more), Icon: "check"})
		}
		for i, x := range rk.Issues {
			badge := "Note"
			if x.Severity == "error" {
				badge = "Won't play"
			}
			title := x.What
			if x.TTN >= 0 {
				title = joinWords(x.What, fmt.Sprintf("%s %d", strings.ToLower(unitOf(it)), x.TTN))
			}
			out = append(out, sdui.Item{ID: fmt.Sprintf("issue-%d", i), Kind: "view", Title: title, Icon: "warning", Badges: []string{badge}})
		}
		if rk.PaddedSectors > 0 {
			out = append(out, sdui.Item{ID: "padded", Kind: "view", Icon: "warning", Badges: []string{"Unreadable"},
				Title: fmt.Sprintf("%d unreadable sectors were read as blank", rk.PaddedSectors), Subtitle: "While reading the disc's index. The film may skip there."})
		}
		if so := rk.SecondOpinion; so != nil {
			row := sdui.Item{ID: "second-opinion", Kind: "view", Icon: "info"}
			switch {
			case so.Note != "":
				row.Title, row.Subtitle = "No second opinion", so.Note
			case so.Agrees:
				row.Title, row.Badges = so.Source+" agrees", []string{"Agrees"}
			default:
				row.Title, row.Badges = so.Source+" reads it differently", []string{"Differs"}
				row.Subtitle = strings.Join(so.Differences, "; ")
			}
			out = append(out, row)
		}
	}
	if rv.u.IsAdmin {
		out = append(out, sdui.Item{ID: "diagnose", Kind: "view", Title: "Check this disc", Subtitle: "Diagnose…", Icon: "settings", Route: fmt.Sprintf("/settings/diagnoser?item=%d", it.ID)})
	}
	return out
}

// discRefusal: an encrypted disc gets a plain message: what happened, why, and what is
// still known about it.
func (rv *sduiResolver) discRefusal(it *library.Item) []sdui.Item {
	if !it.Disc.Scrambled {
		return nil
	}
	name := strings.TrimSpace(it.Disc.Label)
	if name == "" {
		name = it.Title
	}
	out := []sdui.Item{
		{ID: "refused", Kind: "view", Icon: "warning", Title: "This disc can't be played", Badges: []string{"Encrypted"},
			Subtitle: "It is copy-protected. Nothing was changed and nothing was copied: nothing here unlocks copy protection."},
		{ID: "disc", Kind: "view", Icon: "disc", Title: joinWords("Disc", name), Subtitle: "Result: Encrypted"},
	}
	if rk := it.Disc.Rank; rk != nil && rk.Main >= 0 {
		out = append(out, sdui.Item{ID: "film", Kind: "view", Icon: "info", Title: joinWords(titleName(it, library.DiscLabels{}, rk.Main)+" looks like the film", lengthWords(it.RuntimeSec)),
			Subtitle: "Read from the disc's index, which is never encrypted."})
	}
	return out
}
