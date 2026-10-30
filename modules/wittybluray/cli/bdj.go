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

// bdjInfo reports a disc's BD-J titles and whether the runtime has what their
// code links against; with Java on the machine it also runs the first title's
// applications for some seconds and saves what they draw. Its output is what
// a bug report about a BD-J disc needs, and it holds no disc content.
func bdjInfo(d *wittybluray.Disc, seconds int, out string) int {
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
	time.Sleep(time.Duration(seconds) * time.Second)
	fmt.Printf("%d pictures drawn\n", h.Version())
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
