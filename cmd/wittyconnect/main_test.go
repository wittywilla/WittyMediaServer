// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
	"github.com/wittywilla/wittymediaserver/pkg/wittyconnect"
)

type everyone struct{}

func (everyone) Allowed(string) bool { return true }
func (everyone) Seen(string, string) {}

// The helper as the desktop app drives it: a configuration on its input,
// events on its output, a request through the address it names, and an exit
// when the input closes. All on this machine.
func TestHelperLifecycle(t *testing.T) {
	loopback := []string{"/ip4/127.0.0.1/udp/0/quic-v1", "/ip4/127.0.0.1/tcp/0"}
	node, err := p2p.StartNode(p2p.NodeConfig{Seed: bytes.Repeat([]byte{1}, p2p.SeedSize), Devices: everyone{}, DevicesOnly: true, Local: true, Listen: loopback,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "from the server") })})
	if err != nil {
		t.Fatal(err)
	}
	defer node.Close()

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- run(inR, outW, func(c *wittyconnect.Config) {
			c.Tune = func(cc *p2p.ClientConfig) { cc.Local, cc.Listen = true, loopback }
		})
		outW.Close()
	}()
	key, _ := wittyconnect.NewKey()
	cfg, _ := json.Marshal(map[string]any{"key": key, "serverId": node.ID(), "hints": node.Addrs(), "noDht": true, "noRelay": true})
	go fmt.Fprintf(inW, "%s\n", cfg)

	events := bufio.NewScanner(outR)
	next := func(want string) map[string]any {
		t.Helper()
		for events.Scan() {
			var e map[string]any
			if err := json.Unmarshal(events.Bytes(), &e); err != nil {
				t.Fatalf("not JSON: %s", events.Text())
			}
			if e["event"] == want {
				return e
			}
		}
		t.Fatalf("the helper stopped before saying %q", want)
		return nil
	}
	ready := next("ready")
	port, secret := int(ready["port"].(float64)), ready["secret"].(string)
	if port == 0 || len(secret) < 30 || ready["deviceId"] == "" {
		t.Fatalf("ready: %v", ready)
	}
	for {
		st := next("state")["state"].(map[string]any)
		if st["path"] == "local" {
			break
		}
	}
	if addrs := next("addrs")["addrs"].([]any); len(addrs) == 0 {
		t.Fatal("no addresses to keep for next time")
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/", port), nil)
	req.Header.Set(wittyconnect.SecretHeader, secret)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(body) != "from the server" {
		t.Fatalf("through the helper: %q", body)
	}
	go func() { // keep reading, so the helper never blocks on its output
		for events.Scan() {
		}
	}()
	// The app goes away: its end of the pipe closes, and the helper exits.
	inW.Close()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit code %d", code)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the helper outlived the app")
	}
	if _, err := http.DefaultClient.Do(req); err == nil {
		t.Fatal("the local address still answers after the helper stopped")
	}
}

func TestHelperRefusesBadInput(t *testing.T) {
	var out bytes.Buffer
	if code := run(bytes.NewReader([]byte("not json\n")), &out, nil); code == 0 || !bytes.Contains(out.Bytes(), []byte(`"event":"error"`)) {
		t.Fatalf("code %d, said %s", code, out.String())
	}
	out.Reset()
	if code := run(bytes.NewReader([]byte(`{"key":"x","serverId":"y"}`+"\n")), &out, nil); code == 0 || !bytes.Contains(out.Bytes(), []byte(`"event":"error"`)) {
		t.Fatalf("code %d, said %s", code, out.String())
	}
	if code := run(bytes.NewReader(nil), &out, nil); code == 0 {
		t.Fatal("no configuration at all is not a success")
	}
}

// Joining by a code, as the desktop app drives it: the code comes back at once in the form
// people type, a configuration with no key is refused, and closing the input gives up quietly.
// Finding the server once the code is typed in is tested in internal/p2p.
func TestHelperJoin(t *testing.T) {
	var refused bytes.Buffer
	if code := runJoin(strings.NewReader("{}\n"), &refused, nil); code != 2 || !strings.Contains(refused.String(), `"error"`) {
		t.Fatalf("no key: exit %d, %s", code, refused.String())
	}

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() {
		done <- runJoin(inR, outW, func(c *wittyconnect.JoinConfig) {
			c.Tune = func(jc *p2p.JoinConfig) {
				jc.Local, jc.Listen, jc.Bootstrap = true, []string{"/ip4/127.0.0.1/tcp/0"}, nil
			}
		})
		outW.Close()
	}()
	key, _ := wittyconnect.NewKey()
	cfg, _ := json.Marshal(map[string]any{"key": key, "name": "Test PC", "platform": "desktop"})
	go fmt.Fprintf(inW, "%s\n", cfg)
	events := bufio.NewScanner(outR)
	if !events.Scan() {
		t.Fatal("no event")
	}
	var e struct{ Event, Code string }
	if err := json.Unmarshal(events.Bytes(), &e); err != nil || e.Event != "code" || !regexp.MustCompile(`^[0-9A-Z]{4}-[0-9A-Z]{4}$`).MatchString(e.Code) {
		t.Fatalf("first event: %s", events.Text())
	}
	inW.Close() // the app gave up
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("giving up: exit %d", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("still waiting after its input closed")
	}
	if events.Scan() {
		t.Fatalf("said something after giving up: %s", events.Text())
	}
}
