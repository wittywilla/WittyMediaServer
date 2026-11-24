// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Command labphone is the pairing lab's phone: the Android app's connector
// library (pkg/wittyconnect/mobile, exactly what the app embeds) driven in the
// order the app drives it when someone pairs from away, with every step
// written down with its time. It never retries a step: the lab asks whether
// pairing works on the first try.
//
// The order is the app's (WittyMediaClient, android/.../p2p/PeerManager.kt
// join, joinByCode and open; ui/App.kt joinInvite and joinedFromAway):
//
//	link: ParseInvite -> Start(key, serverId, hints, port) -> wait for a path
//	      (45 s) -> Enrol(secret) -> POST /api/v1/identity -> POST /api/v1/pair/redeem
//	code: NewJoinCode -> StartJoin -> (someone types the code into the server)
//	      -> Await -> Start(serverId, addrs) -> wait (45 s) -> identity -> redeem
//
// then, as the app does on its next launch: the connection is closed and
// opened again from what was kept (25 s), and the server is asked again.
//
// The phone's state before pairing (-state) is what the app would carry:
//
//	fresh     a new install: a new key, nothing kept.
//	upgrade   an older app's leftovers: the key it made, the local port it used, and addresses
//	          kept for this server from before (a server that has since restarted on new ports).
//	previous  paired before with another server: the same key and port, and a connection to
//	          that server open (it no longer answers) that the new one replaces, as open() does.
//
// It writes one JSON object per line to -steps: {"t":ms,"side":"phone","step":"…","ok":…,"detail":…},
// and the code (link mode: on stdout as {"event":"code",…}) for the lab to type into the server.
//
//	labphone -mode link -link 'wittymedia://p2p/…' -state fresh -steps steps.jsonl -statedir dir
//	labphone -mode code -state fresh -steps steps.jsonl -statedir dir -codefile code.txt
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/pkg/wittyconnect/mobile"
)

// The app's own waits (WittyMediaClient net/PeerRules.kt).
const (
	joinWait   = 45 * time.Second
	awayWait   = 25 * time.Second
	poll       = 250 * time.Millisecond
	usableWait = 20 * time.Second
)

var (
	start            = time.Now()
	stepMu           sync.Mutex
	stepW            io.Writer = os.Stdout
	order                      = "0.6.3"
	codeWait                   = 180 * time.Second
	webHome, webCode string
)

func step(name string, ok bool, detail any) {
	stepMu.Lock()
	defer stepMu.Unlock()
	b, _ := json.Marshal(map[string]any{"t": time.Since(start).Milliseconds(), "at": time.Now().UTC().Format(time.RFC3339Nano),
		"side": "phone", "step": name, "ok": ok, "detail": detail})
	_, _ = stepW.Write(append(b, '\n'))
}

type kept struct {
	Key      string              `json:"key"`
	Port     int                 `json:"port"`
	Servers  map[string][]string `json:"servers"`  // server ID -> addresses kept for it
	Previous string              `json:"previous"` // the server this phone was paired with before
}

type listener struct{ last string }

func (l *listener) OnState(s string) {
	if s != l.last {
		l.last = s
		step("state", true, json.RawMessage(s))
	}
}

func main() {
	mode := flag.String("mode", "link", "link or code")
	link := flag.String("link", "", "the invite link (link mode)")
	state := flag.String("state", "fresh", "fresh, upgrade or previous")
	stepsPath := flag.String("steps", "", "where the steps go (default stdout)")
	stateDir := flag.String("statedir", "", "where the phone keeps its key, port and servers")
	codeFile := flag.String("codefile", "", "code mode: the code is written here for the lab to type in")
	name := flag.String("name", "Lab phone", "this phone's name on the server's list")
	newKey := flag.Bool("newkey", false, "print a new device key and exit")
	orderFlag := flag.String("order", "0.6.3", "which app's order: 0.6.2 (a path first, then the invite) or 0.6.3 (the invite at once, as the desktop helper does; also enrol-first)")
	cw := flag.Duration("codewait", 180*time.Second, "code mode: how long the phone waits for the server to take its code")
	t0 := flag.Int64("t0", 0, "the case's start, in Unix milliseconds, so both sides share a clock")
	wh := flag.String("home", "", "webcode mode: the web address the phone finds the server at, at home")
	wc := flag.String("code", "", "webcode mode: the Connect a device code typed in")
	retryFile := flag.String("retry", "", "if the first try fails, ask for a new link (this file + .ask) and try once more within 30 s")
	relayListen := flag.String("public-relay", "", "run an ordinary public relay on these multiaddrs (comma-separated) instead")
	flag.Parse()
	if *relayListen != "" {
		if err := publicRelay(*relayListen); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if *t0 > 0 {
		start = time.UnixMilli(*t0)
	}
	if *newKey {
		k, err := mobile.NewKey()
		if err != nil {
			os.Exit(1)
		}
		fmt.Println(k)
		return
	}
	if *stepsPath != "" {
		f, err := os.OpenFile(*stepsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		defer f.Close()
		stepW = f
	}
	order = *orderFlag
	webHome, webCode = *wh, *wc
	codeWait = *cw
	err := run(*mode, *link, *state, *stateDir, *codeFile, *name)
	if err != nil && *retryFile != "" {
		// The person tries again at once: a new QR code from the server, scanned within 30 s, in the
		// same app (this process: whatever the library remembers from the first try is still there).
		failedAt := time.Now()
		step("first-try", false, err.Error())
		_ = os.WriteFile(*retryFile+".ask", []byte("1"), 0o644)
		var again string
		for time.Since(failedAt) < 30*time.Second {
			if b, e := os.ReadFile(*retryFile); e == nil && len(b) > 0 {
				again = strings.TrimSpace(string(b))
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if again == "" {
			err = fmt.Errorf("lab error: no second QR code within 30 s (first try: %v)", err)
		} else {
			step("retry", true, map[string]any{"afterFailureMs": time.Since(failedAt).Milliseconds()})
			t := time.Now()
			err = run("link", again, "retry", *stateDir, "", *name)
			if err != nil {
				step("retry-failed", false, map[string]any{"ms": time.Since(t).Milliseconds(), "err": err.Error()})
				err = fmt.Errorf("the retry failed too, %d ms in: %w", time.Since(t).Milliseconds(), err)
			}
		}
	}
	if err != nil {
		step("result", false, err.Error())
		fmt.Fprintln(os.Stderr, "FAILED:", err)
		os.Exit(1)
	}
	step("result", true, "paired on the first try")
}

func load(dir string) kept {
	var k kept
	b, err := os.ReadFile(filepath.Join(dir, "phone.json"))
	if err == nil {
		_ = json.Unmarshal(b, &k)
	}
	if k.Servers == nil {
		k.Servers = map[string][]string{}
	}
	return k
}

func save(dir string, k kept) {
	if dir == "" {
		return
	}
	b, _ := json.MarshalIndent(k, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, "phone.json"), b, 0o600)
}

func run(mode, link, state, dir, codeFile, name string) error {
	k := load(dir)
	step("phone-state", true, map[string]any{"state": state, "hasKey": k.Key != "", "port": k.Port, "servers": len(k.Servers), "previous": k.Previous})
	if state == "fresh" || k.Key == "" {
		if state != "fresh" {
			return errors.New("lab error: no kept state for " + state)
		}
		key, err := mobile.NewKey()
		if err != nil {
			return err
		}
		k = kept{Key: key, Servers: map[string][]string{}}
		// Kept from its first use (the app's Keystore), so a retry is the same device.
		save(dir, k)
	}
	id, _ := mobile.DeviceID(k.Key)
	step("device-key", true, map[string]any{"deviceId": id})

	// A phone paired with another server before still has that connection open when it
	// pairs again: open() shuts it and starts the new one on the same port.
	var old *mobile.Session
	if state == "previous" && k.Previous != "" {
		cfg := config(k.Key, k.Previous, k.Servers[k.Previous], k.Port)
		t := time.Now()
		s, err := mobile.Start(cfg, &listener{})
		step("previous-server-open", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "err": errText(err)})
		if err == nil {
			old = s
			time.Sleep(2 * time.Second)
			k.Port = s.Port()
		}
	}

	// 0.6.3 at home: the QR code's address and code (h=, c=) are tried first, through the web port, as
	// is a code typed in (mode webcode); no peer-to-peer path is needed. Away, they don't answer and the
	// link goes on as an invite.
	if enrolFirst() && (mode == "link" || mode == "webcode") {
		home, code := webHome, webCode
		if mode == "link" {
			if u, err := url.Parse(link); err == nil {
				home, code = u.Query().Get("h"), u.Query().Get("c")
			}
		}
		if home != "" && code != "" {
			tok, err := webJoin(home, code, name)
			if err == nil {
				return useAtHome(home, tok)
			}
			if mode == "webcode" {
				return err
			}
			step("qr-at-home-unanswered", true, map[string]any{"err": errText(err)})
		} else if mode == "webcode" {
			return errors.New("lab error: webcode needs -home and -code")
		}
	}

	var serverID, secret, sname string
	var hints []string
	if mode == "link" {
		t := time.Now()
		raw, err := mobile.ParseInvite(link)
		var inv struct {
			ServerID string   `json:"serverId"`
			Secret   string   `json:"secret"`
			Name     string   `json:"name"`
			Addrs    []string `json:"addrs"`
		}
		if err == nil {
			err = json.Unmarshal([]byte(raw), &inv)
		}
		step("parse-invite", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "serverId": inv.ServerID, "addrs": inv.Addrs, "err": errText(err)})
		if err != nil {
			return fmt.Errorf("the link isn't an invite: %w", err)
		}
		serverID, secret, sname, hints = inv.ServerID, inv.Secret, inv.Name, appHints(inv.Addrs)
		if state == "upgrade" {
			// Kept from before: addresses this server had then, ahead of the link's own.
			hints = appHints(append(append([]string{}, k.Servers[serverID]...), hints...))
			step("kept-hints", true, map[string]any{"hints": hints})
		}
	} else {
		code, err := mobile.NewJoinCode()
		if err != nil {
			return err
		}
		cfg, _ := json.Marshal(map[string]string{"key": k.Key, "code": code, "name": name, "platform": "android"})
		t := time.Now()
		j, err := mobile.StartJoin(string(cfg))
		step("join-code-shown", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "code": code, "err": errText(err)})
		if err != nil {
			return fmt.Errorf("the code search didn't start: %w", err)
		}
		if codeFile != "" {
			_ = os.WriteFile(codeFile+".tmp", []byte(code), 0o644)
			_ = os.Rename(codeFile+".tmp", codeFile)
		}
		t = time.Now()
		// A person waits a few minutes for a code, not the fifteen it lasts: then it is given up.
		timer := time.AfterFunc(codeWait, j.Cancel)
		ans, err := j.Wait() // Await on Android: the same call
		timer.Stop()
		step("join-answer", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "answer": redact(ans), "err": errText(err)})
		if err != nil {
			return fmt.Errorf("nobody took the code: %w", err)
		}
		var a struct {
			OK       bool     `json:"ok"`
			ServerID string   `json:"serverId"`
			Code     string   `json:"code"`
			Name     string   `json:"serverName"`
			Addrs    []string `json:"addrs"`
			Reason   string   `json:"reason"`
		}
		_ = json.Unmarshal([]byte(ans), &a)
		if !a.OK || a.ServerID == "" {
			return fmt.Errorf("the server refused the code: %s", a.Reason)
		}
		serverID, sname, hints = a.ServerID, a.Name, appHints(a.Addrs)
		secret = "code:" + a.Code
	}

	// open(): an open connection to another server goes first.
	if old != nil {
		old.Close()
		step("previous-server-shut", true, nil)
	}
	port := k.Port
	t := time.Now()
	sess, err := mobile.Start(config(k.Key, serverID, hints, port), &listener{})
	step("connector-start", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "askedPort": port, "port": portOf(sess), "err": errText(err)})
	if err != nil {
		return fmt.Errorf("the connection from away couldn't start: %w", err)
	}
	defer func() {
		if sess != nil {
			sess.Close()
		}
	}()
	// The Android app 0.6.x waits for a path before it shows the invite; the desktop app's
	// helper (and -order enrol-first) shows it at once, which is what gets an unlisted device
	// past a relay that admits only listed ones.
	if !enrolFirst() || mode != "link" {
		if st, ok := waitConnected(sess, joinWait); !ok {
			return fmt.Errorf("the server wasn't reached from away in %v (last state %s)", joinWait, st)
		}
	}

	var signIn string
	if mode == "link" {
		t = time.Now()
		ans, err := sess.Enrol(secret, name, "android")
		var a struct {
			OK     bool   `json:"ok"`
			Code   string `json:"code"`
			Reason string `json:"reason"`
			Error  string `json:"error"`
		}
		_ = json.Unmarshal([]byte(ans), &a)
		step("enrol", err == nil && a.OK, map[string]any{"ms": time.Since(t).Milliseconds(), "answer": redact(ans), "err": errText(err)})
		if err != nil || !a.OK {
			return fmt.Errorf("the server didn't take the invite: %v %s %s", err, a.Reason, a.Error)
		}
		signIn = a.Code
		if enrolFirst() {
			if st, ok := waitConnected(sess, joinWait); !ok {
				return fmt.Errorf("listed, but the server wasn't reached from away in %v (last state %s)", joinWait, st)
			}
			// 0.6.3 (PeerManager.join): connected through a public relay, which carries no requests, it waits up to
			// 20 s (USABLE_WAIT_MS) for a usable path before the first request.
			waitUsable(sess, usableWait)
		}
	} else {
		signIn = strings.TrimPrefix(secret, "code:")
	}

	base := fmt.Sprintf("http://127.0.0.1:%d", sess.Port())
	if err := identity(base, sess.Secret(), "identity"); err != nil {
		return err
	}
	tok, err := redeem(base, sess.Secret(), signIn, name)
	if err != nil {
		return err
	}
	if err := get(base, sess.Secret(), tok, "/api/v1/libraries", "use-after-pairing"); err != nil {
		return err
	}
	// joined(): what the app keeps for next time.
	var addrs []string
	_ = json.Unmarshal([]byte(sess.KnownAddrs()), &addrs)
	k.Port = sess.Port()
	k.Servers[serverID] = addrs
	k.Previous = serverID
	save(dir, k)
	step("kept", true, map[string]any{"server": sname, "addrs": len(addrs), "port": k.Port})

	// The next launch: closed, and opened again from what was kept.
	sess.Close()
	sess = nil
	t = time.Now()
	again, err := mobile.Start(config(k.Key, serverID, appHints(addrs), k.Port), &listener{})
	step("reopen-start", err == nil, map[string]any{"ms": time.Since(t).Milliseconds(), "err": errText(err)})
	if err != nil {
		return fmt.Errorf("reopening: %w", err)
	}
	sess = again
	if st, ok := waitConnected(sess, awayWait); !ok {
		return fmt.Errorf("after pairing, the server wasn't reached again in %v (last state %s)", awayWait, st)
	}
	base = fmt.Sprintf("http://127.0.0.1:%d", sess.Port())
	if err := identity(base, sess.Secret(), "reopen-identity"); err != nil {
		return err
	}
	return get(base, sess.Secret(), tok, "/api/v1/libraries", "reopen-use")
}

func config(key, serverID string, hints []string, port int) string {
	if hints == nil {
		hints = []string{}
	}
	b, _ := json.Marshal(map[string]any{"key": key, "serverId": serverID, "hints": hints, "port": appPort(port)})
	return string(b)
}

// The app's own filters (PeerRules.hints, PeerRules.port).
func appHints(list []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, a := range list {
		a = strings.TrimSpace(a)
		if len(a) > 400 || seen[a] || strings.HasPrefix(a, "/ip4/127.") || strings.HasPrefix(a, "/ip6/::1/") {
			continue
		}
		ok := false
		for _, p := range []string{"/ip4/", "/ip6/", "/dns/", "/dns4/", "/dns6/"} {
			ok = ok || strings.HasPrefix(a, p)
		}
		if !ok {
			continue
		}
		seen[a] = true
		out = append(out, a)
		if len(out) == 12 {
			break
		}
	}
	return out
}

func appPort(p int) int {
	if p >= 1024 && p <= 65535 {
		return p
	}
	return 0
}

func portOf(s *mobile.Session) int {
	if s == nil {
		return 0
	}
	return s.Port()
}

func waitConnected(s *mobile.Session, limit time.Duration) (string, bool) {
	t := time.Now()
	var last string
	for time.Since(t) < limit {
		last = s.State()
		var st struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(last), &st)
		if st.Path == "direct" || strings.HasPrefix(st.Path, "relay") || st.Path == "local" {
			step("connected", true, map[string]any{"ms": time.Since(t).Milliseconds(), "state": json.RawMessage(last)})
			return last, true
		}
		time.Sleep(poll)
	}
	step("connected", false, map[string]any{"ms": time.Since(t).Milliseconds(), "state": json.RawMessage(orNull(last))})
	return last, false
}

var client = &http.Client{Timeout: 30 * time.Second}

// quickClient is the apps' quick API client (Api.kt QUICK_SECONDS, callTimeout 5 s), which both 0.6.2 and 0.6.3
// use for the identity check through the connector.
var quickClient = &http.Client{Timeout: 5 * time.Second}

func call(method, url, secret, token string, body any) (int, []byte, http.Header, error) {
	return callWith(client, method, url, secret, token, body)
}

func callWith(client *http.Client, method, url, secret, token string, body any) (int, []byte, http.Header, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("X-Witty-Connect", secret)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res.StatusCode, b, res.Header, nil
}

// identify(): a nonce, and the server proving its key through the connection.
func identity(base, secret, name string) error {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	t := time.Now()
	code, b, h, err := callWith(quickClient, "POST", base+"/api/v1/identity", secret, "", map[string]string{"nonce": base64.StdEncoding.EncodeToString(raw)})
	detail := map[string]any{"ms": time.Since(t).Milliseconds(), "status": code, "err": errText(err)}
	if h != nil && h.Get("X-Witty-Connect-Error") != "" {
		detail["connectorError"] = h.Get("X-Witty-Connect-Error")
	}
	if code != 200 {
		detail["body"] = clip(b)
	}
	ok := err == nil && code == 200
	step(name, ok, detail)
	if !ok {
		// What the app shows: an IOException is "Couldn't reach your server from away".
		if err != nil {
			return fmt.Errorf("%s: unreachable (the app says \"Couldn't reach your server from away\"): %v", name, err)
		}
		return fmt.Errorf("%s: HTTP %d %s", name, code, clip(b))
	}
	return nil
}

func redeem(base, secret, signIn, name string) (string, error) {
	t := time.Now()
	code, b, _, err := call("POST", base+"/api/v1/pair/redeem", secret, "", map[string]string{"code": signIn, "device": name, "platform": "android"})
	var r struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(b, &r)
	ok := err == nil && code == 200 && r.Token != ""
	d := map[string]any{"ms": time.Since(t).Milliseconds(), "status": code, "err": errText(err)}
	if !ok {
		d["body"] = clip(b)
	}
	step("pair-redeem", ok, d)
	if !ok {
		return "", fmt.Errorf("signing in with the code from the server: HTTP %d %v %s", code, err, clip(b))
	}
	return r.Token, nil
}

func get(base, secret, token, path, name string) error {
	t := time.Now()
	code, b, _, err := call("GET", base+path, secret, token, nil)
	ok := err == nil && code == 200
	d := map[string]any{"ms": time.Since(t).Milliseconds(), "status": code, "err": errText(err)}
	if !ok {
		d["body"] = clip(b)
	}
	step(name, ok, d)
	if !ok {
		return fmt.Errorf("%s %s: HTTP %d %v", name, path, code, err)
	}
	return nil
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}

func clip(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		s = s[:300]
	}
	return s
}

// redact keeps a sign-in code out of the steps.
func redact(ans string) json.RawMessage {
	var m map[string]any
	if json.Unmarshal([]byte(ans), &m) != nil {
		return json.RawMessage(`null`)
	}
	if _, ok := m["code"]; ok {
		m["code"] = "…"
	}
	b, _ := json.Marshal(m)
	return b
}

// enrolFirst: the app shows an invite as soon as the connection starts (0.6.3 on, and the desktop
// helper), rather than after a path is found (0.6.2 and before).
func enrolFirst() bool { return order == "0.6.3" || order == "enrol-first" }

// useAtHome: signed in at home through the web port; the server's libraries, there.
func useAtHome(home, tok string) error {
	t := time.Now()
	req, _ := http.NewRequest("GET", strings.TrimRight(home, "/")+"/api/v1/libraries", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	res, err := client.Do(req)
	code := 0
	if err == nil {
		code = res.StatusCode
		res.Body.Close()
	}
	step("use-after-pairing", err == nil && code == 200, map[string]any{"ms": time.Since(t).Milliseconds(), "at": home, "status": code, "err": errText(err)})
	if err != nil || code != 200 {
		return fmt.Errorf("using the server at home: HTTP %d %v", code, err)
	}
	return nil
}

// waitUsable waits for a path that carries requests: local, direct, or a community relay (PeerRules.kt usable).
func waitUsable(s *mobile.Session, limit time.Duration) bool {
	t := time.Now()
	var last string
	for time.Since(t) < limit {
		last = s.State()
		var st struct {
			Path      string `json:"path"`
			Community bool   `json:"community"`
		}
		_ = json.Unmarshal([]byte(last), &st)
		if st.Path == "local" || st.Path == "direct" || (st.Path == "relayed" && st.Community) {
			step("usable", true, map[string]any{"ms": time.Since(t).Milliseconds(), "state": json.RawMessage(last)})
			return true
		}
		time.Sleep(poll)
	}
	step("usable", false, map[string]any{"ms": time.Since(t).Milliseconds(), "state": json.RawMessage(orNull(last))})
	return false
}
