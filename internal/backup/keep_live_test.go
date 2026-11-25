// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package backup

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// scrubRelayKey stands for a setting kept with a secret inside it taken out (the server's p2p setting).
func scrubRelayKey(v string) string {
	return strings.ReplaceAll(v, `"key":"RELAYKEYPLANTED"`, `"key":""`)
}

var keepOpts = StageOptions{
	SecretSettings: []string{"tmdbKey"},
	ScrubSettings:  map[string]func(string) string{"p2p": scrubRelayKey},
	SecretWords:    map[string]string{"p2pKey": "the connection key"},
}

// plantMore adds the secrets aServer doesn't: the peer-to-peer key, the identity, a setting with a secret
// inside it, and addresses with logins in them.
func plantMore(t *testing.T, d *sql.DB) {
	t.Helper()
	for k, v := range map[string]string{
		"p2pKey":     "P2PKEYPLANTED",
		"serverId":   "SERVERIDPLANTED",
		"p2p":        `{"enabled":true,"ownRelays":[{"addr":"/dns4/relay.example/udp/4001/quic-v1","key":"RELAYKEYPLANTED"}]}`,
		"guideFeed":  "https://guide:FEEDPASSPLANTED@tv.example/feed.xml",
		"brandColor": "#123456",
	} {
		mustExec(t, d, `INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v)
	}
	mustExec(t, d, `INSERT INTO podcasts(id, library_id, url, title, created_at) VALUES (1, 1, 'https://me:PODPASSPLANTED@pods.example/feed', 'A show', 1)`)
}

func backupWithoutPassword(t *testing.T, f Folders) string {
	t.Helper()
	out, man, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(t.TempDir(), "b.zip"),
		SecretSettings: keepOpts.SecretSettings, ScrubSettings: keepOpts.ScrubSettings})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"p2pKey", "serverId", "tmdbKey", "p2p", "guideFeed", "notifySmtpPassword", "serverKey"} {
		if !slices.Contains(man.Stripped, k) {
			t.Errorf("the manifest doesn't say %s was left out: %v", k, man.Stripped)
		}
	}
	if slices.Contains(man.Stripped, "brandColor") {
		t.Error("a plain setting was said to be stripped")
	}
	return out
}

func valueIn(t *testing.T, d *sql.DB, q string) string {
	t.Helper()
	var v sql.NullString
	if err := d.QueryRow(q).Scan(&v); err != nil {
		return "(" + err.Error() + ")"
	}
	return v.String
}

// Restored onto the machine it was made on, a backup without a password brings
// back none of that machine's secrets, and clears none of them: each one the
// backup left out is this machine's own again, and the summary says which.
func TestARestoreKeepsThisMachinesOwnSecrets(t *testing.T) {
	f, _ := aServer(t)
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	plantMore(t, d.DB)
	out := backupWithoutPassword(t, f)
	// What changed since the backup, besides its secrets, is the backup's to put back.
	mustExec(t, d.DB, `UPDATE settings SET value = '#654321' WHERE key = 'brandColor'`)

	in, err := Inspect(ctx, out, "", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	o := keepOpts
	o.Parts, o.LiveDB = everything, d.DB
	req, err := in.Stage(ctx, f, o)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(req.Kept, "the connection key") || len(req.New) != 0 {
		t.Fatalf("what the summary will say: kept %q, new %q", req.Kept, req.New)
	}
	d.Close()
	r := Apply(f)
	if r == nil || !r.OK || !strings.Contains(strings.Join(r.Notes, " "), "This server's own were kept") {
		t.Fatalf("%+v", r)
	}
	d, err = db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for q, want := range map[string]string{
		// Settings left out by name: the server's private connection key, its identity, keys to services.
		`SELECT value FROM settings WHERE key = 'p2pKey'`:             "P2PKEYPLANTED",
		`SELECT value FROM settings WHERE key = 'serverId'`:           "SERVERIDPLANTED",
		`SELECT value FROM settings WHERE key = 'serverKey'`:          "IDENTITYPLANTED",
		`SELECT value FROM settings WHERE key = 'tmdbKey'`:            "TMDBPLANTEDKEY",
		`SELECT value FROM settings WHERE key = 'notifySmtpPassword'`: "SMTPPLANTED",
		// A setting with the secret inside it taken out, otherwise the same: this machine's, secret and all.
		`SELECT value FROM settings WHERE key = 'p2p'`: `{"enabled":true,"ownRelays":[{"addr":"/dns4/relay.example/udp/4001/quic-v1","key":"RELAYKEYPLANTED"}]}`,
		// An address with its login taken out.
		`SELECT value FROM settings WHERE key = 'guideFeed'`: "https://guide:FEEDPASSPLANTED@tv.example/feed.xml",
		`SELECT url FROM podcasts WHERE id = 1`:              "https://me:PODPASSPLANTED@pods.example/feed",
		// Each account's two-factor sign-in and webhook.
		`SELECT totp_secret FROM users WHERE id = 1`:                   "TOTPPLANTEDSECRET",
		`SELECT totp_on FROM users WHERE id = 1`:                       "1",
		`SELECT recovery_codes FROM users WHERE id = 1`:                "RECOVERYPLANTED",
		`SELECT webhook_url FROM notify_channels WHERE user_id = 1`:    "https://hooks.example.com/HOOKPLANTED",
		`SELECT webhook_secret FROM notify_channels WHERE user_id = 1`: "HOOKSECRETPLANTED",
		`SELECT webhook_on FROM notify_channels WHERE user_id = 1`:     "1",
		// And what isn't a secret is the backup's.
		`SELECT value FROM settings WHERE key = 'brandColor'`: "#123456",
	} {
		if got := valueIn(t, d.DB, q); got != want {
			t.Errorf("%s: %q, want %q", q, got, want)
		}
	}
}

// Restored onto another machine, which has none of them: nothing is kept, and
// the summary says what is new or empty. An account that only shares a number
// with one in the backup gets nothing of its secrets.
func TestARestoreOntoAnotherMachineSaysWhatIsNew(t *testing.T) {
	f, _ := aServer(t)
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	plantMore(t, d.DB)
	out := backupWithoutPassword(t, f)
	d.Close()

	other, _ := aServer(t)
	od, err := db.Open(other.Data)
	if err != nil {
		t.Fatal(err)
	}
	// Another machine: no peer-to-peer key or identity of its own, and its account 1 is someone else.
	mustExec(t, od.DB, `DELETE FROM settings WHERE key IN ('tmdbKey', 'notifySmtpPassword', 'serverKey')`)
	mustExec(t, od.DB, `UPDATE users SET name = 'kimberly', totp_secret = 'SOMEONEELSESTOTP' WHERE id = 1`)
	mustExec(t, od.DB, `UPDATE notify_channels SET webhook_url = 'https://hooks.example.com/SOMEONEELSE' WHERE user_id = 1`)
	in, err := Inspect(ctx, out, "", other, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	o := keepOpts
	o.Parts, o.LiveDB = everything, od.DB
	req, err := in.Stage(ctx, other, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Kept) != 0 {
		t.Errorf("kept from a machine that had none of it: %q", req.Kept)
	}
	for _, w := range []string{"the connection key", "serverId", "tmdbKey"} {
		if !slices.Contains(req.New, w) {
			t.Errorf("the summary doesn't say %q is new: %q", w, req.New)
		}
	}
	od.Close()
	if r := Apply(other); r == nil || !r.OK || !strings.Contains(strings.Join(r.Notes, " "), "new or empty now") {
		t.Fatalf("%+v", r)
	}
	od, err = db.Open(other.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer od.Close()
	for q, want := range map[string]string{
		`SELECT COUNT(*) FROM settings WHERE key = 'p2pKey'`:        "0",
		`SELECT COALESCE(totp_secret, '') FROM users WHERE id = 1`:  "",
		`SELECT webhook_url FROM notify_channels WHERE user_id = 1`: "",
	} {
		if got := valueIn(t, od.DB, q); got != want {
			t.Errorf("%s: %q, want %q", q, got, want)
		}
	}
}

// A backup made with a password holds the secrets themselves: those are what a
// restore of it brings, whatever this machine has now.
func TestAProtectedRestoreBringsItsOwnSecrets(t *testing.T) {
	f, _ := aServer(t)
	d, err := db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	plantMore(t, d.DB)
	out, _, err := Create(ctx, Options{Folders: f, Parts: everything, Output: filepath.Join(t.TempDir(), "b.zip"), Password: "a long password",
		SecretSettings: keepOpts.SecretSettings, ScrubSettings: keepOpts.ScrubSettings})
	if err != nil {
		t.Fatal(err)
	}
	mustExec(t, d.DB, `UPDATE settings SET value = 'P2PKEYCHANGEDSINCE' WHERE key = 'p2pKey'`)
	in, err := Inspect(ctx, out, "a long password", f, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	o := keepOpts
	o.Parts, o.LiveDB = everything, d.DB
	req, err := in.Stage(ctx, f, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Kept)+len(req.New) != 0 {
		t.Fatalf("a protected backup's restore kept or made secrets: %q %q", req.Kept, req.New)
	}
	d.Close()
	if r := Apply(f); r == nil || !r.OK {
		t.Fatalf("%+v", r)
	}
	d, err = db.Open(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if got := valueIn(t, d.DB, `SELECT value FROM settings WHERE key = 'p2pKey'`); got != "P2PKEYPLANTED" {
		t.Fatalf("the protected backup's own key: %q", got)
	}
}
