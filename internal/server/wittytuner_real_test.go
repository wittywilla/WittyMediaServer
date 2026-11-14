// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/livetv"
	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
)

// The real WittyTuner (module API 5), from the test catalogue: a tuner added through its own
// routes is in the server's guide, its channel opens through WittyTuner and plays through the
// server's own guard, its SDUI pages answer for an administrator, and its guide sources hand a
// guide to the server's host API.
func TestWittyTunerEndToEnd(t *testing.T) {
	m := modtest.Need(t, "wittytuner")
	ts := newTestServer(t)
	ts.Modules = m
	livetv.SetDirectory(livetv.RemoteDirectory{R: modmgr.Tuners{M: m}, CheckInput: modmgr.CheckInput})
	t.Cleanup(func() { livetv.SetDirectory(nil) })
	if err := ts.StartHostAPI(); err != nil {
		t.Fatal(err)
	}
	ts.h = ts.Handler()
	_, admin := ts.user(t, "owner", "adult", true, "")
	_, adult := ts.user(t, "viewer", "adult", false, "")

	packet := bytes.Repeat([]byte{0xff}, 188)
	packet[0], packet[1], packet[2], packet[3] = 0x47, 0x01, 0x00, 0x10
	streams := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".xml") {
			start := time.Now().Add(-10 * time.Minute).UTC().Format("20060102150405 -0700")
			stop := time.Now().Add(50 * time.Minute).UTC().Format("20060102150405 -0700")
			_, _ = io.WriteString(w, `<tv><channel id="seven.test"><display-name>Seven</display-name></channel><programme start="`+start+`" stop="`+stop+`" channel="seven.test"><title>Evening news</title></programme></tv>`)
			return
		}
		for range 2000 {
			if _, err := w.Write(packet); err != nil {
				return
			}
		}
	}))
	defer streams.Close()

	api := func(method, path, token string, body any) (int, []byte) {
		t.Helper()
		w := ts.do(t, method, "/api/modules/wittytuner"+path, token, body)
		return w.Code, w.Body.Bytes()
	}
	if code, body := api("GET", "/drivers", admin, nil); code != 200 || !bytes.Contains(body, []byte(`"stream"`)) {
		t.Fatalf("drivers: %d %s", code, body)
	}
	if code, _ := api("GET", "/tuners", adult, nil); code != 403 {
		t.Fatalf("tuners for someone who isn't an administrator: %d", code)
	}
	code, body := api("POST", "/tuners", admin, map[string]any{"driver": "stream", "name": "Seven", "options": map[string]string{"url": streams.URL + "/seven.ts", "number": "7"}})
	if code != 200 {
		t.Fatalf("adding a tuner: %d %s", code, body)
	}
	var tn struct {
		ID       int64 `json:"id"`
		Channels int   `json:"channels"`
	}
	_ = json.Unmarshal(body, &tn)
	if tn.Channels != 1 {
		t.Fatalf("the tuner: %s", body)
	}
	code, body = api("GET", "/tuners/"+strconv.FormatInt(tn.ID, 10)+"/channels", admin, nil)
	var chans []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if code != 200 || json.Unmarshal(body, &chans) != nil || len(chans) != 1 {
		t.Fatalf("its channels: %d %s", code, body)
	}
	key := "tun-" + strconv.FormatInt(chans[0].ID, 10)

	// In the server's guide, for anyone who may watch.
	w := ts.do(t, "GET", "/api/guide?list=1", adult, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"key":"`+key+`"`) || !strings.Contains(w.Body.String(), `"number":"7"`) {
		t.Fatalf("the guide: %d %s", w.Code, w.Body)
	}
	// It opens through WittyTuner, and plays through the server's own guard.
	o, err := ts.TV.Open(context.Background(), chans[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if o.Stream.Guard == nil {
		t.Fatal("not through the server's guard")
	}
	n, err := io.Copy(io.Discard, o.Stream.Reader)
	o.Release()
	if err != nil || n == 0 {
		t.Fatalf("read %d: %v", n, err)
	}

	// A guide source: fetched by WittyTuner, kept by the server.
	code, body = api("POST", "/epg/sources", admin, map[string]string{"name": "Listings", "url": streams.URL + "/guide.xml"})
	if code != 200 {
		t.Fatalf("adding a guide source: %d %s", code, body)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		var n int
		_ = ts.DB.QueryRow(`SELECT COUNT(*) FROM epg_channels`).Scan(&n)
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			_, b := api("GET", "/epg/sources", admin, nil)
			t.Fatalf("the server never got the guide: %s", b)
		}
		time.Sleep(100 * time.Millisecond)
	}
	if code, body := api("GET", "/epg/ids", admin, nil); code != 200 || !bytes.Contains(body, []byte("seven.test")) {
		t.Fatalf("guide ids: %d %s", code, body)
	}

	// Its SDUI pages, for an administrator on any app.
	for _, page := range []string{"wittytuner", "wittytuner-setup"} {
		if w := ts.do(t, "GET", "/api/sdui/pages/"+page, admin, nil); w.Code != 200 {
			t.Errorf("page %s: %d %s", page, w.Code, w.Body)
		}
	}
	w = ts.do(t, "GET", "/api/sdui/pages/wittytuner/sections/tuners/items", admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Seven") {
		t.Errorf("the tuners section: %d %s", w.Code, w.Body)
	}
	if code, body := api("GET", "/encrypted-help", adult, nil); code != 200 || !bytes.Contains(body, []byte("ATSC 1.0")) {
		t.Errorf("encrypted help: %d %s", code, body)
	}
	if code, _ := api("DELETE", "/tuners/"+strconv.FormatInt(tn.ID, 10), admin, nil); code != 200 {
		t.Errorf("removing it: %d", code)
	}
	ts.TV.ForgetChannels()
	if w := ts.do(t, "GET", "/api/guide?list=1", adult, nil); strings.Contains(w.Body.String(), `"key":"`+key+`"`) {
		t.Errorf("a removed tuner's channel is in the guide: %s", w.Body)
	}
}
