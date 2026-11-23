// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command wittyinfo makes, checks and edits .wittyinfo files (docs/formats/wittyinfo.md)
// without a server: what people add by hand to a film or an episode, a season or a show.
//
//	wittyinfo new <video>                   start one beside a video, its identity worked out
//	wittyinfo new --show <folder>           start a show's (or --season N) defaults
//	wittyinfo show <file>                   what one holds, in words
//	wittyinfo check <file>...               check files; every problem is listed
//	wittyinfo unpack <file> <folder>        unpack, to change manifest.json by hand
//	wittyinfo pack <folder> <file>          pack again, checked
//	wittyinfo image [--add] <file> <role> <picture> put a poster, backdrop... in it (--add: another one)
//	wittyinfo match <file> <video>          does it fit this cut of the video?
//	wittyinfo edit <video or file>          the editor, in a browser on this computer
//
// It never runs anything a file holds, and edit listens on 127.0.0.1 only, with a key in
// the address that only the person who started it sees.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/i18n"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/wittyinfo"
)

// version is set when releases are built.
var version = "dev"

const usage = `wittyinfo makes, checks and edits .wittyinfo files.

  wittyinfo new <video>                    start one beside a video
  wittyinfo new --show <folder>            start a show's defaults (--season N for a season's)
  wittyinfo show <file>                    what one holds, in words
  wittyinfo check <file>...                check files; every problem is listed
  wittyinfo unpack <file> <folder>         unpack, to change manifest.json by hand
  wittyinfo pack <folder> <file>           pack again, checked
  wittyinfo image [--add] <file> <role> <picture>
                                           put the picture shown for a role; --add puts another
                                           after it; images/<name> shows that one first;
                                           -images/<name> takes one out, - takes them all out;
                                           roles: poster backdrop logo thumb banner clearart disc
                                           square, and back artist in a .wittyaudio
  wittyinfo match <file> <video>           does it fit this cut of the video?
  wittyinfo edit <video or file>           the editor, in a browser on this computer
  wittyinfo version

The format: docs/formats/wittyinfo.md. ffprobe (from FFmpeg) is used to read a video's
length and chapters when it is on the PATH, or named with --ffprobe or WITTYINFO_FFPROBE.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "new":
		err = cmdNew(args)
	case "show":
		err = cmdShow(args)
	case "check":
		err = cmdCheck(args)
	case "unpack":
		err = cmdUnpack(args)
	case "pack":
		err = cmdPack(args)
	case "image":
		err = cmdImage(args)
	case "match":
		err = cmdMatch(args)
	case "edit":
		err = cmdEdit(args)
	case "version", "--version", "-v":
		fmt.Println("wittyinfo", version)
	case "help", "--help", "-h":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "wittyinfo: no command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittyinfo:", err)
		os.Exit(1)
	}
}

// ------------------------------------------------------------------ files

func readInfo(path string) (*wittyinfo.Info, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > wittyinfo.MaxFile {
		return nil, fmt.Errorf("%s is larger than %d MB", path, wittyinfo.MaxFile>>20)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return wittyinfo.Parse(b)
}

// writeInfo writes in one step: a half-written file is never left.
func writeInfo(path string, x *wittyinfo.Info) error {
	x.Stamp("", "wittyinfo", version, time.Now())
	if x.Authorship.Author == "" {
		x.Authorship.Author = os.Getenv("WITTYINFO_AUTHOR")
	}
	b, err := x.Encode()
	if err != nil {
		return err
	}
	tmp := path + ".tmp-" + strconv.FormatInt(time.Now().UnixNano(), 36)
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// sidecarFor is where a video's .wittyinfo goes.
func sidecarFor(video string) string {
	if st, err := os.Stat(video); err == nil && st.IsDir() {
		return filepath.Clean(video) + wittyinfo.Ext
	}
	return strings.TrimSuffix(video, filepath.Ext(video)) + wittyinfo.Ext
}

// isInfo: a path names a .wittyinfo rather than a video.
func isInfo(p string) bool { return strings.EqualFold(filepath.Ext(p), wittyinfo.Ext) }

// ------------------------------------------------------------------ ffprobe

var ffprobeFlag string

func ffprobePath() string {
	for _, p := range []string{ffprobeFlag, os.Getenv("WITTYINFO_FFPROBE")} {
		if p != "" {
			return p
		}
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return ""
}

type probeChapter struct {
	Start float64 `json:"start"`
	Title string  `json:"title"`
}

// probe reads a video's length and chapters with ffprobe, if there is one.
func probe(video string) (dur float64, chapters []probeChapter) {
	bin := ffprobePath()
	if bin == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "-v", "error", "-show_entries", "format=duration:chapter=start_time:chapter_tags=title",
		"-of", "json", "--", video).Output()
	if err != nil {
		return 0, nil
	}
	var p struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
		Chapters []struct {
			Start string            `json:"start_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if json.Unmarshal(out, &p) != nil {
		return 0, nil
	}
	dur, _ = strconv.ParseFloat(p.Format.Duration, 64)
	for _, c := range p.Chapters {
		if at, err := strconv.ParseFloat(c.Start, 64); err == nil {
			chapters = append(chapters, probeChapter{Start: at, Title: c.Tags["title"]})
		}
	}
	return dur, chapters
}

// identityOf works out which video a file is for.
func identityOf(video string) (*wittyinfo.Identity, error) {
	st, err := os.Stat(video)
	if err != nil {
		return nil, err
	}
	id := &wittyinfo.Identity{FileName: filepath.Base(video)}
	if !st.IsDir() {
		fp, size, err := wittyinfo.FingerprintFile(video)
		if err != nil {
			return nil, err
		}
		id.Fingerprint, id.FileSize = fp, size
	}
	dur, _ := probe(video)
	id.DurationSec = wittyinfo.Seconds(dur)
	return id, nil
}

// ------------------------------------------------------------------ commands

func cmdNew(args []string) error {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	show := fs.String("show", "", "a show's folder: start its show.wittyinfo")
	season := fs.Int("season", -1, "with --show: start this season's defaults instead")
	dur := fs.Float64("duration", 0, "the video's length in seconds, when ffprobe isn't there")
	author := fs.String("author", "", "who is making it")
	force := fs.Bool("force", false, "replace a file that is there already")
	fs.StringVar(&ffprobeFlag, "ffprobe", "", "the ffprobe program to use")
	fs.Parse(args)
	var x *wittyinfo.Info
	var out string
	switch {
	case *show != "":
		if *season >= 0 {
			x = wittyinfo.New(wittyinfo.ScopeSeason)
			x.Season = season
			out = filepath.Join(*show, fmt.Sprintf("season%02d%s", *season, wittyinfo.Ext))
		} else {
			x = wittyinfo.New(wittyinfo.ScopeShow)
			out = filepath.Join(*show, "show"+wittyinfo.Ext)
		}
	case fs.NArg() == 1:
		video := fs.Arg(0)
		id, err := identityOf(video)
		if err != nil {
			return err
		}
		if *dur > 0 {
			id.DurationSec = wittyinfo.Seconds(*dur)
		}
		if id.DurationSec <= 0 {
			fmt.Fprintln(os.Stderr, "wittyinfo: the video's length isn't known (no ffprobe?), so the cut can't be checked later. Give it with --duration.")
			id = nil
		}
		x = wittyinfo.New(wittyinfo.ScopeItem)
		x.Identity = id
		out = sidecarFor(video)
	default:
		return errors.New("usage: wittyinfo new <video>, or wittyinfo new --show <folder> [--season N]")
	}
	if _, err := os.Stat(out); err == nil && !*force {
		return fmt.Errorf("%s is there already (--force replaces it)", out)
	}
	x.Stamp(*author, "wittyinfo", version, time.Now())
	if err := writeInfo(out, x); err != nil {
		return err
	}
	fmt.Println("Made", out)
	return nil
}

func cmdShow(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: wittyinfo show <file>")
	}
	x, err := readInfo(args[0])
	if err != nil {
		return err
	}
	fmt.Print(describe(x))
	return nil
}

// describe says what a file holds, in words.
func describe(x *wittyinfo.Info) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("Format: .%s, version %d", x.Format, x.Version)
	if x.Newer() {
		w(" (newer than this program's %d: what it doesn't know is kept as it is)", wittyinfo.Version)
	}
	w("\n")
	switch x.ScopeOr() {
	case wittyinfo.ScopeShow:
		w("Defaults for every episode of a show\n")
	case wittyinfo.ScopeSeason:
		w("Defaults for every episode of season %d\n", *x.Season)
	default:
		w("For one video\n")
	}
	if id := x.Identity; id != nil {
		w(i18n.T("Made for: %s, %s long"), orDash(id.FileName), wittyinfo.Clock(float64(id.DurationSec)))
		if id.Edition != "" {
			w(" (%s)", id.Edition)
		}
		w("\n")
	}
	if a := x.Authorship; a != nil {
		w("By: %s, %s", orDash(a.Author), orDash(a.Updated))
		if a.Tool != nil {
			w(", with %s %s", a.Tool.Name, a.Tool.Version)
		}
		w("\n")
	}
	if len(x.Segments) > 0 {
		w("\nSegments\n")
		for _, s := range x.Segments {
			end := "the end"
			if s.End != nil {
				end = wittyinfo.Clock(float64(*s.End))
			}
			w("  %-11s %9s – %-9s %-6s %s\n", s.Type, wittyinfo.Clock(float64(s.Start)), end, s.Act(), s.Label)
		}
	}
	if len(x.Absent) > 0 {
		w("Has no: %s\n", strings.Join(x.Absent, ", "))
	}
	if len(x.Scenes) > 0 {
		w("\nScenes\n")
		for _, s := range x.Scenes {
			w("  %9s  %s\n", wittyinfo.Clock(float64(s.Start)), s.Title)
		}
	}
	if len(x.Filters) > 0 {
		w("\nContent filters: %d\n", len(x.Filters))
		count := map[string]int{}
		for _, f := range x.Filters {
			count[f.Category]++
		}
		for _, k := range sortedKeys(count) {
			w("  %s: %d\n", k, count[k])
		}
	}
	if m := x.Metadata; m != nil && m.Title != "" {
		w("\nTitle: %s\n", m.Title)
	}
	if len(x.Artwork) > 0 {
		w("\nPictures\n")
		for _, k := range sortedKeys(x.Artwork) {
			for i, name := range x.Artwork[k] {
				role := k
				if i > 0 {
					role = ""
				}
				w("  %-9s %s (%d KB)\n", role, name, len(x.Files[name])/1024)
			}
		}
	}
	if len(x.Subtitles) > 0 {
		w("\nSubtitles: %d\n", len(x.Subtitles))
	}
	if x.Notes != "" {
		w("\nNotes: %s\n", x.Notes)
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "–"
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cmdCheck(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: wittyinfo check <file>...")
	}
	bad := 0
	for _, p := range args {
		if _, err := readInfo(p); err != nil {
			bad++
			var e *wittyinfo.Error
			if errors.As(err, &e) {
				fmt.Printf("%s: %d problem(s)\n", p, len(e.Problems))
				for _, pr := range e.Problems {
					fmt.Println("  ", pr.String())
				}
			} else {
				fmt.Printf("%s: %v\n", p, err)
			}
			continue
		}
		fmt.Printf("%s: fine\n", p)
	}
	if bad > 0 {
		return fmt.Errorf("%d of %d can't be used", bad, len(args))
	}
	return nil
}

func cmdUnpack(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: wittyinfo unpack <file> <folder>")
	}
	x, err := readInfo(args[0])
	if err != nil {
		return err
	}
	dir := args[1]
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s isn't empty", dir)
	}
	man, err := x.Manifest()
	if err != nil {
		return err
	}
	files := map[string][]byte{wittyinfo.ManifestName: man}
	for k, v := range x.Files {
		files[k] = v
	}
	root, err := os.OpenRoot(mustMkdir(dir))
	if err != nil {
		return err
	}
	defer root.Close()
	for _, name := range sortedKeys(files) {
		// Names were checked when the file was read; the root keeps them inside anyway.
		if d := filepath.Dir(filepath.FromSlash(name)); d != "." {
			if err := root.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		if err := root.WriteFile(filepath.FromSlash(name), files[name], 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("Unpacked %d file(s) into %s\n", len(files), dir)
	return nil
}

func mustMkdir(dir string) string {
	_ = os.MkdirAll(dir, 0o755)
	return dir
}

func cmdPack(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: wittyinfo pack <folder> <file>")
	}
	dir, out := args[0], args[1]
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	files := map[string][]byte{}
	var total int64
	err = walkRoot(root, ".", func(name string, b []byte) error {
		if total += int64(len(b)); total > wittyinfo.MaxUnpacked {
			return fmt.Errorf("the folder holds more than %d MB", wittyinfo.MaxUnpacked>>20)
		}
		if len(files) >= wittyinfo.MaxMembers {
			return fmt.Errorf("the folder holds more than %d files", wittyinfo.MaxMembers)
		}
		files[name] = b
		return nil
	})
	if err != nil {
		return err
	}
	man, ok := files[wittyinfo.ManifestName]
	if !ok {
		return fmt.Errorf("%s has no %s", dir, wittyinfo.ManifestName)
	}
	delete(files, wittyinfo.ManifestName)
	// Pack as it stands, then read it back: the same checks as any reader.
	x, err := wittyinfo.Parse(man)
	if err != nil {
		var e *wittyinfo.Error
		if !errors.As(err, &e) {
			return err
		}
		// Members are checked once they are in: parse the manifest alone.
		x = &wittyinfo.Info{}
		if jerr := json.Unmarshal(bytes.TrimPrefix(man, []byte("\xef\xbb\xbf")), x); jerr != nil {
			return err
		}
	}
	for name, b := range files {
		if strings.HasPrefix(name, "images/") {
			if err := wittyinfo.CheckImage(name, b); err != nil {
				return fmt.Errorf("%s: %v", name, err)
			}
		}
	}
	x.Files = files
	if err := writeInfo(out, x); err != nil {
		return err
	}
	fmt.Println("Packed", out)
	return nil
}

// walkRoot reads every ordinary file under a root, by its name with forward slashes.
func walkRoot(root *os.Root, dir string, fn func(name string, b []byte) error) error {
	f, err := root.Open(dir)
	if err != nil {
		return err
	}
	entries, err := f.ReadDir(-1)
	f.Close()
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		name := filepath.ToSlash(p)
		switch {
		case e.IsDir():
			if err := walkRoot(root, p, fn); err != nil {
				return err
			}
		case e.Type().IsRegular():
			if !wittyinfo.ValidMemberName(name) {
				return fmt.Errorf("%s: a name can hold letters, digits, space and . _ - only", name)
			}
			rf, err := root.Open(p)
			if err != nil {
				return err
			}
			b, err := io.ReadAll(io.LimitReader(rf, wittyinfo.MaxImage+1))
			rf.Close()
			if err != nil {
				return err
			}
			if err := fn(name, b); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%s isn't an ordinary file", name)
		}
	}
	return nil
}

func cmdImage(args []string) error {
	add := len(args) > 0 && args[0] == "--add"
	if add {
		args = args[1:]
	}
	if len(args) != 3 {
		return errors.New("usage: wittyinfo image [--add] <file> <role> <picture | - | -name>")
	}
	x, err := readInfo(args[0])
	if err != nil {
		return err
	}
	role, what := args[1], args[2]
	var c wittyinfo.ImageChange
	switch {
	case what == "-":
		c = wittyinfo.ImageChange{Remove: true}
	case strings.HasPrefix(what, "-images/"):
		c = wittyinfo.ImageChange{Remove: true, Name: what[1:]}
	case strings.HasPrefix(what, "images/"):
		c = wittyinfo.ImageChange{First: what}
	default:
		b, err := os.ReadFile(what)
		if err != nil {
			return err
		}
		ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}[http.DetectContentType(b)]
		if ext == "" {
			return errors.New("the picture has to be a JPEG, PNG or WebP")
		}
		c = wittyinfo.ImageChange{Picture: b, Ext: ext, Add: add}
	}
	if err := x.ChangeImages(role, c); err != nil {
		return err
	}
	return writeInfo(args[0], x)
}

func cmdMatch(args []string) error {
	fs := flag.NewFlagSet("match", flag.ExitOnError)
	fs.StringVar(&ffprobeFlag, "ffprobe", "", "the ffprobe program to use")
	fs.Parse(args)
	if fs.NArg() != 2 {
		return errors.New("usage: wittyinfo match <file> <video>")
	}
	x, err := readInfo(fs.Arg(0))
	if err != nil {
		return err
	}
	id, err := identityOf(fs.Arg(1))
	if err != nil {
		return err
	}
	m := x.MatchVideo(float64(id.DurationSec), id.Fingerprint)
	fmt.Printf("%s", m.Result)
	if m.Message != "" {
		fmt.Printf(": %s", m.Message)
	}
	fmt.Println()
	if !m.Usable {
		os.Exit(3)
	}
	return nil
}
