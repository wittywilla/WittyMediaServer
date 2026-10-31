// Command wittyplay-check runs a core and a game with no server and no browser,
// and says what came out: frames a second, whether the picture changes, whether
// there is sound. It is how a core is tried on a new machine, and how to see
// which graphics card is doing the drawing.
package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittyplay/engine"
	"github.com/wittywilla/wittyplay/libretro"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	core := flag.String("core", "", "the core's shared library")
	rom := flag.String("rom", "", "the game")
	system := flag.String("system", "", "the BIOS folder")
	save := flag.String("save", "", "the folder the core may save in")
	seconds := flag.Int("seconds", 30, "how long to run")
	snap := flag.String("snap", "", "folder to write a picture to, once every -snap-every seconds")
	snapEvery := flag.Int("snap-every", 10, "seconds between pictures")
	verbose := flag.Bool("v", false, "show everything the core logs")
	realtime := flag.Bool("realtime", true, "run at the console's own speed; off shows how fast it can go")
	listOptions := flag.Bool("options", false, "list the core's options and stop")
	var opts, press multi
	flag.Var(&opts, "opt", "a core option as key=value; may repeat")
	flag.Var(&press, "press", "hold a RetroPad button at a time, like start@5 or a@8+0.2; may repeat")
	flag.Parse()
	if *core == "" || (*rom == "" && !*listOptions) {
		flag.Usage()
		os.Exit(2)
	}
	runtime.LockOSThread()
	if *system == "" {
		*system = filepath.Dir(*rom)
	}
	if *save == "" {
		d, err := os.MkdirTemp("", "wittyplay-check-")
		if err != nil {
			fail(err)
		}
		defer os.RemoveAll(d)
		*save = d
	}
	options := map[string]string{}
	for _, o := range opts {
		k, v, _ := strings.Cut(o, "=")
		options[k] = v
	}
	presses := parsePresses(press)

	var (
		frames, blank, dupes, changes, audioFrames, loud int64
		lastHash                                         uint64
		lastW, lastH                                     int
		pads                                             [engine.MaxPorts]engine.PadState
		lastData                                         []byte
		lastStride, lastFmt                              int
		lastFlip                                         bool
	)
	video := func(flip bool) func(d []byte, w, h, stride, format int) {
		return func(d []byte, w, h, stride, format int) {
			frames++
			lastW, lastH, lastStride, lastFmt, lastFlip = w, h, stride, format, flip
			hash, nonBlack := sample(d, w, h, stride, format)
			if !nonBlack {
				blank++
			}
			if hash != lastHash {
				changes++
				lastHash = hash
			}
			if *snap != "" {
				lastData = append(lastData[:0], d[:stride*h]...)
			}
		}
	}
	h := engine.Handlers{
		Video:        video(false),
		VideoFlipped: video(true),
		Dupe:         func() { dupes++ },
		Audio: func(s []int16) {
			audioFrames += int64(len(s) / 2)
			for _, v := range s {
				if v > 200 || v < -200 {
					loud++
				}
			}
		},
		Input: func() *[engine.MaxPorts]engine.PadState { return &pads },
		Log: func(level int, text string) {
			if *verbose || level >= 2 {
				fmt.Printf("core[%d]: %s\n", level, text)
			}
		},
		Message:  func(t string) { fmt.Println("message:", t) },
		Geometry: func(w, h int, a float64) { fmt.Printf("geometry: %dx%d aspect %.3f\n", w, h, a) },
		AVInfo:   func(a engine.AVInfo) { fmt.Printf("avinfo: %+v\n", a) },
		Shutdown: func() { fmt.Println("the core asked to shut down"); os.Exit(0) },
	}
	c, err := libretro.Load(*core, libretro.Paths{System: *system, Save: *save}, options, h)
	if err != nil {
		fail(err)
	}
	info := c.SystemInfo()
	fmt.Printf("core: %s %s, extensions %v\n", info.Name, info.Version, info.Extensions)
	if *listOptions {
		for _, line := range c.OptionList() {
			fmt.Println("option:", line)
		}
		return
	}
	start := time.Now()
	av, err := c.LoadGame(*rom)
	if err != nil {
		fail(err)
	}
	fmt.Printf("loaded in %v: %dx%d (max %dx%d) aspect %.3f at %.3f fps, sound %.0f Hz\n", time.Since(start).Round(time.Millisecond),
		av.BaseWidth, av.BaseHeight, av.MaxWidth, av.MaxHeight, av.Aspect, av.FPS, av.SampleRate)
	fmt.Printf("graphics: %q\n", c.Renderer())

	begin := time.Now()
	next := begin
	frame := time.Duration(float64(time.Second) / av.FPS)
	var lastFrames, lastChanges, lastDupes int64
	var runTotal time.Duration
	lastPrint := begin
	lastSnap := begin
	snaps := 0
	for time.Since(begin) < time.Duration(*seconds)*time.Second {
		now := time.Since(begin).Seconds()
		pads[0].Buttons = 0
		for _, p := range presses {
			if now >= p.at && now < p.at+p.hold {
				pads[0].Buttons |= 1 << p.button
			}
		}
		t0 := time.Now()
		c.Run()
		runTotal += time.Since(t0)
		if *realtime {
			next = next.Add(frame)
			if d := time.Until(next); d > 0 {
				time.Sleep(d)
			} else if d < -5*frame {
				next = time.Now()
			}
		}
		if time.Since(lastPrint) >= 5*time.Second {
			el := time.Since(lastPrint).Seconds()
			n := frames + dupes - lastFrames - lastDupes
			fmt.Printf("t=%3.0fs %5.1f fps  %dx%d  changed %d of %d  blank-so-far %d  run %.1fms/frame  audio %d loud %d\n", time.Since(begin).Seconds(), float64(n)/el,
				lastW, lastH, changes-lastChanges, n, blank, float64(runTotal.Microseconds())/1000/float64(max(n, 1)), audioFrames, loud)
			lastFrames, lastChanges, lastDupes, runTotal = frames, changes, dupes, 0
			lastPrint = time.Now()
		}
		if *snap != "" && time.Since(lastSnap) >= time.Duration(*snapEvery)*time.Second && lastData != nil {
			snaps++
			writeSnap(filepath.Join(*snap, "snap-"+strconv.Itoa(snaps)+".png"), lastData, lastW, lastH, lastStride, lastFmt, lastFlip)
			lastSnap = time.Now()
		}
	}
	el := time.Since(begin).Seconds()
	fmt.Printf("TOTAL %.1fs: %d frames %d dupes (%.1f a second), %d changed, %d blank; audio %d samples (want %.0f), %d loud\n", el, frames, dupes, float64(frames+dupes)/el, changes, blank,
		audioFrames, av.SampleRate*el, loud)
	if st := c.SaveState(); st != nil {
		fmt.Printf("save state: %d bytes, restores: %v\n", len(st), c.LoadState(st))
	} else {
		fmt.Println("save state: none")
	}
	// A core that hangs on its way out shouldn't hide the numbers above.
	time.AfterFunc(5*time.Second, func() { fmt.Println("the core did not close in 5 seconds"); os.Exit(0) })
	c.Close()
	fmt.Println("closed cleanly")
}

type pressAt struct {
	button   int
	at, hold float64
}

var buttons = map[string]int{"b": engine.ButtonB, "y": engine.ButtonY, "select": engine.ButtonSelect, "start": engine.ButtonStart, "up": engine.ButtonUp, "down": engine.ButtonDown,
	"left": engine.ButtonLeft, "right": engine.ButtonRight, "a": engine.ButtonA, "x": engine.ButtonX, "l": engine.ButtonL, "r": engine.ButtonR, "l2": engine.ButtonL2, "r2": engine.ButtonR2}

func parsePresses(list []string) []pressAt {
	var out []pressAt
	for _, p := range list {
		name, rest, _ := strings.Cut(p, "@")
		at, hold, ok := strings.Cut(rest, "+")
		h := 0.2
		if ok {
			h, _ = strconv.ParseFloat(hold, 64)
		}
		t, _ := strconv.ParseFloat(at, 64)
		b, known := buttons[name]
		if !known {
			fail(fmt.Errorf("unknown button %q", name))
		}
		out = append(out, pressAt{b, t, h})
	}
	return out
}

// sample hashes a thin grid of the picture and sees whether anything isn't black.
func sample(d []byte, w, h, stride, format int) (uint64, bool) {
	f := fnv.New64a()
	bpp := engine.BytesPerPixel(format)
	nonBlack := false
	for y := 0; y < h; y += max(h/32, 1) {
		row := d[y*stride : y*stride+w*bpp]
		for x := 0; x+bpp <= len(row); x += max(w/32, 1) * bpp {
			px := row[x : x+bpp]
			_, _ = f.Write(px)
			for i, v := range px {
				if v > 16 && (bpp == 2 || i < 3) {
					nonBlack = true
				}
			}
		}
	}
	return f.Sum64(), nonBlack
}

func writeSnap(path string, d []byte, w, h, stride, format int, flip bool) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := y
		if flip {
			src = h - 1 - y
		}
		row := d[src*stride:]
		for x := 0; x < w; x++ {
			var c color.RGBA
			switch format {
			case engine.FormatXRGB8888:
				c = color.RGBA{row[x*4+2], row[x*4+1], row[x*4], 255}
			case engine.FormatRGB565:
				v := uint16(row[x*2]) | uint16(row[x*2+1])<<8
				c = color.RGBA{byte(v>>11) << 3, byte(v>>5) << 2, byte(v) << 3, 255}
			default:
				v := uint16(row[x*2]) | uint16(row[x*2+1])<<8
				c = color.RGBA{byte(v>>10) << 3, byte(v>>5) << 3, byte(v) << 3, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("picture:", err)
		return
	}
	defer f.Close()
	_ = png.Encode(f, img)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "failed:", err)
	os.Exit(1)
}
