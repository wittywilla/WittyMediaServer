package diagnose

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittydvd"
	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// Limits on pressing every button of every menu, on a DVD or a Blu-ray.
const (
	maxScreens     = 300
	maxSettleCells = 400
	maxDeadEnds    = 50
	maxCrashes     = 3
	crawlBudget    = 10 * time.Minute
	longTitleSec   = 600 // past this, a title is the film: the walk stops there
)

const encryptedAdvice = "Encrypted discs aren't supported, and nothing here can change that. A disc without copy protection will play."

func checkDVD(ctx context.Context, j *Job, src *discSource, o DiscOptions) {
	j.Step("Reading the DVD's index files", 0.05)
	var d *wittydvd.Opened
	var err error
	j.Engine("reading the DVD's index files", func() { dvdIndexFiles(j, src) })
	if !j.Engine("opening the DVD", func() { d, err = wittydvd.Open(src.Path) }) {
		return
	}
	if err != nil {
		j.Add(Finding{Severity: SevProblem, Title: "The DVD can't be opened", What: "The engine couldn't get as far as the disc's main index. Without it nothing on the disc can be found.",
			Fix: "If the damage scan was on, it follows below. Otherwise make a new copy of the disc.", Details: []string{"The engine said: " + plainErr(err)}})
		if o.Damage && ctx.Err() == nil {
			_, _, from := barSpans(o)
			j.Engine("scanning for damage", func() { scanDamage(ctx, j, src, damageOptions{from: from, packs: true}) })
		}
		return
	}
	defer d.Close()
	if src.Label == "" {
		j.SetLabel(d.Label)
	}
	j.Add(Finding{Severity: SevOK, Title: "This is a DVD", What: fmt.Sprintf("%s, %s.", kindSentence(src), bytesText(src.Size))})

	j.Step("Checking whether the video is scrambled", 0.1)
	var menusScrambled, scrambled bool
	j.Engine("checking whether the video is scrambled", func() { scrambled, menusScrambled = dvdEncryption(ctx, j, d) })
	if ctx.Err() != nil {
		return
	}

	j.Step("Reading the titles and menus", 0.18)
	var titles []wittydvd.TitleSummary
	var main *wittydvd.TitleSummary
	j.Engine("reading the titles and menus", func() {
		titles = d.Disc.Titles()
		main = d.Disc.MainTitle()
		dvdStructure(j, d, titles, main)
	})

	from, to, dmgFrom := barSpans(o)
	if o.Crawl {
		switch {
		case menusScrambled:
			j.Add(Finding{Severity: SevInfo, Title: "The menu buttons weren't pressed", What: "The menus themselves are scrambled, so they can't be read."})
		default:
			j.Step("Pressing every menu button", from)
			dvdCrawl(ctx, j, d, from, to)
		}
	}
	if o.Damage && ctx.Err() == nil {
		where := dvdWhere(src, titles, main)
		j.Engine("scanning for damage", func() {
			scanDamage(ctx, j, src, damageOptions{from: dmgFrom, where: where, packs: true, scrambled: scrambled})
		})
	}
}

func kindSentence(src *discSource) string {
	if src.IsDir {
		return "A folder"
	}
	return "A " + formatName(src.Format) + " image"
}

// dvdIndexFiles checks each IFO and its backup by itself: the engine quietly
// uses whichever works, and quietly drops a title set when neither does. It says
// which file it was.
func dvdIndexFiles(j *Job, src *discSource) {
	type result struct {
		ifo, bup error
	}
	check := func(base string, parse func([]byte) error) result {
		var r result
		for _, ext := range []string{".IFO", ".BUP"} {
			b, err := src.read("VIDEO_TS/"+base+ext, wittydvd.MaxIFOBytes)
			if err == nil {
				err = parse(b)
			}
			if ext == ".IFO" {
				r.ifo = err
			} else {
				r.bup = err
			}
		}
		return r
	}
	var vmg *wittydvd.VMG
	parseVMG := func(b []byte) error {
		v, err := wittydvd.ParseVMG(b)
		if err == nil && vmg == nil {
			vmg = v
		}
		return err
	}
	var bad, backupOnly, noBackup []string
	seen := func(name string, r result) {
		switch {
		case r.ifo != nil && r.bup != nil:
			bad = append(bad, fmt.Sprintf("%s.IFO and its backup: %s", name, plainErr(r.ifo)))
		case r.ifo != nil:
			backupOnly = append(backupOnly, fmt.Sprintf("%s.IFO: %s", name, plainErr(r.ifo)))
		case r.bup != nil:
			noBackup = append(noBackup, name+".BUP")
		}
	}
	seen("VIDEO_TS", check("VIDEO_TS", parseVMG))
	count := 0
	if vmg != nil {
		count = vmg.NumTitleSets
		for _, t := range vmg.Titles {
			count = max(count, t.VTS)
		}
	}
	for n := 1; n <= min(count, 99); n++ {
		base := fmt.Sprintf("VTS_%02d_0", n)
		seen(base, check(base, func(b []byte) error { _, err := wittydvd.ParseVTS(b, n); return err }))
	}
	if len(bad) > 0 {
		list, more := clip(bad, len(bad), 50)
		j.Add(Finding{Severity: SevProblem, Title: "Index files of this DVD are damaged", What: "Neither these files nor their backups can be read. What they describe won't play, and if the first one is on the list, nothing will.",
			Fix: "Make a new copy of the disc. If the disc itself is scratched, clean it first.", Details: list, More: more})
	}
	if len(backupOnly) > 0 {
		list, more := clip(backupOnly, len(backupOnly), 50)
		j.Add(Finding{Severity: SevWarn, Title: "Index files are damaged, but their backups work", What: "The disc plays from its backup copies, as a player would. It's a sign the copy or the disc isn't in full health.",
			Fix: "It should play. A new copy, or the damage scan, will show whether more is wrong.", Details: list, More: more})
	}
	if len(noBackup) > 0 {
		list, more := clip(noBackup, len(noBackup), 50)
		j.Add(Finding{Severity: SevInfo, Title: "Some backup index files are missing or unreadable", What: "The main copies are fine, so this does no harm by itself. Copies of discs sometimes leave the backups out.", Details: list, More: more})
	}
}

// packRange is a stretch of a VOB space, in sectors.
type packRange struct{ first, last uint32 }

// packTally counts what samples of a stream found.
type packTally struct{ scrambled, total int }

func (t *packTally) add(o packTally) { t.scrambled += o.scrambled; t.total += o.total }

// percent words a tally for people.
func (t packTally) String() string {
	if t.total == 0 {
		return "no video packs found to sample"
	}
	return fmt.Sprintf("%d of %d sampled packs scrambled (%.0f%%)", t.scrambled, t.total, 100*float64(t.scrambled)/float64(t.total))
}

// packHoldsStream says a pack carries sound or picture, the only packs that can
// be scrambled: navigation and padding never are, and don't count either way.
func packHoldsStream(pack []byte) bool {
	if len(pack) < 20 || binary.BigEndian.Uint32(pack) != 0x000001ba || pack[4]&0xc0 != 0x40 {
		return false
	}
	p := 14 + int(pack[13]&7)
	for p+9 <= len(pack) {
		if pack[p] != 0 || pack[p+1] != 0 || pack[p+2] != 1 {
			return false
		}
		id := pack[p+3]
		if id == 0xbb || id == 0xbe || id == 0xbf {
			p += 6 + int(binary.BigEndian.Uint16(pack[p+4:]))
			continue
		}
		return true
	}
	return false
}

// samplePacks looks at a few packs from evenly spaced places across some stretches
// of a VOB space. This looks only at the scrambling flag: nothing is decrypted.
func samplePacks(ctx context.Context, j *Job, rd wittydvd.SectorReader, ranges []packRange, points int) packTally {
	const perPoint = 8
	var total int64
	for _, r := range ranges {
		if r.last >= r.first {
			total += int64(r.last-r.first) + 1
		}
	}
	var t packTally
	if total == 0 {
		return t
	}
	for i := range points {
		at := total * int64(i) / int64(points)
		var sector uint32
		for _, r := range ranges {
			if r.last < r.first {
				continue
			}
			n := int64(r.last-r.first) + 1
			if at < n {
				sector = r.first + uint32(at)
				break
			}
			at -= n
		}
		start := time.Now()
		buf, err := rd.ReadSectors(sector, perPoint)
		if err != nil && len(buf) == 0 {
			continue
		}
		for p := 0; p+wittydvd.Sector <= len(buf); p += wittydvd.Sector {
			pack := buf[p : p+wittydvd.Sector]
			if !packHoldsStream(pack) {
				continue
			}
			t.total++
			if wittydvd.PackScrambled(pack) {
				t.scrambled++
			}
		}
		if !j.Rest(ctx, time.Since(start)) {
			return t
		}
	}
	return t
}

// dvdEncryption samples the main title, the rest and the menus: some discs are
// scrambled only in part, so a sample of one place wouldn't do.
func dvdEncryption(ctx context.Context, j *Job, d *wittydvd.Opened) (scrambled, menus bool) {
	sizes := func(sp wittydvd.Space) uint32 {
		ext, err := d.Extents(sp)
		if err != nil {
			return 0
		}
		var n int64
		for _, e := range ext {
			n += e.Length
		}
		return uint32(n / wittydvd.Sector)
	}
	var mainT, other, menu packTally
	mainVTS := 0
	if m := d.Disc.MainTitle(); m != nil {
		mainVTS = m.VTS
		sp := wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: m.VTS}
		if rd, err := d.Reader(sp); err == nil {
			limit := sizes(sp)
			var ranges []packRange
			seen := map[packRange]bool{}
			for _, c := range m.Cells {
				r := packRange{c.Cell.FirstSector, c.Cell.LastSector}
				if seen[r] || r.first >= limit || r.last < r.first {
					continue
				}
				seen[r] = true
				if r.last >= limit {
					r.last = limit - 1
				}
				ranges = append(ranges, r)
			}
			mainT = samplePacks(ctx, j, rd, ranges, 32)
		}
	}
	whole := func(sp wittydvd.Space, points int, into *packTally) {
		n := sizes(sp)
		if n == 0 || ctx.Err() != nil {
			return
		}
		rd, err := d.Reader(sp)
		if err != nil {
			return
		}
		into.add(samplePacks(ctx, j, rd, []packRange{{0, n - 1}}, points))
	}
	whole(wittydvd.Space{Domain: wittydvd.DomainVMGM}, 6, &menu)
	for n := 1; n <= 99; n++ {
		if d.Disc.VTS[n] == nil {
			continue
		}
		whole(wittydvd.Space{Domain: wittydvd.DomainVTSM, VTS: n}, 6, &menu)
		if n != mainVTS {
			whole(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: n}, 8, &other)
		} else if mainT.total == 0 {
			whole(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: n}, 32, &mainT)
		}
	}
	all := mainT
	all.add(other)
	all.add(menu)
	lines := []string{"Main title: " + mainT.String(), "Other titles: " + other.String(), "Menus: " + menu.String()}
	switch {
	case ctx.Err() != nil:
	case all.total == 0:
		j.Add(Finding{Severity: SevWarn, Title: "No video could be sampled", What: "No readable sound or picture packs were found where the disc says they are. The video files may be missing, or the copy may be incomplete.", Details: lines})
	case all.scrambled == 0:
		j.Add(Finding{Severity: SevOK, Title: "The video isn't scrambled", What: "Samples from across the disc are all clear, so this disc can play.", Details: lines})
	default:
		part := "The whole disc looks scrambled."
		if all.scrambled*10 < all.total*9 {
			part = "Only part of it is, which is how some discs are made. The clear part may start and then fail where the scrambling begins."
		}
		j.Add(Finding{Severity: SevProblem, Title: "This DVD is scrambled",
			What: fmt.Sprintf("%.0f%% of the sound and picture sampled is CSS-scrambled. %s The server never decrypts anything, so scrambled video won't play here.", 100*float64(all.scrambled)/float64(all.total), part),
			Fix:  encryptedAdvice, Details: lines})
	}
	return all.scrambled > 0, menu.scrambled > 0
}

func countPGCs(units []wittydvd.LangUnit) int {
	n := 0
	for _, lu := range units {
		n += len(lu.PGCs)
	}
	return n
}

func regionText(mask int) string {
	if mask == 0 {
		return "any region"
	}
	var allowed []string
	for r := 1; r <= 8; r++ {
		if mask&(1<<(r-1)) == 0 {
			allowed = append(allowed, strconv.Itoa(r))
		}
	}
	if len(allowed) == 0 {
		return "no region at all, which is usually a damaged index"
	}
	return "region " + strings.Join(allowed, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

// dvdStructure writes down what the disc holds: only counts and numbers.
func dvdStructure(j *Job, d *wittydvd.Opened, titles []wittydvd.TitleSummary, main *wittydvd.TitleSummary) {
	menus := countPGCs(d.Disc.VMG.Menus)
	sets := 0
	for n := 1; n <= 99; n++ {
		if v := d.Disc.VTS[n]; v != nil {
			sets++
			menus += countPGCs(v.Menus)
		}
	}
	det := []string{
		plural(sets, "title set", "title sets") + ", " + plural(len(titles), "title", "titles") + ", " + plural(menus, "menu screen program", "menu screen programs"),
		"Plays in " + regionText(d.Disc.VMG.RegionMask),
	}
	mainNo := 0
	if main != nil {
		mainNo = main.Number
		det = append(det, fmt.Sprintf("Main title (best guess): title %d, %s", main.Number, hms(main.Duration)))
	}
	if d.Disc.VMG.FirstPlay == nil {
		det = append(det, "The disc has no first-play program")
	}
	shown := 0
	for _, t := range titles {
		if shown >= 40 {
			break
		}
		shown++
		std := strings.ToUpper(t.Video.Standard)
		shape := "4:3"
		if t.Video.Wide {
			shape = "16:9"
		}
		mark := ""
		if t.Number == mainNo {
			mark = " (main)"
		}
		det = append(det, fmt.Sprintf("Title %d%s: %s, %s, %s, %s, %s, %s %s", t.Number, mark, hms(t.Duration), plural(len(t.Chapters), "chapter", "chapters"), plural(t.Angles, "angle", "angles"), plural(len(t.Audio), "audio track", "audio tracks"), plural(len(t.Subp), "subtitle track", "subtitle tracks"), std, shape))
	}
	what := "What the disc's index says it holds."
	if len(titles) > 40 {
		what = "The first 40 titles are listed. A disc with very many titles often has decoys, added as copy protection: the main title guess skips them."
	}
	j.Add(Finding{Severity: SevInfo, Title: "What's on this DVD", What: what, Details: det, More: len(titles) - shown})

	if len(titles) == 0 {
		j.Add(Finding{Severity: SevProblem, Title: "The DVD lists no titles", What: "The index was read, but it names nothing to play.", Fix: "Make a new copy of the disc."})
	}
	// Files the index needs. A cell past the end of the video files is either a
	// decoy or a copy that stopped early; the main title makes the difference.
	var missing []string
	past, pastMain := 0, 0
	for n := 1; n <= 99; n++ {
		v := d.Disc.VTS[n]
		if v == nil {
			continue
		}
		if ext, err := d.Extents(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: n}); err == nil && len(ext) == 0 && len(v.PGCs) > 0 {
			missing = append(missing, fmt.Sprintf("VTS_%02d_1.VOB", n))
		}
		if ext, err := d.Extents(wittydvd.Space{Domain: wittydvd.DomainVTSM, VTS: n}); err == nil && len(ext) == 0 && countPGCs(v.Menus) > 0 {
			missing = append(missing, fmt.Sprintf("VTS_%02d_0.VOB", n))
		}
	}
	for _, t := range titles {
		ext, err := d.Extents(wittydvd.Space{Domain: wittydvd.DomainVTS, VTS: t.VTS})
		if err != nil {
			continue
		}
		var size int64
		for _, e := range ext {
			size += e.Length
		}
		for _, c := range t.Cells {
			if int64(c.Cell.LastSector)*wittydvd.Sector >= size {
				past++
				if t.Number == mainNo {
					pastMain++
				}
				break
			}
		}
	}
	if len(missing) > 0 {
		list, more := clip(missing, len(missing), 50)
		j.Add(Finding{Severity: SevProblem, Title: "Video files of this DVD are missing", What: "The index names video that isn't there. What it holds won't play.", Fix: "Make a new copy of the disc, with every file of the VIDEO_TS folder.", Details: list, More: more})
	}
	switch {
	case pastMain > 0:
		j.Add(Finding{Severity: SevProblem, Title: "The main title runs past the end of the video files", What: "The copy looks incomplete: the index expects more video than there is.", Fix: "Make a new copy of the disc."})
	case past > 0:
		j.Add(Finding{Severity: SevInfo, Title: "Some titles point past the end of the video files", What: plural(past, "title does", "titles do") + " this, though not the main one. Decoy titles added as copy protection do this. The ones with real video are fine."})
	}
}

// slowReader is a sector reader that stops when the job does, and rests when the
// server is busy.
type slowReader struct {
	ctx context.Context
	j   *Job
	rd  wittydvd.SectorReader
}

func (r *slowReader) ReadSectors(first uint32, count int) ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	b, err := r.rd.ReadSectors(first, count)
	r.j.Rest(r.ctx, time.Since(start))
	return b, err
}

type dvdButton struct {
	n     int
	cmd   wittydvd.Command
	text  string
	inert bool
}

// dvdMenu is what a crawl keeps of a menu: its buttons, and none of its pictures.
type dvdMenu struct {
	buttons   []dvdButton
	scrambled bool
}

type dvdMenuKey struct {
	sp          wittydvd.Space
	first, last uint32
	subp        wittydvd.SubpControl
	hasSubp     bool
}

type dvdCrawler struct {
	ctx      context.Context
	j        *Job
	d        *wittydvd.Opened
	menus    map[dvdMenuKey]*dvdMenu
	pressed  int
	crashes  int
	deadEnds deadEndList
}

// menu reads a cell's buttons, once per cell. The engine's own cache keeps
// pictures, which a crawl of hundreds of screens has no use for.
func (c *dvdCrawler) menu(nav *wittydvd.Nav, s *wittydvd.Step) (*dvdMenu, error) {
	k := dvdMenuKey{sp: wittydvd.SpaceFor(s.Domain, s.VTS), first: s.FirstSector, last: s.LastSector}
	var video wittydvd.VideoAttr
	switch s.Domain {
	case wittydvd.DomainVTSM, wittydvd.DomainVTS:
		v := c.d.Disc.VTS[s.VTS]
		if v == nil {
			return nil, errors.New("a title set this cell belongs to is missing")
		}
		video = v.Video
		if s.Domain == wittydvd.DomainVTSM {
			video = v.MenuVideo
		}
	default:
		video = c.d.Disc.VMG.MenuVideo
	}
	var subc *wittydvd.SubpControl
	if pgc := nav.StepPGC(s); pgc != nil {
		if subc = pgc.SubpControl[0]; subc != nil {
			k.subp, k.hasSubp = *subc, true
		}
	}
	if m, ok := c.menus[k]; ok {
		return m, nil
	}
	rd, err := c.d.Reader(k.sp)
	if err != nil {
		return nil, err
	}
	pic, err := wittydvd.ScanMenu(&slowReader{c.ctx, c.j, rd}, s.FirstSector, s.LastSector, wittydvd.MenuScanOptions{Palette: s.Palette, Video: video, SubpControl: subc, Wide: true, NavButton: s.DefaultButton, MaxHops: 2000})
	if err != nil {
		return nil, err
	}
	if pic == nil { // a cell past the end of the video reads as nothing at all
		pic = &wittydvd.MenuPicture{}
	}
	m := &dvdMenu{scrambled: pic.Scrambled}
	for _, b := range pic.Buttons {
		m.buttons = append(m.buttons, dvdButton{n: b.N, cmd: b.Cmd, text: b.CmdText, inert: b.Inert})
	}
	c.menus[k] = m
	return m, nil
}

// settled is where pressing something ended up.
type settled struct {
	step *wittydvd.Step
	menu *dvdMenu
	// bad words a dead end; fine says where it went if it's no menu and no
	// problem (the film started).
	bad, fine string
}

// settle runs through intros, clips and timed stills until the viewer has
// something to press, the disc stops, or a long title starts.
func (c *dvdCrawler) settle(nav *wittydvd.Nav, s *wittydvd.Step) settled {
	for hop := 0; hop < maxSettleCells; hop++ {
		if c.ctx.Err() != nil {
			return settled{step: s, fine: "the check stopped"}
		}
		if s == nil {
			return settled{bad: "the engine gave no next step"}
		}
		if s.Kind == wittydvd.StepStop {
			why := "the disc stopped"
			if s.Reason != "" {
				why += ": " + s.Reason
			}
			return settled{step: s, bad: why}
		}
		m, err := c.menu(nav, s)
		if err == nil && m.scrambled {
			return settled{step: s, bad: "it reached scrambled video"}
		}
		if err == nil && len(m.buttons) > 0 {
			return settled{step: s, menu: m}
		}
		if s.Kind == wittydvd.StepTitle && s.Title != nil && s.Title.Duration > longTitleSec {
			return settled{step: s, fine: fmt.Sprintf("it plays title %d (%s)", s.Title.TTN, hms(s.Title.Duration))}
		}
		if math.IsInf(s.StillSec, 1) {
			return settled{step: s, bad: "it holds on a picture for ever, with no buttons to press"}
		}
		s = nav.CellEnded()
	}
	return settled{step: s, bad: fmt.Sprintf("it gave up after %d cells without reaching anything to press", maxSettleCells)}
}

func pathWords(p []int) string {
	if len(p) == 0 {
		return "Starting the disc"
	}
	parts := make([]string, len(p))
	for i, n := range p {
		parts[i] = strconv.Itoa(n)
	}
	return "Buttons " + strings.Join(parts, " > ")
}

// deadEndList keeps the different dead ends a crawl finds. One broken button can
// be reached along many paths, and the first path says all there is to say.
type deadEndList struct {
	index  map[string]int
	lines  []string
	counts []int
	total  int
}

func (l *deadEndList) add(key, line string) {
	if i, ok := l.index[key]; ok {
		l.counts[i]++
		return
	}
	l.total++
	if len(l.lines) >= maxDeadEnds {
		return
	}
	if l.index == nil {
		l.index = map[string]int{}
	}
	l.index[key] = len(l.lines)
	l.lines = append(l.lines, line)
	l.counts = append(l.counts, 0)
}

// report words the list for a finding.
func (l *deadEndList) report() []string {
	out := make([]string, len(l.lines))
	for i, line := range l.lines {
		out[i] = line
		if n := l.counts[i]; n > 0 {
			out[i] += fmt.Sprintf(" (%s to the same end)", plural(n, "other path leads", "other paths lead"))
		}
	}
	return out
}

// press presses one button on a copy of the navigator. A crash on one button
// doesn't end the crawl: it's noted, and after a few the crawl gives up.
func (c *dvdCrawler) press(nav *wittydvd.Nav, b dvdButton) (st settled, next *wittydvd.Nav) {
	defer func() {
		if p := recover(); p != nil {
			logging.ReportPanic("WittyDiagnoser", p, "cat", "discs", "session", c.j.ID, "step", "pressing a DVD menu button")
			c.crashes++
			st = settled{bad: "the engine crashed: " + fmt.Sprint(p)}
		}
	}()
	next = nav.Clone()
	cmd := b.cmd
	c.pressed++
	return c.settle(next, next.PressButton(b.n, &cmd)), next
}

type dvdScreen struct {
	nav  *wittydvd.Nav
	menu *dvdMenu
	path []int
}

// dvdCrawl presses every button of every menu it can reach, breadth first, and
// says where each ends. The disc's random numbers come from a fixed seed, so two
// runs agree.
func dvdCrawl(parent context.Context, j *Job, d *wittydvd.Opened, from, to float64) {
	ctx, cancel := context.WithTimeout(parent, crawlBudget)
	defer cancel()
	c := &dvdCrawler{ctx: ctx, j: j, d: d, menus: map[dvdMenuKey]*dvdMenu{}}
	screens, unvisited := 0, 0
	first := ""
	ok := j.Engine("pressing DVD menu buttons", func() {
		rnd := rand.New(rand.NewPCG(1, 2))
		nav := wittydvd.NewNav(d.Disc, wittydvd.NavOptions{RegisterOptions: wittydvd.RegisterOptions{Rand: rnd.Float64}})
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
		key := func(s *wittydvd.Step) string { return fmt.Sprintf("%s/%d/%d/%d", s.DomainName, s.VTS, s.PGCN, s.CellN) }
		seen := map[string]bool{key(start.step): true}
		queue := []dvdScreen{{nav: nav, menu: start.menu}}
		for len(queue) > 0 && screens < maxScreens && ctx.Err() == nil && c.crashes < maxCrashes {
			cur := queue[0]
			queue = queue[1:]
			screens++
			j.Progress(from + (to-from)*float64(screens)/float64(screens+len(queue)+1))
			for _, b := range cur.menu.buttons {
				if b.inert {
					continue
				}
				if ctx.Err() != nil || c.crashes >= maxCrashes {
					break
				}
				path := append(append([]int{}, cur.path...), b.n)
				st, nx := c.press(cur.nav, b)
				switch {
				case ctx.Err() != nil:
				case st.menu != nil:
					if k := key(st.step); !seen[k] {
						seen[k] = true
						queue = append(queue, dvdScreen{nav: nx, menu: st.menu, path: path})
					}
				case st.bad != "":
					c.deadEnds.add(b.text+st.bad, fmt.Sprintf("%s (%s): %s", pathWords(path), b.text, st.bad))
				}
			}
		}
		unvisited = len(queue)
	})
	if parent.Err() != nil {
		return
	}
	reportCrawl(j, "DVD", ok, c.crashes, screens, c.pressed, unvisited, first, &c.deadEnds, ctx.Err() != nil)
}

// reportCrawl words what a crawl found, for either kind of disc.
func reportCrawl(j *Job, kind string, ok bool, crashes, screens, pressed, unvisited int, first string, dead *deadEndList, outOfTime bool) {
	if crashes > 0 {
		j.Add(Finding{Severity: SevProblem, Title: "The engine crashed on this disc", What: "The server's disc engine crashed while pressing a menu button. That's a bug in the server, and is itself a useful result: the same button would probably crash the player's session.",
			Fix: "Please send this report (it holds no disc content) with a bug report."})
	}
	summary := []string{plural(screens, "menu screen", "menu screens") + " visited, " + plural(pressed, "button", "buttons") + " pressed"}
	if !ok {
		return
	}
	total := dead.total
	if screens == 0 && total == 0 {
		j.Add(Finding{Severity: SevOK, Title: "This disc has no menu to press", What: "On starting, " + first + ".", Details: summary})
		return
	}
	all := unvisited == 0 && !outOfTime
	if !all {
		why := "the limit of " + strconv.Itoa(maxScreens) + " screens"
		if outOfTime {
			why = "the time limit of " + crawlBudget.String()
		}
		j.Add(Finding{Severity: SevInfo, Title: "Not every menu was visited", What: "The check stopped at " + why + ", with " + plural(unvisited, "screen", "screens") + " still to see. Discs with games or many setup menus can go on for a long way.", Details: summary})
	}
	if total == 0 {
		title := "Every menu button leads somewhere"
		if !all {
			title = "Every menu button tried leads somewhere"
		}
		j.Add(Finding{Severity: SevOK, Title: title, What: "Every button pressed led to another menu or to something playing.", Details: summary})
		return
	}
	j.Add(Finding{Severity: SevWarn, Title: plural(total, "menu button leads nowhere", "menu buttons lead nowhere"),
		What:    "These paths end with nothing to do. A disc can do this on purpose (a button that ends the disc), but it's also how a problem in the engine or the copy shows up.",
		Fix:     "If one of these is a button you need, send this report (it holds no disc content) with a bug report. " + kind + " buttons are numbered by the disc.",
		Details: append(summary, dead.report()...), More: total - len(dead.lines)})
}

var vobNameRe = regexp.MustCompile(`(?i)^VTS_(\d\d)_(\d)\.VOB$`)

// dvdWhere words which title or menu a place in a VOB belongs to.
func dvdWhere(src *discSource, titles []wittydvd.TitleSummary, main *wittydvd.TitleSummary) func(name string, off int64) string {
	mainNo := 0
	if main != nil {
		mainNo = main.Number
	}
	return func(name string, off int64) string {
		base := path.Base(name)
		if strings.EqualFold(base, "VIDEO_TS.VOB") {
			return "the main menu"
		}
		m := vobNameRe.FindStringSubmatch(base)
		if m == nil {
			return ""
		}
		vts, _ := strconv.Atoi(m[1])
		k, _ := strconv.Atoi(m[2])
		if k == 0 {
			return fmt.Sprintf("the menus of title set %d", vts)
		}
		// A title's sectors count through the numbered files, one after another.
		for n := 1; n < k; n++ {
			f := src.file(path.Join(path.Dir(name), fmt.Sprintf("VTS_%02d_%d.VOB", vts, n)))
			if f != nil {
				off += f.Size
			}
		}
		sector := uint32(off / wittydvd.Sector)
		var hit []string
		for _, t := range titles {
			if t.VTS != vts {
				continue
			}
			for _, c := range t.Cells {
				if sector >= c.Cell.FirstSector && sector <= c.Cell.LastSector {
					if t.Number == mainNo {
						return "the main title"
					}
					if len(hit) < 3 {
						hit = append(hit, strconv.Itoa(t.Number))
					}
					break
				}
			}
		}
		if len(hit) == 0 {
			return "not used by any title"
		}
		return "title " + strings.Join(hit, ", ")
	}
}
