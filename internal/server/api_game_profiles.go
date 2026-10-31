package server

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	stddraw "image/draw"
	"image/jpeg"
	_ "image/png" // decoders for checking uploaded backgrounds
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittyphotos"
	"github.com/wittywilla/wittyplay"
)

// Controller profiles and the picture behind the controller screen. Both belong to
// whoever is signed in and to nobody else: no route here takes a person's id, so there
// is nothing to change to reach someone else's. See docs/GAMING-API.md.

// gameProfileRoutes registers them.
func (s *Server) gameProfileRoutes(m *http.ServeMux) {
	a := s.requireUser
	m.Handle("GET /api/game-controllers", a(s.handleGameControllers))
	m.Handle("GET /api/game-profiles", a(s.handleGameProfiles))
	m.Handle("POST /api/game-profiles", a(s.handleGameProfileCreate))
	m.Handle("GET /api/game-profiles/resolve", a(s.handleGameProfileResolve))
	m.Handle("GET /api/game-profiles/export", a(s.handleGameProfileExport))
	m.Handle("POST /api/game-profiles/import", a(s.handleGameProfileImport))
	m.Handle("GET /api/game-profiles/{id}", a(s.handleGameProfileGet))
	m.Handle("PUT /api/game-profiles/{id}", a(s.handleGameProfilePut))
	m.Handle("DELETE /api/game-profiles/{id}", a(s.handleGameProfileDelete))
	m.Handle("POST /api/game-profiles/{id}/copy", a(s.handleGameProfileCopy))
	m.Handle("POST /api/game-profiles/{id}/active", a(s.handleGameProfileActive))
	m.Handle("GET /api/game-background", a(s.handleGameBackgroundGet))
	m.Handle("PUT /api/game-background", a(s.handleGameBackgroundPut))
	m.Handle("DELETE /api/game-background", a(s.handleGameBackgroundDelete))
	m.Handle("GET /api/game-background/image", a(s.handleGameBackgroundImage))
	m.Handle("PUT /api/game-background/image", a(s.handleGameBackgroundUpload))
}

// GET /api/game-controllers: the controllers this server knows by vendor and product.
func (s *Server) handleGameControllers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, max-age=3600")
	writeJSON(w, 200, wittyplay.KnownControllers())
}

func (s *Server) consoleKnown(id string) bool { return s.gameConsole(id) != nil }

// cleanGameProfile checks a profile from a client, and that its game is one this
// account can see, on the console it says.
func (s *Server) cleanGameProfile(r *http.Request, p wittyplay.Profile) (wittyplay.Profile, error) {
	clean, err := wittyplay.CleanProfile(p, s.consoleKnown)
	if err != nil {
		return p, err
	}
	if clean.Game != 0 {
		it, err := s.visibleItem(r, clean.Game)
		if err != nil || it.Kind != "game" || it.Console != clean.Console {
			return p, errors.New("that game isn't in your libraries")
		}
	}
	return clean, nil
}

func (s *Server) profileError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, library.ErrNoProfile):
		writeErr(w, 404, "not found")
	case errors.Is(err, library.ErrTooManyProfiles):
		writeErr(w, 409, err.Error())
	default:
		writeErr(w, 500, err.Error())
	}
}

func readProfile(w http.ResponseWriter, r *http.Request) (wittyplay.Profile, bool) {
	var p wittyplay.Profile
	r.Body = http.MaxBytesReader(w, r.Body, wittyplay.MaxProfileSize)
	if err := readJSON(r, &p); err != nil {
		writeErr(w, 400, "this profile can't be read")
		return p, false
	}
	return p, true
}

// GET /api/game-profiles?kind=&console=
func (s *Server) handleGameProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.GameProfiles(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	q := r.URL.Query()
	out := list[:0]
	for _, p := range list {
		if (q.Get("kind") == "" || q.Get("kind") == p.Kind) && (q.Get("console") == "" || p.Console == "" || q.Get("console") == p.Console) {
			out = append(out, p)
		}
	}
	writeJSON(w, 200, out)
}

// POST /api/game-profiles
func (s *Server) handleGameProfileCreate(w http.ResponseWriter, r *http.Request) {
	p, ok := readProfile(w, r)
	if !ok {
		return
	}
	p.ID = 0
	clean, err := s.cleanGameProfile(r, p)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	saved, err := s.Store.SaveGameProfile(r.Context(), userFrom(r).ID, clean)
	if err != nil {
		s.profileError(w, err)
		return
	}
	writeJSON(w, 201, saved)
}

func (s *Server) ownProfile(w http.ResponseWriter, r *http.Request) (wittyplay.Profile, bool) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 404, "not found")
		return wittyplay.Profile{}, false
	}
	p, err := s.Store.GameProfile(r.Context(), userFrom(r).ID, id)
	if err != nil {
		s.profileError(w, err)
		return p, false
	}
	return p, true
}

// GET /api/game-profiles/{id}
func (s *Server) handleGameProfileGet(w http.ResponseWriter, r *http.Request) {
	if p, ok := s.ownProfile(w, r); ok {
		writeJSON(w, 200, p)
	}
}

// PUT /api/game-profiles/{id}
func (s *Server) handleGameProfilePut(w http.ResponseWriter, r *http.Request) {
	cur, ok := s.ownProfile(w, r)
	if !ok {
		return
	}
	p, ok := readProfile(w, r)
	if !ok {
		return
	}
	p.ID = cur.ID
	clean, err := s.cleanGameProfile(r, p)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	saved, err := s.Store.SaveGameProfile(r.Context(), userFrom(r).ID, clean)
	if err != nil {
		s.profileError(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

// DELETE /api/game-profiles/{id}
func (s *Server) handleGameProfileDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt(r, "id")
	if !ok {
		writeErr(w, 404, "not found")
		return
	}
	if err := s.Store.DeleteGameProfile(r.Context(), userFrom(r).ID, id); err != nil {
		s.profileError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// copyName is a profile's name with "copy" after it, still fitting.
func copyName(name string) string {
	const suffix = " copy"
	r := []rune(name)
	if max := 60 - len(suffix); len(r) > max {
		r = r[:max]
	}
	return string(r) + suffix
}

// POST /api/game-profiles/{id}/copy: a copy that isn't in use yet.
func (s *Server) handleGameProfileCopy(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownProfile(w, r)
	if !ok {
		return
	}
	p.ID, p.Active, p.Name = 0, false, copyName(p.Name)
	saved, err := s.Store.SaveGameProfile(r.Context(), userFrom(r).ID, p)
	if err != nil {
		s.profileError(w, err)
		return
	}
	writeJSON(w, 201, saved)
}

// POST /api/game-profiles/{id}/active with {"active": bool}
func (s *Server) handleGameProfileActive(w http.ResponseWriter, r *http.Request) {
	p, ok := s.ownProfile(w, r)
	if !ok {
		return
	}
	var b struct {
		Active bool `json:"active"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	p.Active = b.Active
	saved, err := s.Store.SaveGameProfile(r.Context(), userFrom(r).ID, p)
	if err != nil {
		s.profileError(w, err)
		return
	}
	writeJSON(w, 200, saved)
}

// GET /api/game-profiles/resolve?kind=gamepad&controller=&console=&game=&raw=1: what a
// client should use for this controller here.
func (s *Server) handleGameProfileResolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	console := q.Get("console")
	if s.gameConsole(console) == nil {
		writeErr(w, 404, "not found")
		return
	}
	kind := q.Get("kind")
	if kind == "" {
		kind = wittyplay.ProfileGamepad
	}
	if kind != wittyplay.ProfileGamepad && kind != wittyplay.ProfileKeyboard {
		writeErr(w, 400, "a profile is for a gamepad or a keyboard")
		return
	}
	game, _ := strconv.ParseInt(q.Get("game"), 10, 64)
	if game != 0 {
		if it, err := s.visibleItem(r, game); err != nil || it.Kind != "game" {
			game = 0 // a game this account can't see counts as no game
		}
	}
	list, err := s.Store.GameProfiles(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	controller := q.Get("controller")
	if kind == wittyplay.ProfileKeyboard {
		controller = wittyplay.ControllerKeyboard
	}
	p := wittyplay.Resolve(list, kind, controller, console, game, q.Get("raw") == "1")
	out := map[string]any{"profile": p, "options": wittyplay.EffectiveOptions(console, p)}
	if kind == wittyplay.ProfileKeyboard {
		out["keys"] = wittyplay.EffectiveKeys(console, p)
	} else {
		out["map"] = wittyplay.EffectiveMap(console, p)
	}
	writeJSON(w, 200, out)
}

// GET /api/game-profiles/export?id=1&id=2: a file of these profiles, or of all of them.
func (s *Server) handleGameProfileExport(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.GameProfiles(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	want := map[string]bool{}
	for _, id := range r.URL.Query()["id"] {
		want[id] = true
	}
	f := wittyplay.ProfileFile{Format: wittyplay.ProfileFormat, Version: 1, Profiles: []wittyplay.Profile{}}
	for _, p := range list {
		if len(want) > 0 && !want[strconv.FormatInt(p.ID, 10)] {
			continue
		}
		p.ID, p.UpdatedAt, p.Active = 0, 0, false // these mean something only here
		f.Profiles = append(f.Profiles, p)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="wittyplay-controller-profiles.json"`)
	writeJSON(w, 200, f)
}

type importRejected struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Error string `json:"error"`
}

// POST /api/game-profiles/import: a file from export, entirely untrusted. Good profiles
// are kept and each bad one is said why; a profile takes effect only if nothing else
// with its scope is in use.
func (s *Server) handleGameProfileImport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, wittyplay.MaxImportSize))
	if err != nil {
		writeErr(w, 413, "this file is too large")
		return
	}
	entries, err := wittyplay.ParseProfileFile(body)
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	ctx, userID := r.Context(), userFrom(r).ID
	existing, err := s.Store.GameProfiles(ctx, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	scope := func(p wittyplay.Profile) string {
		return fmt.Sprint(p.Kind, "\x00", p.Controller, "\x00", p.Console, "\x00", p.Game)
	}
	inUse := map[string]bool{}
	for _, p := range existing {
		if p.Active {
			inUse[scope(p)] = true
		}
	}
	imported, rejected, notes := []wittyplay.Profile{}, []importRejected{}, []string{}
	for i, e := range entries {
		e.ID = 0
		if e.Game != 0 { // another server's numbers: only the same title is the same game
			title := e.GameTitle
			if it, err := s.visibleItem(r, e.Game); err != nil || it.Kind != "game" || it.Console != e.Console || it.Title != title {
				e.Game = 0
				notes = append(notes, fmt.Sprintf("“%s” was for a game that isn't here, so it now applies to the whole console.", wittyplay.ShortText(e.Name)))
			}
		}
		clean, err := s.cleanGameProfile(r, e)
		if err != nil {
			rejected = append(rejected, importRejected{Index: i, Name: wittyplay.ShortText(e.Name), Error: err.Error()})
			continue
		}
		clean.Active = !inUse[scope(clean)]
		saved, err := s.Store.SaveGameProfile(ctx, userID, clean)
		if err != nil {
			if errors.Is(err, library.ErrTooManyProfiles) {
				rejected = append(rejected, importRejected{Index: i, Name: clean.Name, Error: err.Error()})
				continue
			}
			writeErr(w, 500, err.Error())
			return
		}
		if saved.Active {
			inUse[scope(saved)] = true
		}
		imported = append(imported, saved)
	}
	writeJSON(w, 200, map[string]any{"imported": imported, "rejected": rejected, "notes": notes})
}

// ---- the background ----

// Limits on an upload. Decoding an image takes far more memory than its file, so its
// size in pixels is checked from its header first, and one is decoded at a time.
const (
	maxBackgroundBytes  = 10 << 20
	maxBackgroundPixels = 24_000_000
	maxBackgroundSide   = 8192
	minBackgroundSide   = 16
	backgroundOutSide   = 2560 // what's kept: plenty for a screen, and small to send
)

var backgroundFormats = map[string]bool{"png": true, "jpeg": true, "webp": true}

var errBackgroundFormat = errors.New("use a PNG, JPEG or WebP image")

var decodeBackgroundSlot = make(chan struct{}, 1)

func (s *Server) backgroundPath(userID int64) string {
	return filepath.Join(s.Cfg.DataDir, "gaming", "backgrounds", "u"+strconv.FormatInt(userID, 10)+".jpg")
}

// cleanBackground decodes an upload and draws it again as a new JPEG, so that all that
// survives is pixels: nothing the file carried beyond its picture, whatever it says it
// is. Its header is read before anything else, and refused if it's too big to decode.
func cleanBackground(b []byte) (out []byte, err error) {
	defer func() {
		if p := recover(); p != nil { // a decoder tripped on a hostile file
			out, err = nil, errBackgroundFormat
		}
	}()
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || !backgroundFormats[format] {
		return nil, errBackgroundFormat
	}
	if cfg.Width < minBackgroundSide || cfg.Height < minBackgroundSide {
		return nil, fmt.Errorf("the picture must be at least %d pixels on each side", minBackgroundSide)
	}
	if cfg.Width > maxBackgroundSide || cfg.Height > maxBackgroundSide || cfg.Width*cfg.Height > maxBackgroundPixels {
		return nil, errors.New("the picture has too many pixels: use one of up to 24 megapixels")
	}
	src, decoded, err := image.Decode(bytes.NewReader(b))
	if err != nil || decoded != format {
		return nil, errBackgroundFormat
	}
	orientation := 1
	if format == "jpeg" { // phones store upright photos sideways, and say so in the part we drop
		if info, err := wittyphotos.Parse(b); err == nil && info.Orientation >= 2 && info.Orientation <= 8 {
			orientation = info.Orientation
		}
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if long := max(w, h); long > backgroundOutSide {
		w, h = max(1, w*backgroundOutSide/long), max(1, h*backgroundOutSide/long)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	stddraw.Draw(dst, dst.Bounds(), image.Black, image.Point{}, stddraw.Src) // transparency becomes black
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, sb, draw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, upright(dst, orientation), &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// upright turns a picture by its EXIF orientation, 1 to 8.
func upright(src *image.RGBA, orientation int) image.Image {
	if orientation <= 1 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dw, dh := w, h
	if orientation >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch orientation {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.SetRGBA(dx, dy, src.RGBAAt(x, y))
		}
	}
	return dst
}

// GET /api/game-background
func (s *Server) handleGameBackgroundGet(w http.ResponseWriter, r *http.Request) {
	b, err := s.Store.GetGameBackground(r.Context(), userFrom(r).ID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, b)
}

// PUT /api/game-background: position, fit and dimming.
func (s *Server) handleGameBackgroundPut(w http.ResponseWriter, r *http.Request) {
	var in library.GameBackground
	if err := readJSON(r, &in); err != nil {
		writeErr(w, 400, "this setting can't be read")
		return
	}
	if (in.Fit != "cover" && in.Fit != "contain") || in.X < 0 || in.X > 100 || in.Y < 0 || in.Y > 100 || in.Dim < 0 || in.Dim > 90 {
		writeErr(w, 400, "this setting is out of range")
		return
	}
	ctx, userID := r.Context(), userFrom(r).ID
	cur, err := s.Store.GetGameBackground(ctx, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	in.Version = cur.Version // which picture it is isn't for a client to say
	if err := s.Store.SetGameBackground(ctx, userID, in); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, in)
}

// PUT /api/game-background/image: the image itself as the body.
func (s *Server) handleGameBackgroundUpload(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBackgroundBytes))
	if err != nil {
		writeErr(w, 413, "the picture must be 10 MB or smaller")
		return
	}
	ctx, userID := r.Context(), userFrom(r).ID
	select {
	case decodeBackgroundSlot <- struct{}{}:
	case <-ctx.Done():
		return
	}
	clean, err := cleanBackground(b)
	<-decodeBackgroundSlot
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	dst := s.backgroundPath(userID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, clean, 0o644); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		writeErr(w, 500, err.Error())
		return
	}
	cur, err := s.Store.GetGameBackground(ctx, userID)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	cur.Version = time.Now().UnixMilli()
	if err := s.Store.SetGameBackground(ctx, userID, cur); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, cur)
}

// GET /api/game-background/image: only ever the signed-in person's own.
func (s *Server) handleGameBackgroundImage(w http.ResponseWriter, r *http.Request) {
	path := s.backgroundPath(userFrom(r).ID)
	if _, err := os.Stat(path); err != nil {
		writeErr(w, 404, "no picture")
		return
	}
	// The address carries ?v=, and it's private to this account. The sandboxing headers
	// every API answer has stay on it, and it's only ever sent as a JPEG.
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, path)
}

// DELETE /api/game-background
func (s *Server) handleGameBackgroundDelete(w http.ResponseWriter, r *http.Request) {
	userID := userFrom(r).ID
	if err := os.Remove(s.backgroundPath(userID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		writeErr(w, 500, err.Error())
		return
	}
	if err := s.Store.RemoveGameBackground(r.Context(), userID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, library.DefaultGameBackground())
}

// removeUserGameFiles is for when an account goes: its rows go with it, and this is
// the one file of ours they don't cover.
func (s *Server) removeUserGameFiles(userID int64) {
	_ = os.Remove(s.backgroundPath(userID))
}

// activeProfilesFor is what the controls route sends: a person's profiles in use for a
// console and game, whichever controller they're for.
func activeProfilesFor(list []wittyplay.Profile, console string, game int64) []wittyplay.Profile {
	out := []wittyplay.Profile{}
	for _, p := range list {
		if p.Active && (p.Console == "" || p.Console == console) && (p.Game == 0 || p.Game == game) {
			out = append(out, p)
		}
	}
	return out
}
