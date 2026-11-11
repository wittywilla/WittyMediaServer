// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command corebuilds keeps internal/server/corebuilds.txt true: which game cores libretro's build
// server publishes for each platform a server can download them on. A core with no build for a
// platform isn't offered to install there, so nobody meets a 404 halfway through.
//
//	go run ./scripts/corebuilds -write internal/server/corebuilds.txt   # look again, and rewrite it
//	go run ./scripts/corebuilds internal/server/corebuilds.txt          # check it: exit 1 if it's wrong
//
// It only asks for each download's headers (HEAD), and never downloads a core. The address is
// WittyPlay's (cores.DownloadURL): nightly/<platform>/latest/<core>_libretro.<ext>.zip. A core is
// added by adding its line with ? in every column and writing the file.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// platforms are the table's columns: its name in the file, the build server's folder for it, and
// the core file's extension there.
var platforms = []struct{ name, folder, ext string }{
	{"windows/x64", "windows/x86_64", "dll"},
	{"linux/x64", "linux/x86_64", "so"},
	{"linux/arm64", "linux/aarch64", "so"},
	{"linux/arm32", "linux/armv7-neon-hf", "so"},
	{"macos/x64", "apple/osx/x86_64", "dylib"},
	{"macos/arm64", "apple/osx/arm64", "dylib"},
}

const host = "https://buildbot.libretro.com/nightly/"

func url(core string, p int) string {
	return host + platforms[p].folder + "/latest/" + core + "_libretro." + platforms[p].ext + ".zip"
}

var client = &http.Client{Timeout: 30 * time.Second}

// published asks whether a build is there: true, false, or an error when the server didn't say.
func published(u string) (bool, error) {
	var last error
	for try := 0; try < 3; try++ {
		req, err := http.NewRequest(http.MethodHead, u, nil)
		if err != nil {
			return false, err
		}
		req.Header.Set("User-Agent", "wittymediaserver-corebuilds")
		res, err := client.Do(req)
		if err == nil {
			res.Body.Close()
			switch {
			case res.StatusCode == http.StatusOK:
				return true, nil
			case res.StatusCode == http.StatusNotFound:
				return false, nil
			}
			err = fmt.Errorf("%s answered %s", u, res.Status)
		}
		last = err
		time.Sleep(time.Duration(try+1) * 2 * time.Second)
	}
	return false, last
}

func main() {
	write := flag.Bool("write", false, "rewrite the file from what the build server says")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: corebuilds [-write] internal/server/corebuilds.txt")
		os.Exit(2)
	}
	path := flag.Arg(0)
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var head []string
	type row struct {
		core string
		has  []string
	}
	var rows []row
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		fields := strings.Fields(line)
		if strings.HasPrefix(line, "#") || len(fields) == 0 {
			if len(rows) == 0 {
				head = append(head, line)
			}
			continue
		}
		rows = append(rows, row{fields[0], fields[1:]})
	}
	f.Close()

	type job struct{ r, p int }
	got := make([][]string, len(rows))
	for i := range got {
		got[i] = make([]string, len(platforms))
	}
	var problems []string
	var mu sync.Mutex
	jobs := make(chan job)
	var wg sync.WaitGroup
	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				ok, err := published(url(rows[j.r].core, j.p))
				mu.Lock()
				switch {
				case err != nil:
					got[j.r][j.p] = "?"
					problems = append(problems, err.Error())
				case ok:
					got[j.r][j.p] = "y"
				default:
					got[j.r][j.p] = "n"
				}
				mu.Unlock()
			}
		}()
	}
	for r := range rows {
		for p := range platforms {
			jobs <- job{r, p}
		}
	}
	close(jobs)
	wg.Wait()

	if *write {
		var b strings.Builder
		for _, h := range head {
			b.WriteString(h + "\n")
		}
		for r, row := range rows {
			b.WriteString(fmt.Sprintf("%-22s %s\n", row.core, strings.Join(got[r], " ")))
		}
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	for r, row := range rows {
		for p := range platforms {
			want := ""
			if p < len(row.has) {
				want = row.has[p]
			}
			switch {
			case *write:
			case want == "y" && got[r][p] == "n":
				problems = append(problems, fmt.Sprintf("%s for %s is listed but gone: %s", row.core, platforms[p].name, url(row.core, p)))
			case want == "n" && got[r][p] == "y":
				problems = append(problems, fmt.Sprintf("%s for %s is published now; write the table again to offer it", row.core, platforms[p].name))
			case want != "y" && want != "n":
				problems = append(problems, fmt.Sprintf("%s for %s isn't known yet; write the table again", row.core, platforms[p].name))
			}
		}
	}
	for _, p := range problems {
		fmt.Println(p)
	}
	if len(problems) > 0 {
		os.Exit(1)
	}
	fmt.Printf("%d cores, %d platforms: all as listed\n", len(rows), len(platforms))
}
