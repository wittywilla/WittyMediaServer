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
