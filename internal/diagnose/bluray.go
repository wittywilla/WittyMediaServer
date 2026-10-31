package diagnose

import (
	"context"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/bdj"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

const (
	maxJarBytes = 64 << 20 // a JAR bigger than this isn't read
	maxJars     = 32
	// An IG stream's first display set comes early in its clip; this is how far
	// to look, in packets, before deciding it isn't there.
	menuScanPackets = 1 << 20
)

func checkBluray(ctx context.Context, j *Job, src *discSource, o DiscOptions) {
	j.Step("Reading the Blu-ray's index files", 0.05)
	var d *wittybluray.Disc
	var err error
	if !j.Engine("opening the Blu-ray", func() { d, err = wittybluray.Open(src.Path) }) {
		return
	}
	if err != nil {
		j.Add(Finding{Severity: SevProblem, Title: "The Blu-ray can't be opened", What: "The engine couldn't find anything to play: no usable index and no playlists.",
			Fix: "Make a new copy of the disc, with its whole BDMV folder.", Details: []string{"The engine said: " + plainErr(err)}})
		if o.Damage && ctx.Err() == nil {
			_, _, from := barSpans(o)
			j.Engine("scanning for damage", func() { scanDamage(ctx, j, src, damageOptions{from: from, ts: true}) })
		}
		return
	}
	defer d.Close()
	if src.Label == "" {
		j.SetLabel(d.Label)
	}
	j.Add(Finding{Severity: SevOK, Title: "This is a Blu-ray", What: fmt.Sprintf("%s, %s.", kindSentence(src), bytesText(src.Size))})

	j.Step("Checking whether the video is encrypted", 0.1)
	j.Engine("checking whether the video is encrypted", func() { blurayEncryption(j, d) })

	j.Step("Reading the playlists and clips", 0.14)
	var main *wittybluray.Playlist
	j.Engine("reading the playlists and clips", func() {
		main = d.MainPlaylist()
		blurayDamaged(j, d)
		blurayStructure(j, d, main)
		blurayClips(j, d, main)
	})

	if d.BDJ {
		j.Step("Looking at the disc's Java programs", 0.17)
		j.Engine("looking at the disc's Java programs", func() { blurayBDJ(ctx, j, d) })
	}
	if ctx.Err() != nil {
		return
	}

	from, to, dmgFrom := barSpans(o)
	if o.Crawl {
		switch {
		case d.Encrypted:
			j.Add(Finding{Severity: SevInfo, Title: "The menu buttons weren't pressed", What: "The disc's streams are encrypted, and its menus are in those streams, so they can't be read."})
		default:
			j.Step("Pressing every menu button", from)
			blurayCrawl(ctx, j, d, from, to)
		}
	}
	if o.Damage && ctx.Err() == nil {
		where := blurayWhere(d, main)
		j.Engine("scanning for damage", func() {
			scanDamage(ctx, j, src, damageOptions{from: dmgFrom, where: where, ts: true, scrambled: d.Encrypted})
		})
	}
}

func blurayEncryption(j *Job, d *wittybluray.Disc) {
	switch {
	case d.Encrypted:
		j.Add(Finding{Severity: SevProblem, Title: "This Blu-ray is encrypted", What: "Its streams are protected with AACS. The server never decrypts anything, so it won't play here.", Fix: decryptAdvice})
	case d.AACSFolder:
		j.Add(Finding{Severity: SevOK, Title: "The video isn't encrypted", What: "The disc still has an AACS folder, as decrypted copies usually do, but samples of the streams are clear."})
	default:
		j.Add(Finding{Severity: SevOK, Title: "The video isn't encrypted", What: "Samples of the streams are clear, and the disc has no AACS folder."})
	}
	if d.BDPlus {
		j.Add(Finding{Severity: SevWarn, Title: "This Blu-ray has a BD+ program", What: "The server doesn't run BD+. On a copy that wasn't fully converted, parts of the picture can be damaged on purpose, and only the disc's own program would repair them.", Fix: "If the picture breaks up, " + strings.ToLower(decryptAdvice[:1]) + decryptAdvice[1:]})
	}
}

func blurayDamaged(j *Job, d *wittybluray.Disc) {
	if len(d.Damaged) == 0 {
		return
	}
	indexGone := false
	for _, n := range d.Damaged {
		up := strings.ToUpper(n)
		if strings.HasPrefix(up, "INDEX") || strings.HasPrefix(up, "MOVIEOBJ") {
			indexGone = true
		}
	}
	list, more := clip(d.Damaged, len(d.Damaged), 50)
	if indexGone {
		j.Add(Finding{Severity: SevProblem, Title: "The disc's program files are damaged", What: "Neither these files nor their backups could be read. Without its program the disc has no menus, though its playlists may still play.", Fix: "Make a new copy of the disc.", Details: list, More: more})
		return
	}
	j.Add(Finding{Severity: SevWarn, Title: "Some playlists or clip files are damaged", What: "Neither these files nor their backups could be read, and the rest of the disc works without them. What they describe won't play.", Fix: "Make a new copy of the disc if what's missing matters to you.", Details: list, More: more})
}

func chapters(pl *wittybluray.Playlist) int {
	n := 0
	for _, m := range pl.Marks {
		if m.Type == 1 {
			n++
		}
	}
	return n
}

func objectWords(o wittybluray.IndexObject) string {
	switch o.Kind {
	case wittybluray.ObjectHDMV:
		if o.Interactive {
			return "HDMV, with a menu"
		}
		return "HDMV"
	case wittybluray.ObjectBDJ:
		return "BD-J (Java)"
	}
	return "nothing"
}

var videoFormats = map[int]string{1: "480i", 2: "576i", 3: "480p", 4: "1080i", 5: "720p", 6: "1080p", 7: "576p", 8: "2160p"}

// blurayStructure writes down what the disc holds: only counts and numbers.
func blurayStructure(j *Job, d *wittybluray.Disc, main *wittybluray.Playlist) {
	det := []string{
		plural(len(d.Playlists), "playlist", "playlists") + ", " + plural(len(d.Clips), "clip", "clips") + ", " + plural(len(d.MovieObjects), "movie object", "movie objects"),
	}
	if d.Index != nil {
		hdmv, java := 0, 0
		for _, t := range d.Index.Titles {
			switch t.Object.Kind {
			case wittybluray.ObjectHDMV:
				hdmv++
			case wittybluray.ObjectBDJ:
				java++
			}
		}
		det = append(det,
			"First play: "+objectWords(d.Index.FirstPlay),
			"Top menu: "+objectWords(d.Index.TopMenu),
			fmt.Sprintf("Titles: %d HDMV, %d BD-J", hdmv, java))
		if d.Index.Version >= "0300" {
			det = append(det, "Ultra HD Blu-ray format")
		}
	}
	if d.Stereo3D {
		det = append(det, "Has 3D (it plays here as 2D)")
	}
	if d.Recording {
		det = append(det, "A recording rather than a pressed disc: each clip plays by itself")
	}
	if main != nil && len(main.Items) > 0 {
		it := &main.Items[0]
		det = append(det, fmt.Sprintf("Main feature (best guess): playlist %05d, %s, %s, %s", main.Number, hms(d.PlaylistDuration(main)), plural(chapters(main), "chapter", "chapters"), plural(len(main.Items), "clip", "clips")))
		line := fmt.Sprintf("%s, %s", plural(len(it.Audio), "audio track", "audio tracks"), plural(len(it.PG), "subtitle track", "subtitle tracks"))
		if len(it.Angles) > 1 {
			line += ", " + plural(len(it.Angles), "angle", "angles")
		}
		det = append(det, line)
		for _, v := range it.Video {
			s := "Video: " + v.CodingName()
			if f := videoFormats[v.Format]; f != "" {
				s += " " + f
			}
			switch v.DynamicRange {
			case 1:
				s += ", HDR10"
			case 2:
				s += ", Dolby Vision"
			}
			if v.HDR10Plus {
				s += ", HDR10+"
			}
			det = append(det, s)
		}
		if len(it.DV) > 0 {
			det = append(det, "Has a Dolby Vision layer")
		}
	}
	// The longest few: enough to see what the disc is like. A disc with hundreds
	// of playlists usually has decoys.
	nums := make([]int, 0, len(d.Playlists))
	for n := range d.Playlists {
		nums = append(nums, n)
	}
	slices.SortFunc(nums, func(a, b int) int {
		x, y := d.PlaylistDuration(d.Playlists[a]), d.PlaylistDuration(d.Playlists[b])
		switch {
		case x > y:
			return -1
		case x < y:
			return 1
		}
		return a - b
	})
	shown := 0
	for _, n := range nums {
		if shown >= 12 {
			break
		}
		shown++
		pl := d.Playlists[n]
		det = append(det, fmt.Sprintf("Playlist %05d: %s, %s, %s", n, hms(d.PlaylistDuration(pl)), plural(len(pl.Items), "clip", "clips"), plural(chapters(pl), "chapter", "chapters")))
	}
	what := "What the disc's index says it holds. The longest playlists are listed."
	if len(d.Playlists) > 100 {
		what += " A disc with this many playlists often has decoys, added as copy protection: the main feature guess skips them."
	}
	j.Add(Finding{Severity: SevInfo, Title: "What's on this Blu-ray", What: what, Details: det, More: len(nums) - shown})
	if main == nil {
		j.Add(Finding{Severity: SevWarn, Title: "No main feature can be told apart", What: "No playlist on this disc reads as a whole film: each one names a clip that's missing, or runs past a clip's end."})
	}
}

// blurayClips compares each clip's stream file with what its information says.
func blurayClips(j *Job, d *wittybluray.Disc, main *wittybluray.Playlist) {
	inMain := map[string]bool{}
	if main != nil {
		for i := range main.Items {
			inMain[main.Items[i].Clip] = true
		}
	}
	var missing, short []string
	missingMain := 0
	names := make([]string, 0, len(d.Clips))
	for n := range d.Clips {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		c := d.Clips[n]
		ext, err := d.ClipExtents(n)
		if err != nil || len(ext) == 0 {
			missing = append(missing, n)
			if inMain[n] {
				missingMain++
			}
			continue
		}
		var size int64
		for _, e := range ext {
			size += e.Length
		}
		if want := c.Size(); want > 0 && size < want {
			short = append(short, fmt.Sprintf("%s: %s of %s", n, bytesText(size), bytesText(want)))
			if inMain[n] {
				missingMain++
			}
		}
	}
	if len(missing) > 0 {
		sev := SevWarn
		if missingMain > 0 {
			sev = SevProblem
		}
		list, more := clip(missing, len(missing), 50)
		j.Add(Finding{Severity: sev, Title: "Video files of this Blu-ray are missing", What: "The disc describes these clips, but their stream files aren't there.", Fix: "Make a new copy of the disc, with its whole BDMV folder.", Details: list, More: more})
	}
	if len(short) > 0 {
		sev := SevWarn
		if missingMain > 0 {
			sev = SevProblem
		}
		list, more := clip(short, len(short), 50)
		j.Add(Finding{Severity: sev, Title: "Some video files are shorter than they should be", What: "These stream files hold less than the disc's own information says. The copy may have stopped early.", Fix: "Make a new copy of the disc.", Details: list, More: more})
	}
}

// safeJarName accepts the plain names discs use: a name from a table on the disc
// must not lead out of its folder.
func safeJarName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == '-':
		default:
			return false
		}
	}
	return true
}

// blurayBDJ reports a disc's Java programs from their files alone. The disc's
// code is never run here: only the names its classes refer to are read.
func blurayBDJ(ctx context.Context, j *Job, d *wittybluray.Disc) {
	seen := map[string]bool{}
	names := []string{}
	add := func(o wittybluray.IndexObject) {
		if o.Kind == wittybluray.ObjectBDJ && !seen[o.BDJName] {
			seen[o.BDJName] = true
			names = append(names, o.BDJName)
		}
	}
	add(d.Index.FirstPlay)
	add(d.Index.TopMenu)
	for _, t := range d.Index.Titles {
		add(t.Object)
	}
	slices.Sort(names)
	jars := map[string]bool{}
	apps := 0
	var bad []string
	for _, n := range names {
		o, err := d.BDJObject(n)
		if err != nil {
			bad = append(bad, n)
			continue
		}
		apps += len(o.Apps)
		for _, a := range o.Apps {
			if safeJarName(a.BaseDir) {
				jars[a.BaseDir] = true
			}
		}
		for _, c := range o.Cache {
			if !c.Dir && safeJarName(c.Name) {
				jars[c.Name] = true
			}
		}
	}
	jarNames := make([]string, 0, len(jars))
	for k := range jars {
		jarNames = append(jarNames, k)
	}
	slices.Sort(jarNames)
	classes := 0
	missing := map[string]int{}
	var unread []string
	skipped := 0
	for i, jar := range jarNames {
		if ctx.Err() != nil {
			return
		}
		if i >= maxJars {
			skipped = len(jarNames) - i
			break
		}
		start := time.Now()
		b, err := d.ReadProgramFile("JAR/"+jar+".jar", maxJarBytes+1)
		switch {
		case err != nil:
			unread = append(unread, jar+".jar: can't be read")
			continue
		case len(b) > maxJarBytes:
			unread = append(unread, jar+".jar: over the "+bytesText(maxJarBytes)+" this check reads")
			continue
		}
		m, n, err := bdj.MissingClasses(b)
		if err != nil {
			unread = append(unread, jar+".jar: "+plainErr(err))
			continue
		}
		classes += n
		for k, v := range m {
			missing[k] += len(v)
		}
		if !j.Rest(ctx, time.Since(start)) {
			return
		}
	}
	det := []string{
		plural(len(names), "BD-J object", "BD-J objects") + ", " + plural(apps, "application", "applications") + ", " + plural(len(jarNames), "JAR file", "JAR files") + ", " + plural(classes, "class", "classes"),
	}
	j.Add(Finding{Severity: SevInfo, Title: "This Blu-ray uses Java (BD-J) menus", What: "Part of what this disc shows comes from Java programs on it. This check only reads what's in them: it never runs the disc's code.", Details: det})
	on := j.env.setting("bdjEnabled", "0") == "1"
	java, jerr := bdj.Find(j.env.setting("javaPath", ""))
	switch {
	case !on:
		what := "It's off here, so the disc plays from its playlists, without its Java menus."
		if jerr != nil {
			what += " Java wasn't found on this machine either."
		}
		j.Add(Finding{Severity: SevInfo, Title: "BD-J is turned off", What: what, Fix: "To try the disc's own menus, install Java and turn BD-J on under Settings, Playback & discs. It's off by default because it runs the disc's code."})
	case jerr != nil || java == nil:
		j.Add(Finding{Severity: SevWarn, Title: "BD-J is on, but Java wasn't found", What: "Without Java the disc plays from its playlists, without its Java menus.", Fix: "Install any Java 8 or newer, or set where it is under Settings, Playback & discs."})
	default:
		j.Add(Finding{Severity: SevOK, Title: "BD-J is on and Java was found", What: "This disc's menus can be tried. Retail discs are largely untested, so yours may not work yet."})
	}
	if len(bad) > 0 {
		list, more := clip(bad, len(bad), 50)
		j.Add(Finding{Severity: SevWarn, Title: "Some BD-J objects are damaged", What: "Neither these objects nor their backups can be read. The titles they belong to can't start their Java programs.", Details: list, More: more})
	}
	if len(unread) > 0 || skipped > 0 {
		list, more := clip(unread, len(unread), 50)
		what := "These weren't looked into."
		if skipped > 0 {
			what = fmt.Sprintf("These weren't looked into, and neither were the last %d: a disc rarely has so many.", skipped)
		}
		j.Add(Finding{Severity: SevWarn, Title: "Some JAR files weren't read", What: what, Details: list, More: more})
	}
	if len(missing) > 0 {
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		lines := make([]string, 0, 50)
		for _, k := range keys {
			if len(lines) >= 50 {
				break
			}
			lines = append(lines, fmt.Sprintf("%s (named by %s)", k, plural(missing[k], "class", "classes")))
		}
		j.Add(Finding{Severity: SevInfo, Title: "This disc's code names platform parts the server's runtime lacks",
			What:    "A disc fails only if it actually reaches that code, so this doesn't have to mean trouble. It's what a report about a BD-J disc needs.",
			Details: lines, More: len(keys) - len(lines)})
	} else if classes > 0 {
		j.Add(Finding{Severity: SevOK, Title: "The server's runtime has every platform part this disc's code names"})
	}
}

// What a crawl keeps of a Blu-ray menu: buttons and their commands, no pictures.
type igButton struct {
	id   int
	cmds []wittybluray.Command
}

type igGroup struct {
	defaultValid int
	buttons      []igButton
}

type igPage struct {
	id     int
	groups []igGroup
}

// visible follows what Page.Visible does in the engine.
func (p *igPage) visible(enabled, disabled map[int]bool) []igButton {
	var out []igButton
	for _, g := range p.groups {
		pick := -1
		for i, b := range g.buttons {
			if enabled[b.id] && !disabled[b.id] {
				pick = i
				break
			}
		}
		if pick < 0 {
			for i, b := range g.buttons {
				if b.id == g.defaultValid && !disabled[b.id] {
					pick = i
				}
			}
		}
		if pick >= 0 {
			out = append(out, g.buttons[pick])
		}
	}
	return out
}

type igMenu struct {
	pages []igPage
}

func (m *igMenu) page(id int) *igPage {
	for i := range m.pages {
		if m.pages[i].id == id {
			return &m.pages[i]
		}
	}
	return nil
}

type igKey struct {
	clip string
	pid  uint16
}

type igEntry struct {
	menu *igMenu
	err  error
}

type bdCrawler struct {
	ctx      context.Context
	j        *Job
	d        *wittybluray.Disc
	menus    map[igKey]igEntry
	pressed  int
	crashes  int
	deadEnds deadEndList
	bdjSeen  int
}

// menu loads a step's menu once, keeping only its buttons: pictures of a whole
// menu can run to many megabytes.
func (c *bdCrawler) menu(st *wittybluray.Step) (*igMenu, error) {
	if !st.Menu || st.Playlist == nil || st.Item >= len(st.Playlist.Items) {
		return nil, nil
	}
	clipName := st.IGClip
	if clipName == "" {
		clipName = st.Playlist.Items[st.Item].Clip
	}
	k := igKey{clipName, st.IGPID}
	if e, ok := c.menus[k]; ok {
		return e.menu, e.err
	}
	start := time.Now()
	m, err := c.d.LoadMenu(clipName, st.IGPID, menuScanPackets)
	c.j.Rest(c.ctx, time.Since(start))
	if err != nil {
		c.menus[k] = igEntry{err: err}
		return nil, err
	}
	out := &igMenu{}
	for i := range m.Pages {
		p := &m.Pages[i]
		np := igPage{id: p.ID}
		for gi := range p.BOGs {
			g := &p.BOGs[gi]
			ng := igGroup{defaultValid: g.DefaultValid}
			for bi := range g.Buttons {
				ng.buttons = append(ng.buttons, igButton{id: g.Buttons[bi].ID, cmds: g.Buttons[bi].Commands})
			}
			np.groups = append(np.groups, ng)
		}
		out.pages = append(out.pages, np)
	}
	c.menus[k] = igEntry{menu: out}
	return out, nil
}

type bdScreen struct {
	nav      *wittybluray.Nav
	step     *wittybluray.Step
	menu     *igMenu
	page     int
	enabled  map[int]bool
	disabled map[int]bool
	path     []int
}

type bdSettled struct {
	step      *wittybluray.Step
	menu      *igMenu
	bad, fine string
}

// settle runs on through whatever plays by itself until the viewer has a menu,
// the disc stops or hands over to Java, or something long starts.
func (c *bdCrawler) settle(nav *wittybluray.Nav, st *wittybluray.Step) bdSettled {
	for hop := range maxSettleCells / 4 {
		_ = hop
		if c.ctx.Err() != nil {
			return bdSettled{fine: "the check stopped"}
		}
		switch st.Kind {
		case wittybluray.StepStop:
			why := "the disc stopped"
			if st.Reason != "" {
				why += ": " + st.Reason
			}
			return bdSettled{step: st, bad: why}
		case wittybluray.StepUnsupported:
			c.bdjSeen++
			return bdSettled{step: st, fine: "it hands over to a Java program"}
		case wittybluray.StepStay:
			return bdSettled{step: st, fine: "the menu stays as it is"}
		case wittybluray.StepPlay:
		default:
			return bdSettled{step: st, bad: "the engine gave a step it can't name: " + string(st.Kind)}
		}
		if st.Playlist == nil {
			return bdSettled{step: st, bad: "the engine gave no playlist to play"}
		}
		long := c.d.PlaylistDuration(st.Playlist) > longTitleSec
		m, err := c.menu(st)
		if err != nil && !long {
			return bdSettled{step: st, bad: "its menu can't be read: " + plainErr(err)}
		}
		// A film's pop-up menu is a menu like any other: its buttons get pressed too.
		if err == nil && m != nil && len(m.pages) > 0 {
			return bdSettled{step: st, menu: m}
		}
		if long {
			return bdSettled{step: st, fine: fmt.Sprintf("it plays playlist %05d (%s)", st.Playlist.Number, hms(c.d.PlaylistDuration(st.Playlist)))}
		}
		if st.StillSec < 0 {
			return bdSettled{step: st, bad: "it holds on a picture for ever, with no buttons to press"}
		}
		st = nav.PlaylistEnded()
	}
	return bdSettled{step: st, bad: fmt.Sprintf("it gave up after %d playlists without reaching anything to press", maxSettleCells/4)}
}

func copySet(m map[int]bool) map[int]bool {
	c := make(map[int]bool, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// bdPress presses one button on a copy of the navigator.
func (c *bdCrawler) press(cur *bdScreen, b igButton) (out bdSettled, next *wittybluray.Nav) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("WittyDiagnoser", p, "cat", "discs", "session", c.j.ID, "step", "pressing a Blu-ray menu button")
			c.crashes++
			out = bdSettled{bad: "the engine crashed: " + fmt.Sprint(p)}
		}
	}()
	next = cur.nav.Clone()
	next.SetMenuState(cur.page, b.id)
	c.pressed++
	st := next.PressButton(b.cmds)
	if st != nil && st.Kind == wittybluray.StepStay {
		return bdSettled{step: st}, next // handled by the caller: a page or a button changed
	}
	return c.settle(next, st), next
}

// firstPage is the page a menu opens on: the disc's first.
func firstPage(m *igMenu) int {
	if len(m.pages) == 0 {
		return 0
	}
	return m.pages[0].id
}

// blurayCrawl presses every button of every page of every menu it can reach,
// breadth first. Discs whose menus are Java programs have nothing to press here.
func blurayCrawl(parent context.Context, j *Job, d *wittybluray.Disc, from, to float64) {
	ctx, cancel := context.WithTimeout(parent, crawlBudget)
	defer cancel()
	c := &bdCrawler{ctx: ctx, j: j, d: d, menus: map[igKey]igEntry{}}
	screens, unvisited := 0, 0
	first := ""
	ok := j.Engine("pressing Blu-ray menu buttons", func() {
		nav := wittybluray.NewNav(d, wittybluray.NavOptions{})
		start := c.settle(nav, nav.Start())
		if start.menu == nil {
			first = start.bad
			if first == "" {
				first = start.fine
			}
			if start.bad != "" {
				c.deadEnds.add(start.bad, pathWords(nil)+": "+start.bad)
			}
			return
		}
		key := func(st *wittybluray.Step, page int) string {
			if st.Playlist == nil {
				return strconv.Itoa(page)
			}
			return fmt.Sprintf("%d/%d", st.Playlist.Number, page)
		}
		sp := firstPage(start.menu)
		seen := map[string]bool{key(start.step, sp): true}
		queue := []bdScreen{{nav: nav, step: start.step, menu: start.menu, page: sp}}
		for len(queue) > 0 && screens < maxScreens && ctx.Err() == nil && c.crashes < maxCrashes {
			cur := queue[0]
			queue = queue[1:]
			screens++
			j.Progress(from + (to-from)*float64(screens)/float64(screens+len(queue)+1))
			page := cur.menu.page(cur.page)
			if page == nil {
				why := fmt.Sprintf("it asks for page %d, which the menu doesn't have", cur.page)
				c.deadEnds.add(why, pathWords(cur.path)+": "+why)
				continue
			}
			for _, b := range page.visible(cur.enabled, cur.disabled) {
				if len(b.cmds) == 0 {
					continue
				}
				if ctx.Err() != nil || c.crashes >= maxCrashes {
					break
				}
				path := append(append([]int{}, cur.path...), b.id)
				st, nx := c.press(&cur, b)
				switch {
				case ctx.Err() != nil:
				case st.bad != "":
					c.deadEnds.add(st.bad, pathWords(path)+": "+st.bad)
				case st.step != nil && st.step.Kind == wittybluray.StepStay:
					// The same menu, maybe on another page or with other buttons.
					next := bdScreen{nav: nx, step: cur.step, menu: cur.menu, page: cur.page, enabled: copySet(cur.enabled), disabled: copySet(cur.disabled), path: path}
					changed := false
					if fx := st.step.Effects; fx != nil {
						if fx.ButtonPage != nil && fx.ButtonPage.SetPage {
							next.page = int(fx.ButtonPage.Page)
							changed = true
						}
						for _, id := range fx.EnableButton {
							next.enabled[int(id)] = true
							delete(next.disabled, int(id))
							changed = true
						}
						for _, id := range fx.DisableButton {
							next.disabled[int(id)] = true
							delete(next.enabled, int(id))
							changed = true
						}
					}
					k := key(cur.step, next.page) + buttonStateKey(next.enabled, next.disabled)
					if changed && !seen[k] {
						seen[k] = true
						queue = append(queue, next)
					}
				case st.menu != nil:
					pg := firstPage(st.menu)
					if k := key(st.step, pg); !seen[k] {
						seen[k] = true
						queue = append(queue, bdScreen{nav: nx, step: st.step, menu: st.menu, page: pg, path: path})
					}
				}
			}
		}
		unvisited = len(queue)
	})
	if parent.Err() != nil {
		return
	}
	reportCrawl(j, "Blu-ray", ok, c.crashes, screens, c.pressed, unvisited, first, &c.deadEnds, ctx.Err() != nil)
	if c.bdjSeen > 0 && screens > 0 {
		j.Add(Finding{Severity: SevInfo, Title: "Some menu buttons hand over to Java programs", What: plural(c.bdjSeen, "button does", "buttons do") + ". What those programs do can't be seen without running them, and this check doesn't."})
	}
}

// buttonStateKey tells apart screens that differ only in which buttons are on.
func buttonStateKey(enabled, disabled map[int]bool) string {
	if len(enabled) == 0 && len(disabled) == 0 {
		return ""
	}
	var on, off []int
	for k := range enabled {
		on = append(on, k)
	}
	for k := range disabled {
		off = append(off, k)
	}
	slices.Sort(on)
	slices.Sort(off)
	return fmt.Sprintf("+%v-%v", on, off)
}

// blurayWhere words which playlists a place in a stream file belongs to.
func blurayWhere(d *wittybluray.Disc, main *wittybluray.Playlist) func(name string, off int64) string {
	return func(name string, _ int64) string {
		base := path.Base(name)
		clipName := strings.TrimSuffix(base, path.Ext(base))
		if !strings.EqualFold(path.Ext(base), ".m2ts") && !strings.EqualFold(path.Ext(base), ".mts") && !strings.EqualFold(path.Ext(base), ".m2t") {
			return ""
		}
		if main != nil {
			for i := range main.Items {
				if main.Items[i].Clip == clipName {
					return "the main feature"
				}
			}
		}
		n := 0
		for _, pl := range d.Playlists {
			for i := range pl.Items {
				if pl.Items[i].Clip == clipName {
					n++
					break
				}
			}
		}
		if n == 0 {
			return "not used by any playlist"
		}
		return "in " + plural(n, "playlist", "playlists")
	}
}
