package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
	"github.com/wittywilla/wittymoonlight"
	"github.com/wittywilla/wittysunshine"
)

// Playing a rig's game in a browser, through a separate program (the bridge) that an admin
// chooses to download. It and the port in front of it start when someone presses Play, and stop
// when nobody has used them for a while, so a server pays nothing until this is used. See
// docs/GAME-RIGS.md for why it has a port of its own.

type rigBrowser struct {
	s      *Server
	bridge *wittymoonlight.Bridge
	proxy  *wittymoonlight.Proxy

	mu     sync.Mutex
	srv    *http.Server
	port   int
	secure bool
	stopCh chan struct{}
}

func (s *Server) browser() *rigBrowser {
	g := s.rigs()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browser != nil {
		return g.browser
	}
	b := &rigBrowser{s: s}
	b.bridge = &wittymoonlight.Bridge{Dir: filepath.Join(g.dir, "browser"), Client: library.SafeClient}
	b.proxy = wittymoonlight.NewProxy(b.bridge.Port, wittymoonlight.Hooks{MayStart: s.rigMayStart, Started: s.rigStreamStarted})
	g.browser = b
	return b
}

// rigMayStart is the last check before a stream starts, as the ticket may be hours old: the
// person still exists, may still see this game, and isn't walking into someone else's.
func (s *Server) rigMayStart(ctx context.Context, sess wittymoonlight.Session) error {
	u, err := s.Auth.User(ctx, sess.User)
	if err != nil || u == nil {
		return errors.New("that account can't play here any more")
	}
	rig, err := s.rigByID(ctx, sess.RigID)
	if err != nil {
		return errors.New("that PC has been removed")
	}
	var itemID int64
	if err := s.DB.QueryRowContext(ctx, `SELECT item_id FROM wittysunshine_apps WHERE rig_id = ? AND app_id = ? AND shown = 1`, sess.RigID, sess.AppID).Scan(&itemID); err != nil {
		return errors.New("that game isn't available any more")
	}
	it, err := s.Store.Item(ctx, itemID)
	if err != nil {
		return errors.New("that game isn't available any more")
	}
	if a := accessFor(u); a != nil {
		var sh *library.Show
		if !a.AllowsItem(it, sh) {
			return errors.New("that account can't play this game any more")
		}
	}
	st := s.rigStatusNow(ctx, rig)
	if st.State != "online" && st.State != "busy" {
		return errors.New(st.Message)
	}
	if !s.rigMayJoin(rig, st, sess.AppID, u.ID, u.IsAdmin) {
		return errors.New("something else is running on that PC, so this can't start until it's closed")
	}
	return nil
}

func (s *Server) rigStreamStarted(sess wittymoonlight.Session) {
	g := s.rigs()
	g.mu.Lock()
	g.starter[sess.RigID] = sess.User
	delete(g.statuses, sess.RigID)
	g.mu.Unlock()
}

func rigSession(user, rig int64, app int) wittymoonlight.Session {
	return wittymoonlight.Session{User: user, RigID: rig, AppID: app}
}

// rigUDP reads the WebRTC port range, "47910-47920".
func rigUDP(v string) (int, int) {
	a, b, ok := strings.Cut(v, "-")
	if !ok {
		return 0, 0
	}
	lo, err1 := strconv.Atoi(strings.TrimSpace(a))
	hi, err2 := strconv.Atoi(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || lo < 1024 || hi > 65535 || hi < lo {
		return 0, 0
	}
	return lo, hi
}

// start runs the bridge and its port if they aren't running.
func (b *rigBrowser) start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.s
	get := func(k, def string) string { return s.DB.GetSetting(ctx, k, def) }
	b.bridge.UDPMin, b.bridge.UDPMax = rigUDP(get("rigBrowserUdp", settingDefaults["rigBrowserUdp"]))
	b.bridge.PublicIP = strings.TrimSpace(get("rigBrowserPublicIp", ""))
	b.bridge.STUN = nil
	for _, u := range strings.FieldsFunc(get("rigBrowserStun", ""), func(r rune) bool { return r == ',' || r == ' ' }) {
		if strings.HasPrefix(u, "stun:") || strings.HasPrefix(u, "turn:") {
			b.bridge.STUN = append(b.bridge.STUN, u)
		}
	}
	if err := b.bridge.Start(ctx); err != nil {
		return err
	}
	if b.srv != nil {
		return nil
	}
	port, err := strconv.Atoi(get("rigBrowserPort", settingDefaults["rigBrowserPort"]))
	if err != nil || port < 1024 || port > 65535 {
		port = 47900
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(port)))
	if err != nil {
		b.bridge.Stop()
		return fmt.Errorf("port %d for browser play can't be opened: %w", port, err)
	}
	srv := &http.Server{Handler: b.proxy, ReadHeaderTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}

	secure := false
	if s.HTTPS != nil {
		if cfg := s.HTTPS.TLSConfig(); cfg != nil {
			srv.TLSConfig, secure = cfg, true
		}
	}
	b.proxy.Secure = secure
	go func() {
		if secure {
			_ = srv.ServeTLS(ln, "", "")
		} else {
			_ = srv.Serve(ln)
		}
	}()
	b.srv, b.port, b.secure = srv, port, secure
	b.stopCh = make(chan struct{})
	go b.watchIdle(b.stopCh)
	s.Log.Info("browser play started", "port", port, "encrypted", secure)
	return nil
}

// watchIdle stops everything once nothing has come through for a while and no PC is playing.
// A stream doesn't pass through this port once it's going, so a busy PC counts as in use.
func (b *rigBrowser) watchIdle(stop chan struct{}) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
		}
		if time.Since(b.proxy.LastUse()) < rigBrowserIdle {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		busy := false
		rows, _ := b.s.rigRows(ctx)
		for _, rig := range rows {
			if rig.BridgeHost != "" && b.s.rigStatusNow(ctx, rig).State == "busy" {
				busy = true
			}
		}
		cancel()
		if !busy {
			b.stop()
			return
		}
	}
}

func (b *rigBrowser) stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopCh != nil {
		select {
		case <-b.stopCh:
		default:
			close(b.stopCh)
		}
		b.stopCh = nil
	}
	if b.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = b.srv.Shutdown(ctx)
		cancel()
		_ = b.srv.Close()
		b.srv = nil
	}
	b.bridge.Stop()
}

// rigBrowserTicket makes the link a person's browser opens. Nothing is offered over plain HTTP
// from outside the home, as that link is the key to the stream page.
func (s *Server) rigBrowserTicket(r *http.Request, user int64, rig *rigRow, appID int) (url, note string) {
	b := s.browser()
	if !b.bridge.Installed() {
		return "", ""
	}
	if err := b.start(r.Context()); err != nil {
		s.Log.Warn("browser play", "err", err)
		return "", ""
	}
	b.mu.Lock()
	port, secure := b.port, b.secure
	b.mu.Unlock()
	if !secure && !onLocalNetwork(r) {
		return "", ""
	}
	ticket, err := b.proxy.NewTicket(wittymoonlight.Session{User: user, RigID: rig.ID, AppID: appID, HostID: rig.BridgeHost})
	if err != nil {
		return "", ""
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	clean, _, err := wittysunshine.CleanAddress(strings.Trim(host, "[]"))
	if err != nil {
		return "", ""
	}
	scheme := "http"
	if secure {
		scheme = "https"
	} else {
		note = "On plain http, browsers hide game controllers from the page, so this is keyboard and mouse only. Turning on HTTPS in Remote Access, or using the Moonlight app, gets controllers working."
	}
	return scheme + "://" + net.JoinHostPort(clean, strconv.Itoa(port)) + "/wms-ticket/" + ticket, note
}

// GET /api/rigs/browser: whether browser play is here, for the settings page.
func (s *Server) handleRigBrowser(w http.ResponseWriter, r *http.Request) {
	b := s.browser()
	writeJSON(w, 200, map[string]any{
		"available": wittymoonlight.BridgeAvailable(runtime.GOOS, runtime.GOARCH), "installed": b.bridge.Installed(), "running": b.bridge.Port() != 0,
		"name": wittymoonlight.BridgeProject, "version": wittymoonlight.BridgeVersion, "license": wittymoonlight.BridgeLicense, "source": wittymoonlight.BridgeSource,
	})
}

// POST /api/rigs/browser/install: download the bridge from its own project's release, after the
// admin has seen its name and licence on the settings page.
func (s *Server) handleRigBrowserInstall(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := s.browser().bridge.Install(ctx); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	s.Log.Info("browser play installed", "version", wittymoonlight.BridgeVersion, "by", userFrom(r).Name)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// DELETE /api/rigs/browser: remove it, and everything it was holding.
func (s *Server) handleRigBrowserRemove(w http.ResponseWriter, r *http.Request) {
	b := s.browser()
	b.stop()
	b.proxy.RevokeUser(0)
	if err := b.bridge.Remove(); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE wittysunshine_rigs SET bridge_host = ''`); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "note": "To finish, remove " + wittymoonlight.BridgeDevice + " from the paired devices in Sunshine on each PC."})
}

// POST /api/rigs/{id}/browser: pair browser play with this PC. It has its own certificate, so
// this takes its own PIN.
func (s *Server) handleRigBrowserPair(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	b := s.browser()
	if !b.bridge.Installed() {
		writeErr(w, 409, "Install browser play first.")
		return
	}
	g := s.rigs()
	g.mu.Lock()
	if p := g.pairs[rig.ID]; p != nil && p.State == "waiting" {
		g.mu.Unlock()
		writeJSON(w, 200, p)
		return
	}
	g.mu.Unlock()
	ctx := r.Context()
	if err := b.start(ctx); err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	host := rig.BridgeHost
	if host == "" {
		id, err := b.bridge.AddRig(ctx, rig.Address, rig.Port)
		if err != nil {
			writeErr(w, 502, "Browser play couldn't reach this PC: "+err.Error())
			return
		}
		host = strings.Trim(string(id), `"`)
	}
	pctx, cancel := context.WithTimeout(context.Background(), wittysunshine.PairTimeout)
	pin, done, err := b.bridge.PairRig(pctx, json.RawMessage(host))
	if err != nil {
		cancel()
		writeErr(w, 502, err.Error())
		return
	}
	p := &rigPairing{PIN: pin, State: "waiting", Started: time.Now(), Kind: "browser", cancel: cancel}
	g.mu.Lock()
	g.pairs[rig.ID] = p
	g.mu.Unlock()
	go func() {
		defer cancel()
		err := <-done
		g.mu.Lock()
		defer g.mu.Unlock()
		p.PIN = ""
		if err != nil {
			p.State, p.Message = "failed", err.Error()
			return
		}
		if _, err := s.DB.ExecContext(context.Background(), `UPDATE wittysunshine_rigs SET bridge_host = ? WHERE id = ?`, host, rig.ID); err != nil {
			p.State, p.Message = "failed", err.Error()
			return
		}
		p.State, p.Message = "paired", "Browser play is ready on this PC."
	}()
	writeJSON(w, 200, p)
}

// DELETE /api/rigs/{id}/browser: stop offering browser play for this PC.
func (s *Server) handleRigBrowserForget(w http.ResponseWriter, r *http.Request) {
	rig := s.rigForRequest(w, r)
	if rig == nil {
		return
	}
	s.rigBrowserForget(rig)
	if _, err := s.DB.ExecContext(r.Context(), `UPDATE wittysunshine_rigs SET bridge_host = '' WHERE id = ?`, rig.ID); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "note": "To finish, remove " + wittymoonlight.BridgeDevice + " from the paired devices in Sunshine on that PC."})
}

// rigBrowserForget drops a rig from the bridge. It's only done if the bridge is running: if not,
// nothing can reach that entry, and it goes when the bridge is removed.
func (s *Server) rigBrowserForget(rig *rigRow) {
	b := s.browser()
	b.proxy.RevokeRig(rig.ID)
	if b.bridge.Port() == 0 || rig.BridgeHost == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := b.bridge.ForgetRig(ctx, json.RawMessage(rig.BridgeHost)); err != nil {
		s.Log.Debug("browser play: forgetting a PC", "err", err)
	}
}
