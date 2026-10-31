package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay/ipc"
)

func rms(sum float64, n int64) float64 {
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}

func writePNG(path string, hdr ipc.VideoHeader, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, toImage(hdr, data))
}

// openReport opens results.jsonl: one line per check, written as it finishes, so
// a run that is stopped keeps what it found. With -continue it reads what is there.
func openReport(c *config) (*os.File, []Result, error) {
	path := filepath.Join(c.out, "results.jsonl")
	var done []Result
	if c.keepGoing {
		if f, err := os.Open(path); err == nil {
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1<<20), 1<<24)
			for sc.Scan() {
				var r Result
				if json.Unmarshal(sc.Bytes(), &r) == nil {
					done = append(done, r)
				}
			}
			f.Close()
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		return f, done, err
	}
	f, err := os.Create(path)
	return f, nil, err
}

type tally struct {
	console, core string
	games         int
	outcomes      map[string]int
	warnings      map[string]int
	fpsRatio      []float64
	cpu           []float64
	peak          float64
	failed        []Result
	reasons       map[string]int
}

// writeSummary writes summary.txt and prints it. It names no games: those are in
// results.jsonl, and a failed game is given by its number there.
func writeSummary(c *config, all []Result) error {
	by := map[string]*tally{}
	var order []string
	for _, r := range all {
		k := r.Console + "/" + r.Core
		t := by[k]
		if t == nil {
			t = &tally{console: r.Console, core: r.Core, outcomes: map[string]int{}, warnings: map[string]int{}, reasons: map[string]int{}}
			by[k] = t
			order = append(order, k)
		}
		t.games++
		t.outcomes[r.Outcome]++
		for _, w := range r.Warnings {
			t.warnings[w]++
		}
		if r.TargetFPS > 0 && r.FPS > 0 {
			t.fpsRatio = append(t.fpsRatio, r.FPS/r.TargetFPS)
		}
		if r.CPUPercent > 0 {
			t.cpu = append(t.cpu, r.CPUPercent)
		}
		t.peak = math.Max(t.peak, r.PeakMB)
		if r.Outcome != OutcomeOK && r.Outcome != OutcomeRefused {
			t.failed = append(t.failed, r)
			t.reasons[r.Outcome+": "+firstLine(r.Reason)]++
		}
	}
	sort.Strings(order)
	var b strings.Builder
	fmt.Fprintf(&b, "wittyplay-check, %s, %.0f seconds a game, memory limit %d MB\n\n", time.Now().Format("2006-01-02 15:04"), c.seconds, c.memoryMB)
	for _, k := range order {
		t := by[k]
		fmt.Fprintf(&b, "%s: %d games, %d ok\n", k, t.games, t.outcomes[OutcomeOK])
		if len(t.outcomes) > 1 || t.outcomes[OutcomeOK] == 0 {
			fmt.Fprintf(&b, "  outcomes: %s\n", counts(t.outcomes))
		}
		if len(t.warnings) > 0 {
			fmt.Fprintf(&b, "  warnings: %s\n", counts(t.warnings))
		}
		if len(t.fpsRatio) > 0 {
			sort.Float64s(t.fpsRatio)
			sort.Float64s(t.cpu)
			fmt.Fprintf(&b, "  speed: median %.0f%% of target, worst %.0f%%", 100*t.fpsRatio[len(t.fpsRatio)/2], 100*t.fpsRatio[0])
			if len(t.cpu) > 0 {
				fmt.Fprintf(&b, "; CPU median %.0f%% of a thread, worst %.0f%%", t.cpu[len(t.cpu)/2], t.cpu[len(t.cpu)-1])
			}
			if t.peak > 0 {
				fmt.Fprintf(&b, "; peak memory %.0f MB", t.peak)
			}
			b.WriteString("\n")
		}
		reasons := make([]string, 0, len(t.reasons))
		for r := range t.reasons {
			reasons = append(reasons, r)
		}
		sort.Slice(reasons, func(i, j int) bool { return t.reasons[reasons[i]] > t.reasons[reasons[j]] })
		for _, r := range reasons {
			fmt.Fprintf(&b, "  %d x %s\n", t.reasons[r], r)
		}
		if n := len(t.failed); n > 0 {
			nums := make([]string, 0, n)
			sort.Slice(t.failed, func(i, j int) bool { return t.failed[i].Index < t.failed[j].Index })
			for _, r := range t.failed {
				nums = append(nums, strconv.Itoa(r.Index))
			}
			fmt.Fprintf(&b, "  not ok, by number: %s\n", strings.Join(nums, " "))
		}
		if c.shots {
			makeSheets(c, t.core, all)
		}
	}
	text := b.String()
	fmt.Print("\n" + text)
	return os.WriteFile(filepath.Join(c.out, "summary.txt"), []byte(text), 0o644)
}

func counts(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if m[keys[i]] != m[keys[j]] {
			return m[keys[i]] > m[keys[j]]
		}
		return keys[i] < keys[j]
	})
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
	}
	return strings.Join(parts, ", ")
}

// makeSheets joins each core's pictures into pages a person can look over: wrong
// colours and garbled pictures are things only eyes catch.
func makeSheets(c *config, core string, all []Result) {
	const cols, rows, tw, th = 14, 10, 128, 112
	var list []Result
	for _, r := range all {
		if r.Core == core && r.Shot != "" {
			list = append(list, r)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Index < list[j].Index })
	dir := filepath.Join(c.out, "sheets")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	page := 0
	for start := 0; start < len(list); start += cols * rows {
		page++
		sheet := image.NewRGBA(image.Rect(0, 0, cols*tw, rows*th))
		var key strings.Builder
		for i, r := range list[start:min(start+cols*rows, len(list))] {
			img := readPNG(r.Shot)
			if img == nil {
				continue
			}
			x, y := (i%cols)*tw, (i/cols)*th
			fit(sheet, image.Rect(x, y, x+tw-1, y+th-1), img)
			fmt.Fprintf(&key, "row %d col %d: #%d %s\n", i/cols+1, i%cols+1, r.Index, r.Outcome)
		}
		name := filepath.Join(dir, fmt.Sprintf("%s-%02d", core, page))
		if f, err := os.Create(name + ".png"); err == nil {
			_ = png.Encode(f, sheet)
			f.Close()
		}
		_ = os.WriteFile(name+".txt", []byte(key.String()), 0o644)
	}
}

func readPNG(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// fit draws img scaled into r with nearest-neighbour.
func fit(dst draw.Image, r image.Rectangle, img image.Image) {
	b := img.Bounds()
	for y := 0; y < r.Dy(); y++ {
		for x := 0; x < r.Dx(); x++ {
			dst.Set(r.Min.X+x, r.Min.Y+y, img.At(b.Min.X+x*b.Dx()/r.Dx(), b.Min.Y+y*b.Dy()/r.Dy()))
		}
	}
}
