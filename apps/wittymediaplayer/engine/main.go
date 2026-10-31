// wittymediaplayer-engine is the helper behind the WittyMediaPlayer window. It
// reads disc structures and runs the disc engines in a process of its own, so a
// damaged or hostile disc can't take the window down, and it needs nothing but
// its stdin and stdout: it has no network code at all.
//
//	wittymediaplayer-engine --helper                 the window's helper
//	wittymediaplayer-engine --drives                 lists optical drives as JSON
//	wittymediaplayer-engine --image <source> <out>   copies a disc to an .iso or .img
//	wittymediaplayer-engine --cd-image <source> <out.cue> [secure|burst] [offset]
//
// Ctrl+C stops a copy. With --watch-stdin before --image, so does closing stdin, which
// is how the window does it. Either way the partial file is removed.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wittywilla/wittydisc/session"
	"github.com/wittywilla/wittymediaplayer/engine/internal/cdrip"
	"github.com/wittywilla/wittymediaplayer/engine/internal/disc"
	"github.com/wittywilla/wittymediaplayer/engine/internal/drives"
	"github.com/wittywilla/wittymediaplayer/engine/internal/imaging"
	"github.com/wittywilla/wittymediaplayer/engine/internal/modhost"
	"github.com/wittywilla/wittymodule"
)

// Version is the engine's version, set by the build.
var Version = "0.1.0"

// HelperFlag starts the helper protocol.
const HelperFlag = "--helper"

func main() {
	args := os.Args[1:]
	watch := false
	if len(args) > 0 && args[0] == "--watch-stdin" {
		watch, args = true, args[1:]
	}
	if len(args) == 0 || args[0] != HelperFlag {
		attachConsole() // started by hand: answer in the terminal it came from
	}
	switch {
	case len(args) == 1 && args[0] == HelperFlag:
		h := &helper{mgr: disc.NewManager(Version)}
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
		var stdin io.Reader
		if watch {
			stdin = os.Stdin
		}
		os.Exit(runImage(args[1], args[2], stdin, os.Stdout))
	case len(args) >= 3 && args[0] == "--cd-image":
		var stdin io.Reader
		if watch {
			stdin = os.Stdin
		}
		secure, offset := true, 0
		if len(args) > 3 {
			secure = args[3] != "burst"
		}
		if len(args) > 4 {
			offset, _ = strconv.Atoi(args[4])
		}
		os.Exit(runCDImage(args[1], args[2], secure, offset, stdin, os.Stdout))
	case len(args) >= 1 && args[0] == "--cd-rip":
		var stdin io.Reader
		if watch {
			stdin = os.Stdin
		}
		os.Exit(runCDRip(args[1:], stdin, os.Stdout))
	case len(args) == 2 && args[0] == "--cd-stream":
		os.Exit(runCDStream(args[1], os.Stdin, os.Stdout))
	case len(args) == 1 && args[0] == "--version":
		fmt.Println("wittymediaplayer-engine", Version)
	default:
		fmt.Fprintln(os.Stderr, "wittymediaplayer-engine helps the WittyMediaPlayer window.\n  --drives                  list optical drives\n  --image <source> <out>    copy a disc to an .iso or .img (protected discs are refused)")
		os.Exit(2)
	}
}

// ------------------------------------------------------------------ helper

// helper answers the window. Calls about a disc go to its module: a downloaded version's
// own process when one is in use, or the built-in copy.
type helper struct {
	mgr     *modhost.Manager
	builtin *session.Host
	remote  *modhost.Remote
	mod     modhost.Module
	shaky   bool // a call has failed since the last that worked
}

func (h *helper) closeSession() {
	if h.builtin != nil {
		h.builtin.Close()
		h.builtin = nil
	}
	if h.remote != nil {
		h.remote.Stop()
		h.remote = nil
	}
}

// failed counts a failure at the process level against the version in use.
func (h *helper) failed(err error) {
	h.remote.Stop()
	h.mgr.CallFailed(h.mod, h.remote.Version, err.Error())
	h.remote = nil
	h.shaky = true
}

// call passes a call to the module in use.
func (h *helper) call(method string, raw json.RawMessage) (any, []byte, error) {
	if h.remote != nil {
		res, err := h.remote.Call(method, raw)
		var ce *wittymodule.CallError
		switch {
		case err == nil:
			if h.shaky {
				h.mgr.CallOK(h.mod)
				h.shaky = false
			}
		case errors.As(err, &ce):
			return nil, nil, err
		default:
			h.failed(err)
			return nil, nil, errors.New("this disc's module stopped unexpectedly. Open the disc again to carry on")
		}
		if len(res.JSON) == 0 {
			return struct{}{}, res.Blob, nil
		}
		return res.JSON, res.Blob, nil
	}
	if h.builtin == nil {
		return nil, nil, errors.New("no disc is open")
	}
	res, blob, err := h.builtin.Handle(method, raw)
	return res, blob, refusal(err)
}

// openDisc starts a disc with its module. A downloaded version gets the same refusal
// check as anything else first, by the built-in copy, so what it says about copy
// protection is never simply taken on trust. If it can't start or fails on the way in,
// the built-in copy carries on.
func (h *helper) openDisc(path, kind string, drive bool, prefs json.RawMessage) (any, []byte, error) {
	h.mod, _ = h.mgr.Module(disc.ModuleFor(kind))
	params, err := json.Marshal(map[string]any{"path": path, "drive": drive, "prefs": prefs})
	if err != nil {
		return nil, nil, err
	}
	if c := h.mgr.Choose(h.mod); c.Entry != "" {
		if err := disc.CheckProtected(path, kind, drive); err != nil {
			return nil, nil, refusal(err)
		}
		rem, err := modhost.Start(c.Entry, h.mod.ID, c.Version)
		if err != nil {
			h.mgr.CallFailed(h.mod, c.Version, err.Error())
		} else {
			h.remote = rem
			res, _, err := h.call("open", params)
			if err == nil {
				return withKind(res, kind)
			}
			var ce *wittymodule.CallError
			if errors.As(err, &ce) {
				h.closeSession()
				return nil, nil, err
			}
		}
	}
	h.builtin = disc.BuiltinHost(kind)
	res, _, err := h.call("open", params)
	if err != nil {
		h.closeSession()
		return nil, nil, err
	}
	return withKind(res, kind)
}

// withKind adds what kind of disc it is to a module's answer.
func withKind(res any, kind string) (any, []byte, error) {
	b, err := json.Marshal(res)
	if err != nil {
		return nil, nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, nil, err
	}
	k, _ := json.Marshal(kind)
	out["kind"] = k
	return out, nil, nil
}

// openReply answers an "open" call.
type openReply struct {
	Kind         string             `json:"kind"`
	Info         *disc.Info         `json:"info,omitempty"`
	Presentation *disc.Presentation `json:"presentation,omitempty"`
	CD           *cdrip.Info        `json:"cd,omitempty"`
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
	if errors.Is(err, os.ErrNotExist) {
		return "", "", drive, &disc.RefusalError{Code: disc.CodeUnreadable, Message: "This can't be found. It may have been moved, or the disc taken out."}
	}
	if err != nil {
		return "", "", drive, err
	}
	if kind == disc.KindData {
		if drive && audioCD(path) {
			return "", "", true, &disc.RefusalError{Code: disc.CodeAudioCD, Message: cdrip.ErrNoRead.Error()}
		}
		return "", "", drive, &disc.RefusalError{Code: disc.CodeNotVideo, Message: disc.MsgNotVideo}
	}
	if kind == disc.KindDVD && drive && drives.IsDriveRoot(path) && drives.Protection(path) == drives.ProtectionCSS {
		return "", "", true, &disc.RefusalError{Code: disc.CodeProtected, Message: disc.MsgDVDProtected}
	}
	return path, kind, drive, nil
}

// openAudioCD says what an audio CD holds, from a drive or an image. Anything else, even a
// disc image of another kind, is for the rest of open to look at: it returns nothing for
// those, and an error only for what's clearly an audio CD and won't read.
func openAudioCD(path string) (*cdrip.Info, error) {
	isDrive := drives.IsDriveRoot(path)
	sure := isDrive
	switch {
	case isDrive:
		path = drives.NormalizeRoot(path)
		if !audioCD(path) {
			return nil, nil
		}
	case cdrip.IsImage(path):
		sure = true
	case !cdrip.MayBeImage(path):
		return nil, nil
	default:
		// .iso, .img and .bin may be a video disc or data; those go the usual way.
		if k, err := disc.Classify(path); err == nil && k != disc.KindData && k != disc.KindFile {
			return nil, nil
		}
	}
	src, err := cdrip.Open(path)
	if err != nil {
		if !sure {
			return nil, nil
		}
		msg := capitalize(err.Error())
		if !strings.HasSuffix(msg, ".") {
			msg += "."
		}
		return nil, &disc.RefusalError{Code: disc.CodeUnreadable, Message: msg}
	}
	defer src.Close()
	info := *src.Info()
	info.Path = path
	return &info, nil
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
			Path  string          `json:"path"`
			Prefs json.RawMessage `json:"prefs"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		h.closeSession()
		cd, err := openAudioCD(in.Path)
		if err != nil {
			return nil, nil, refusal(err)
		}
		if cd != nil {
			return openReply{Kind: "audiocd", CD: cd}, nil, nil
		}
		path, kind, drive, err := resolve(in.Path)
		if err != nil {
			return nil, nil, refusal(err)
		}
		if kind == disc.KindFile {
			return openReply{Kind: kind}, nil, nil
		}
		return h.openDisc(path, kind, drive, in.Prefs)
	case "drives":
		list, err := drives.List()
		return list, nil, err
	case "modules":
		return h.mgr.Statuses(), nil, nil
	case "module-rollback":
		var in struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		mod, ok := h.mgr.Module(in.ID)
		if !ok {
			return nil, nil, fmt.Errorf("%q isn't one of this player's modules", in.ID)
		}
		v, err := h.mgr.Rollback(mod)
		if err != nil {
			return nil, nil, &wittymodule.CallError{Code: "nothing-to-do", Message: capitalize(err.Error()) + "."}
		}
		return map[string]string{"inUse": v}, nil, nil
	case "close":
		h.closeSession()
		return struct{}{}, nil, nil
	}
	return h.call(method, raw)
}

// ------------------------------------------------------------------ imaging

// runImage copies a disc, printing one JSON line per report. Ctrl+C stops it, as
// does stdin closing when stop is given, and the partial file goes with it.
func runImage(source, out string, stop io.Reader, stdout io.Writer) int {
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if stop != nil {
		go func() {
			_, _ = io.Copy(io.Discard, stop)
			cancel()
		}()
	}
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
