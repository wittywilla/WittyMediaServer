package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/wittywilla/wittymediaplayer/engine/internal/cdrip"
)

// runCDRip reads tracks and writes them to stdout for the window to encode: one line of
// JSON per report, and a line announcing each stretch of samples before its bytes. It
// never touches the disk.
//
//	--cd-rip <source> <tracks, comma-separated> <secure|burst> <offset in samples>
func runCDRip(args []string, stop io.Reader, stdout io.Writer) int {
	out := bufio.NewWriterSize(stdout, 1<<20)
	defer out.Flush()
	enc := json.NewEncoder(out)
	say := func(v map[string]any) {
		_ = enc.Encode(v)
		_ = out.Flush()
	}
	fail := func(track int, err error) int {
		code := "failed"
		switch {
		case errors.Is(err, cdrip.ErrNoRead):
			code = "unreadable"
		case errors.Is(err, cdrip.ErrDiscGone):
			code = "gone"
		case errors.Is(err, context.Canceled):
			code = "cancelled"
		}
		say(map[string]any{"event": "error", "track": track, "code": code, "message": capitalize(err.Error()) + "."})
		return 1
	}
	if len(args) != 4 {
		return fail(0, errors.New("that isn't how to ask for a copy"))
	}
	src, err := cdrip.Open(args[0])
	if err != nil {
		return fail(0, err)
	}
	defer src.Close()
	info := src.Info()
	offset, _ := strconv.Atoi(args[3])
	if offset < -5000 || offset > 5000 {
		return fail(0, errors.New("that drive offset isn't one any drive has"))
	}
	opt := cdrip.Options{Secure: args[2] == "secure", Offset: offset}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if stop != nil {
		go func() {
			_, _ = io.Copy(io.Discard, stop)
			cancel()
		}()
	}
	failed := false
	for _, s := range strings.Split(args[1], ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return fail(0, fmt.Errorf("%q isn't a track", s))
		}
		var t *cdrip.Track
		for _, a := range info.AudioTracks() {
			if a.Number == n {
				t = a
			}
		}
		if t == nil {
			return fail(n, fmt.Errorf("this disc has no audio track %d", n))
		}
		say(map[string]any{"event": "track", "track": n, "bytes": int64(t.Sectors()) * cdrip.SectorBytes})
		last := -1
		rep, err := cdrip.RipTrack(ctx, src, t, opt, func(done, total int) {
			if pct := done * 100 / total; pct != last {
				last = pct
				say(map[string]any{"event": "progress", "track": n, "percent": pct})
			}
		}, func(p []byte) error {
			if err := enc.Encode(map[string]any{"event": "pcm", "track": n, "bytes": len(p)}); err != nil {
				return err
			}
			if _, err := out.Write(p); err != nil {
				return err
			}
			return out.Flush()
		})
		if errors.Is(err, cdrip.ErrNoRead) {
			// One bad track needn't cost the rest of the disc.
			fail(n, err)
			failed = true
			continue
		}
		if err != nil {
			return fail(n, err)
		}
		say(map[string]any{"event": "done", "track": n, "report": rep, "quality": rep.Quality()})
	}
	say(map[string]any{"event": "finished"})
	if failed {
		return 1
	}
	return 0
}

// runCDImage copies a disc to a .cue and .bin, and opens the result to check it.
func runCDImage(source, cue string, secure bool, offset int, stop io.Reader, stdout io.Writer) int {
	enc := json.NewEncoder(stdout)
	fail := func(code string, err error) int {
		_ = enc.Encode(map[string]any{"event": "error", "code": code, "message": capitalize(err.Error()) + "."})
		return 1
	}
	src, err := cdrip.Open(source)
	if err != nil {
		return fail("unreadable", err)
	}
	defer src.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if stop != nil {
		go func() {
			_, _ = io.Copy(io.Discard, stop)
			cancel()
		}()
	}
	last := -1
	rep, err := cdrip.WriteImage(ctx, src, cue, cdrip.Options{Secure: secure, Offset: offset}, func(done, total int) {
		if pct := done * 100 / total; pct != last {
			last = pct
			_ = enc.Encode(map[string]any{"event": "progress", "percent": pct})
		}
	})
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return fail("cancelled", errors.New("the copy was stopped, and the partial file removed"))
		}
		return fail("failed", err)
	}
	check, err := cdrip.Open(cue)
	verified := false
	if err == nil {
		verified = check.Info().DiscID == src.Info().DiscID
		check.Close()
	}
	_ = enc.Encode(map[string]any{"event": "done", "path": cue, "report": rep, "verified": verified})
	return 0
}

// runCDStream serves the WAV form of a disc to the player's decoder: a line "R <offset>
// <length>" on stdin, answered by a line with the count and then those bytes. It ends
// when stdin closes.
func runCDStream(source string, in io.Reader, stdout io.Writer) int {
	out := bufio.NewWriterSize(stdout, 1<<16)
	src, err := cdrip.Open(source)
	if err != nil {
		fmt.Fprintf(out, "E %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
		out.Flush()
		return 1
	}
	defer src.Close()
	l := cdrip.StreamLayout(src.Info())
	size := int64(44) + int64(l.Last-l.First)*cdrip.SectorBytes
	fmt.Fprintf(out, "S %d\n", size)
	out.Flush()
	sc := bufio.NewScanner(in)
	buf := make([]byte, 1<<20)
	for sc.Scan() {
		var off int64
		var n int
		if _, err := fmt.Sscanf(sc.Text(), "R %d %d", &off, &n); err != nil || n <= 0 || off < 0 {
			return 1
		}
		n = min(n, len(buf))
		got, err := cdrip.ReadStream(src, l, buf[:n], off)
		if err != nil {
			fmt.Fprintf(out, "E %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
			out.Flush()
			return 1
		}
		fmt.Fprintf(out, "D %d\n", got)
		out.Write(buf[:got])
		out.Flush()
	}
	return 0
}
