// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"encoding/json"
	"fmt"
	"github.com/wittywilla/wittymediaserver/internal/modules/modtest"
	"net/http"
	"testing"
)

func TestRadioRelayHasALimit(t *testing.T) {
	modtest.Need(t, "wittyplay")
	var l radioLive
	for i := 0; i < radioStreamsPerUser; i++ {
		if !l.takeStream(1) {
			t.Fatal("refused too soon")
		}
	}
	if l.takeStream(1) {
		t.Error("one person went over")
	}
	if !l.takeStream(2) {
		t.Error("that stopped someone else")
	}
	l.dropStream(1)
	if !l.takeStream(1) {
		t.Error("a place didn't come back")
	}
	for i := 2; i < radioStreamsAtOnce; i++ {
		l.takeStream(int64(i * 10))
		l.takeStream(int64(i * 10))
	}
	if l.takeStream(9999) {
		t.Error("the server went over")
	}
}

func TestRadioRelayAnswersWhenFull(t *testing.T) {
	modtest.Need(t, "wittyplay")
	ts, lib, st := radioFixture(t, func(http.ResponseWriter, *http.Request) {})
	it := addStation(t, ts, lib, "Live", st.URL+"/a")
	u := userFromToken(t, ts, ts.adult)
	for range radioStreamsPerUser {
		ts.s.radio.takeStream(u)
	}
	w := ts.do("GET", fmt.Sprintf("/api/radio/%d/stream", it.ID), ts.adult, nil)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Errorf("%d %s", w.Code, w.Body)
	}
}

func userFromToken(t *testing.T, ts *galleryServer, token string) int64 {
	t.Helper()
	var v struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(ts.do("GET", "/api/me", token, nil).Body.Bytes(), &v); err != nil || v.ID == 0 {
		t.Fatalf("%v", err)
	}
	return v.ID
}

func TestModuleReportsAreLimited(t *testing.T) {
	modtest.Need(t, "wittyplay")
	ts := newTestServer(t)
	ts.Modules = installedBundle(t, `export default 1`, `a{}`)
	_, tok := ts.user(t, "reader", "adult", false, "")
	limited := false
	for range 500 {
		w := ts.do(t, "POST", "/api/modules/wittycomics/failed", tok, map[string]string{"version": "0.2.0", "reason": "x"})
		if w.Code == 429 {
			limited = w.Header().Get("Retry-After") != ""
			break
		}
	}
	if !limited {
		t.Error("reports are never limited")
	}
}
