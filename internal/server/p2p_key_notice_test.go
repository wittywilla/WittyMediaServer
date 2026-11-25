// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/p2p"
)

// A backup made without a password never holds the server's connection key:
// it is the server's private key. Restored, it leaves the server with devices on
// its list and no key, so a new key and a new connection ID (audit F8, #746).
// The administrators are told, Remote Access says so, and every device that
// hasn't connected since shows as needing a new invite, until it connects
// again or is removed. The backup screen says so beforehand.
func TestARestoredBackupWithoutAPasswordAsksForNewInvites(t *testing.T) {
	b := newBackupServer(t)
	ctx := context.Background()
	b.p2p.tune = func(c *p2p.NodeConfig) {
		c.Local, c.Listen = true, p2pLoopback
		c.Layers = p2p.Layers{}
	}
	b.StartP2P(ctx, b.h)
	t.Cleanup(b.StopP2P)
	if w := b.call("PUT", "/api/p2p/settings", b.admin, map[string]any{"enabled": true, "accept": true}); w.Code != 200 {
		t.Fatalf("turning peer to peer on: %d %s", w.Code, w.Body)
	}
	oldID := b.p2pNode().ID()
	kim := userIDOf(t, &testServer{Server: b.Server, h: b.h}, b.admin)
	phone, laptop := testP2PID(t, 4), testP2PID(t, 5)
	for _, id := range []string{phone, laptop} {
		if _, err := b.p2pAddDevice(ctx, kim, id, "A device", "android", "invite"); err != nil {
			t.Fatal(err)
		}
	}
	hourAgo := time.Now().Add(-time.Hour).Unix()
	if _, err := b.DB.ExecContext(ctx, `UPDATE p2p_devices SET last_seen = ?, created_at = ?`, hourAgo, hourAgo-60); err != nil {
		t.Fatal(err)
	}
	_ = b.p2p.devices.load(ctx, b.Server)

	// Beforehand, the backup screen says what a backup without a password can't keep.
	var view struct {
		P2PKeyNote string `json:"p2pKeyNote"`
	}
	_ = json.Unmarshal(b.call("GET", "/api/backup", b.admin, nil).Body.Bytes(), &view)
	if !strings.Contains(view.P2PKeyNote, "connection ID") {
		t.Fatalf("the backup screen's note: %q", view.P2PKeyNote)
	}

	// A backup of the settings without a password: the key isn't in it. Nor is the device list, which no
	// backup holds (internal/backup, neverTables): a restore on this server keeps it as it is here, unless
	// the accounts are restored too, which empties it.
	key := b.DB.GetSetting(ctx, p2pSeedKey, "")
	if key == "" {
		t.Fatal("no key to begin with")
	}
	if w := b.call("POST", "/api/backup/create", b.admin, map[string]any{"parts": []string{"settings"}}); w.Code != 202 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	j := b.waitJob(t)
	if j.Error != "" || j.File == "" {
		t.Fatalf("%+v", j)
	}
	raw, err := os.ReadFile(filepath.Join(b.backupFolders().ArchiveDir(), j.File))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var dbBytes []byte
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".db") {
			rc, _ := f.Open()
			dbBytes, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if len(dbBytes) == 0 {
		t.Fatal("no database in the backup")
	}
	if bytes.Contains(dbBytes, []byte(key)) {
		t.Fatal("a backup without a password holds the server's private connection key")
	}
	if bytes.Contains(dbBytes, []byte(phone)) {
		t.Fatal("a backup holds the peer-to-peer device list")
	}

	// Restored on this server: its settings come from the backup, with no key, and its device list stays
	// (internal/backup, buildFinalDB). The server starts its peer-to-peer connections again.
	b.StopP2P()
	if err := b.DB.SetSetting(ctx, p2pSeedKey, ""); err != nil {
		t.Fatal(err)
	}
	b.StartP2P(ctx, b.h)
	if b.p2pNode() == nil || b.p2pNode().ID() == oldID {
		t.Fatal("no new connection ID")
	}

	// The administrators are told.
	w := b.call("GET", "/api/me/notifications", b.admin, nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "new connection ID") {
		t.Fatalf("the administrator's notifications: %d %s", w.Code, w.Body)
	}
	// Remote Access says so, and each device waits for a new invite.
	var p2pv p2pView
	_ = json.Unmarshal(b.call("GET", "/api/p2p/settings", b.admin, nil).Body.Bytes(), &p2pv)
	if p2pv.KeyNotice == nil || p2pv.KeyNotice.Reinvite != 2 || !strings.Contains(p2pv.KeyNotice.Words, "new connection ID") {
		t.Fatalf("the Remote Access notice: %+v", p2pv.KeyNotice)
	}
	for _, d := range p2pv.Devices {
		if !d.Reinvite || !d.Pending || d.Connected || !strings.Contains(d.Why, "Invite this device again") {
			t.Fatalf("a device from before the restore: %+v", d)
		}
	}

	// The phone connects again (it learnt the new ID): it is back to normal by itself. The laptop is removed.
	b.p2p.devices.Seen(phone, "direct")
	if err := b.p2pRemoveDevice(ctx, laptop); err != nil {
		t.Fatal(err)
	}
	p2pv = p2pView{}
	_ = json.Unmarshal(b.call("GET", "/api/p2p/settings", b.admin, nil).Body.Bytes(), &p2pv)
	if p2pv.KeyNotice != nil || len(p2pv.Devices) != 1 || p2pv.Devices[0].Reinvite {
		t.Fatalf("once nobody waits: %+v %+v", p2pv.KeyNotice, p2pv.Devices)
	}
	if b.DB.GetSetting(ctx, p2pKeyNewSetting, "") != "" {
		t.Fatal("what was kept about it outlived the last device waiting")
	}
}

// A first key, with nobody on the list, is no news.
func TestAFirstKeyIsNoNews(t *testing.T) {
	ts, admin := p2pServer(t, nil)
	if ts.DB.GetSetting(context.Background(), p2pKeyNewSetting, "") != "" {
		t.Fatal("a first key was taken for a changed one")
	}
	var v p2pView
	_ = json.Unmarshal(ts.do(t, "GET", "/api/p2p/settings", admin, nil).Body.Bytes(), &v)
	if v.KeyNotice != nil {
		t.Fatalf("%+v", v.KeyNotice)
	}
}
