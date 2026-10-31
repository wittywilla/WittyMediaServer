package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittybluray"
	"github.com/wittywilla/wittybluray/bdj"
)

// The remote's keys by name, for pressing during a run.
var bdjKeyCodes = map[string]int{"up": 38, "down": 40, "left": 37, "right": 39, "enter": 10, "red": 403, "green": 404, "yellow": 405, "blue": 406, "popup": 461,
	"0": 48, "1": 49, "2": 50, "3": 51, "4": 52, "5": 53, "6": 54, "7": 55, "8": 56, "9": 57}

// jdkNames lists the Java library classes the disc's JARs name.
func jdkNames(d *wittybluray.Disc, jars map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for j := range jars {
		b, err := d.ReadProgramFile("JAR/"+j+".jar", 256<<20)
		if err != nil {
			continue
		}
		names, err := bdj.JDKClasses(b)
		if err != nil {
			continue
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

// bdjInfo reports a disc's BD-J titles and whether the runtime has what their
// code links against; with Java on the machine it also runs the first title's
// applications for some seconds, pressing any keys named, and saves what they
// draw. Its output is what a bug report about a BD-J disc needs, and it holds no
// disc content.
func bdjInfo(d *wittybluray.Disc, seconds int, out string, keys []string) int {
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
	if len(names) == 0 {
		fmt.Println("this disc has no BD-J titles")
		return 0
	}
	sort.Strings(names)
	jars := map[string]bool{}
	var first *wittybluray.BDJObject
	for _, n := range names {
		o, err := d.BDJObject(n)
		if err != nil {
			fmt.Printf("object %s: %v\n", n, err)
			continue
		}
		if first == nil {
			first = o
		}
		fmt.Printf("object %s: version %s, %d applications, playlists %v (all=%v, autostart=%v), menu call masked=%v\n", n, o.Version, len(o.Apps), o.Playlists, o.AccessToAll, o.AutostartFirst, o.MenuCallMask)
		for _, a := range o.Apps {
			fmt.Printf("  app %x-%x control=%d jar=%s class=%s params=%v\n", a.Org, a.ID, a.Control, a.BaseDir, a.InitialClass, a.Params)
			jars[a.BaseDir] = true
		}
		for _, c := range o.Cache {
			if !c.Dir {
				jars[c.Name] = true
			}
		}
	}
	// What the disc's code needs and the runtime lacks.
	var missing map[string][]string
	var total int
	for j := range jars {
		b, err := d.ReadProgramFile("JAR/"+j+".jar", 256<<20)
		if err != nil {
			fmt.Printf("jar %s: %v\n", j, err)
			continue
		}
		m, n, err := bdj.MissingClasses(b)
		if err != nil {
			fmt.Printf("jar %s: %v\n", j, err)
			continue
		}
		total += n
		if missing == nil {
			missing = map[string][]string{}
		}
		for k, v := range m {
			missing[k] = append(missing[k], v...)
		}
	}
	fmt.Printf("%d classes in the disc's JAR files\n", total)
	if len(missing) == 0 {
		fmt.Println("every platform class they name is in the runtime")
	} else {
		fmt.Printf("%d platform classes they name are not in the runtime:\n", len(missing))
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s (used by %d classes)\n", k, len(missing[k]))
		}
	}
	if seconds <= 0 {
		return 0
	}
	java, err := bdj.Find("")
	if err != nil {
		fmt.Println(err)
		return 0
	}
	tmp, err := os.MkdirTemp("", "wittybluray-bdj")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(tmp)
	if refused, err := java.Refused(context.Background(), filepath.Join(tmp, "c"), jdkNames(d, jars)); err == nil && len(refused) > 0 {
		sort.Strings(refused)
		fmt.Printf("%d Java library classes they name are refused by the sandbox:\n", len(refused))
		for _, c := range refused {
			fmt.Println(" ", c)
		}
	}
	fmt.Printf("running object %s with %s for %d seconds\n", first.Name, java.Path, seconds)
	h, err := bdj.Start(context.Background(), d, bdj.Options{Java: java, CacheDir: filepath.Join(tmp, "c"), WorkDir: filepath.Join(tmp, "w"), PersistDir: filepath.Join(tmp, "p"),
		Log: func(s string) { fmt.Println("  java:", s) },
		OnEvent: func(e bdj.Event) {
			if e.Kind != "gpr" && e.Kind != "psr" {
				fmt.Println("  app:", e.Kind, strings.Join(e.Args, " "))
			}
		}})
	if err != nil {
		fmt.Println(err)
		return 1
	}
	defer h.Close()
	h.Sync(wittybluray.NewRegisters(wittybluray.RegisterOptions{}))
	h.StartTitle(0xffff, first)
	// Keys go in evenly through the run, each after the menu has had time to show.
	each := time.Duration(seconds) * time.Second / time.Duration(len(keys)+1)
	for _, k := range keys {
		time.Sleep(each)
		code, known := bdjKeyCodes[k]
		if !known {
			fmt.Printf("no key called %q\n", k)
			continue
		}
		fmt.Println("  pressing", k)
		h.Send("key %d", code)
	}
	time.Sleep(each)
	fmt.Printf("%d pictures drawn\n", h.Version())
	if mem, threads, cpu, ok := h.Usage(); ok {
		fmt.Printf("the Java process holds %d MB and %d threads, and has used %v of processor time\n", mem>>20, threads, cpu.Round(time.Second))
	}
	if out != "" && h.Version() > 0 {
		b, _ := h.PNG()
		if err := os.WriteFile(out, b, 0o644); err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("the last picture is in", out)
		}
	}
	return 0
}
