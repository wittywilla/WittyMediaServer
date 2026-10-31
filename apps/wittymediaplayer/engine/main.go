// wittymediaplayer-engine is the helper behind the WittyMediaPlayer window. It
// reads disc structures and runs the disc engines in a process of its own, so a
// damaged or hostile disc can't take the window down, and it needs nothing but
// its stdin and stdout: it has no network code at all.
//
//	wittymediaplayer-engine --helper                 the window's helper
//	wittymediaplayer-engine --drives                 lists optical drives as JSON
//	wittymediaplayer-engine --image <source> <out>   copies a disc to an .iso or .img
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
	"github.com/wittywilla/wittymediaplayer/engine/internal/drives"
	"github.com/wittywilla/wittymediaplayer/engine/internal/imaging"
	"github.com/wittywilla/wittymodule"
)

// Version is the engine's version, set by the build.
var Version = "0.1.0"

// HelperFlag starts the helper protocol.
const HelperFlag = "--helper"

func main() {
	args := os.Args[1:]
	switch {
	case len(args) == 1 && args[0] == HelperFlag:
		h := &helper{}
		err := wittymodule.Serve(os.Stdin, os.Stdout, wittymodule.Hello{Module: "wittymediaplayer-engine", Version: Version, API: 1}, h.handle)
		h.closeSession()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case len(args) == 1 && args[0] == "--drives":
		list, err := drives.List()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(list)
	case len(args) == 3 && args[0] == "--image":
		os.Exit(runImage(args[1], args[2], os.Stdin, os.Stdout))
	case len(args) == 1 && args[0] == "--version":
		fmt.Println("wittymediaplayer-engine", Version)
	default:
		fmt.Fprintln(os.Stderr, "wittymediaplayer-engine helps the WittyMediaPlayer window.\n  --drives                  list optical drives\n  --image <source> <out>    copy a disc to an .iso or .img (protected discs are refused)")
		os.Exit(2)
	}
}

// ------------------------------------------------------------------ helper

type helper struct {
	session disc.Session
}

func (h *helper) closeSession() {
	if h.session != nil {
		_ = h.session.Close()
		h.session = nil
	}
}

// openReply answers an "open" call.
type openReply struct {
	Kind         string             `json:"kind"`
	Info         *disc.Info         `json:"info,omitempty"`
	Presentation *disc.Presentation `json:"presentation,omitempty"`
}

func refusal(err error) error {
	var r *disc.RefusalError
	if errors.As(err, &r) {
		return &wittymodule.CallError{Code: r.Code, Message: r.Message}
	}
	return err
}

// resolve turns what the viewer opened into a path and kind, or a plain refusal.
func resolve(path string) (string, string, bool, error) {
	drive := false
	if drives.IsDriveRoot(path) {
		path = drives.NormalizeRoot(path)
	}
	if drives.IsDrive(path) {
		drive = true
		if !drives.HasMedia(path) {
			return "", "", true, &disc.RefusalError{Code: disc.CodeNoMedia, Message: disc.MsgNoMedia}
		}
	}
	kind, err := disc.Classify(path)
	if err != nil {
		return "", "", drive, err
	}
	if kind == disc.KindData {
		if drive && audioCD(path) {
			return "", "", true, &disc.RefusalError{Code: disc.CodeAudioCD, Message: disc.MsgAudioCD}
		}
		return "", "", drive, &disc.RefusalError{Code: disc.CodeNotVideo, Message: disc.MsgNotVideo}
	}
	if kind == disc.KindDVD && drive && drives.IsDriveRoot(path) && drives.Protection(path) == drives.ProtectionCSS {
		return "", "", true, &disc.RefusalError{Code: disc.CodeProtected, Message: disc.MsgDVDProtected}
	}
	return path, kind, drive, nil
}

func audioCD(path string) bool {
	list, _ := drives.List()
	for _, d := range list {
		if strings.EqualFold(d.Path, filepath.VolumeName(path)+`\`) || d.Path == path {
			return d.Audio
		}
	}
	return false
}

func (h *helper) handle(method string, raw json.RawMessage) (any, []byte, error) {
	switch method {
	case "open":
		var in struct {
			Path  string     `json:"path"`
			Prefs disc.Prefs `json:"prefs"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		h.closeSession()
		path, kind, drive, err := resolve(in.Path)
		if err != nil {
			return nil, nil, refusal(err)
		}
		if kind == disc.KindFile {
			return openReply{Kind: kind}, nil, nil
		}
		s, err := disc.Open(path, kind, drive, in.Prefs)
		if err != nil {
			return nil, nil, refusal(err)
		}
		h.session = s
		info := s.Info()
		return openReply{Kind: kind, Info: &info, Presentation: s.Start()}, nil, nil
	case "drives":
		list, err := drives.List()
		return list, nil, err
	}
	if h.session == nil {
		return nil, nil, errors.New("no disc is open")
	}
	switch method {
	case "action":
		var a disc.Action
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, nil, err
		}
		return h.session.Do(a), nil, nil
	case "title":
		var in struct {
			TTN     int `json:"ttn"`
			Chapter int `json:"chapter"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		return h.session.PlayTitle(in.TTN, in.Chapter), nil, nil
	case "locate":
		var in struct {
			Sec float64 `json:"sec"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		loc, err := h.session.Locate(in.Sec)
		return loc, nil, err
	case "art":
		var in struct {
			Art   int    `json:"art"`
			Kind  string `json:"kind"`
			N     int    `json:"n"`
			Frame int    `json:"frame"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.Art(in.Art, in.Kind, in.N, in.Frame)
		if !ok {
			return nil, nil, &wittymodule.CallError{Code: "missing", Message: "no such picture"}
		}
		return struct{}{}, b, nil
	case "sound":
		var in struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.Sound(in.ID)
		if !ok {
			return nil, nil, &wittymodule.CallError{Code: "missing", Message: "no such sound"}
		}
		return struct{}{}, b, nil
	case "subs":
		var in struct {
			TTN    int     `json:"ttn"`
			Stream int     `json:"stream"`
			From   float64 `json:"from"`
			To     float64 `json:"to"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		ev, err := h.session.Subtitles(in.TTN, in.Stream, in.From, in.To)
		return ev, nil, err
	case "subpic":
		var in struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.SubPic(in.Key)
		if !ok {
			return nil, nil, &wittymodule.CallError{Code: "missing", Message: "no such picture"}
		}
		return struct{}{}, b, nil
	case "close":
		h.closeSession()
		return struct{}{}, nil, nil
	}
	return nil, nil, fmt.Errorf("unknown call %q", method)
}

// ------------------------------------------------------------------ imaging

// runImage copies a disc, printing one JSON line per report. Closing stdin stops
// it, and the partial file goes with it.
func runImage(source, out string, stdin io.Reader, stdout io.Writer) int {
	enc := json.NewEncoder(stdout)
	fail := func(code, msg string) int {
		_ = enc.Encode(imaging.Progress{Event: "error", Code: code, Message: msg})
		return 1
	}
	path, kind, drive := source, "", false
	if drives.IsDriveRoot(path) {
		path, drive = drives.NormalizeRoot(path), true
	}
	if drive && !drives.HasMedia(path) {
		return fail(disc.CodeNoMedia, disc.MsgNoMedia)
	}
	// Writing the image onto the disc being copied can't work, and on a hard disk
	// it would be a surprise to overwrite the source.
	if abs, err := filepath.Abs(out); err == nil && drive && strings.EqualFold(filepath.VolumeName(abs), path[:2]) {
		return fail("destination", "The image can't be saved on the disc it's copied from. Pick another folder.")
	}
	k, err := disc.Classify(path)
	if err != nil {
		return fail(disc.CodeUnreadable, "This disc couldn't be read: "+err.Error())
	}
	kind = k
	if kind == disc.KindFile {
		return fail(disc.CodeNotVideo, "Only discs and disc images can be copied.")
	}
	if drive && audioCD(path) {
		return fail(disc.CodeAudioCD, "This is an audio CD, which has no files to put in an image.")
	}
	if drive && kind == disc.KindDVD && drives.Protection(path) == drives.ProtectionCSS {
		return fail(disc.CodeProtected, disc.MsgDVDProtected)
	}
	if err := disc.CheckProtected(path, kind, drive); err != nil {
		var r *disc.RefusalError
		if errors.As(err, &r) {
			return fail(r.Code, r.Message)
		}
		return fail(disc.CodeUnreadable, "This disc couldn't be read: "+err.Error())
	}
	src, closeSrc, err := openSource(path, drive)
	if err != nil {
		return fail(disc.CodeUnreadable, "This disc couldn't be read: "+err.Error())
	}
	defer closeSrc()
	if err := imaging.CheckOut(out, src.Size()); err != nil {
		return fail("destination", capitalize(err.Error())+".")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_, _ = io.Copy(io.Discard, stdin)
		cancel()
	}()
	res, err := imaging.Copy(ctx, src, imaging.Options{Out: out, Kind: kind, Progress: func(p imaging.Progress) { _ = enc.Encode(p) }})
	if err != nil {
		var r *disc.RefusalError
		switch {
		case errors.As(err, &r):
			return fail(r.Code, r.Message)
		case errors.Is(err, context.Canceled):
			return fail("cancelled", "The copy was stopped, and the partial file removed.")
		}
		return fail("failed", capitalize(err.Error())+".")
	}
	_ = enc.Encode(res)
	return 0
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// fileSource reads an image file as if it were a disc.
type fileSource struct {
	*os.File
	size int64
}

func (f fileSource) Size() int64 { return f.size }

func openSource(path string, drive bool) (imaging.Source, func(), error) {
	if drive {
		d, err := drives.OpenRaw(path)
		if err != nil {
			return nil, nil, err
		}
		return d, func() { d.Close() }, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	if st.IsDir() {
		f.Close()
		return nil, nil, errors.New("a folder has no sectors to copy; open it to play it instead")
	}
	return fileSource{f, st.Size()}, func() { f.Close() }, nil
}
