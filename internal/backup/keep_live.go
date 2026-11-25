// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package backup

import (
	"context"
	"database/sql"
	"strconv"

	"github.com/wittywilla/wittymediaserver/internal/i18n"
)

// A backup made without a password holds no secrets (stripSecrets): settings
// left out by name, settings and addresses with the secrets inside them taken
// out, accounts' two-factor sign-in, notification webhooks. Restored onto the
// machine it was made on, or any that still holds its own, that would clear
// what that machine has and nobody asked to lose: its peer-to-peer key (a new
// connection ID, and every device paired from away invited again), its
// identity (every app taking it for another server), keys to services, every
// account's two-factor sign-in.
//
// So one rule (#755): where the backup left a secret out and this machine has
// its own, this machine's is kept. It never left the machine, so keeping it
// shows it to nobody. Only where this machine has none either (a new one, or
// another) does it come back empty, or new where the server makes one.
//
// It runs as the restore is put together, with this server's database
// attached as cur, and only for a backup made without a password: a protected
// one holds the secrets themselves, which are what was asked for.

// keepLiveSecrets keeps this machine's secrets in the database being built (d,
// with cur attached), for the parts being restored. It answers what was kept
// and what this machine has none of either, in words.
func keepLiveSecrets(ctx context.Context, d *sql.DB, in *Inspection, chosen map[string]bool, o StageOptions) (kept, fresh []string, err error) {
	if chosen[PartSettings] {
		k, f, err := keepLiveSettings(ctx, d, in, o)
		if err != nil {
			return nil, nil, err
		}
		kept, fresh = append(kept, k...), append(fresh, f...)
	}
	if chosen[PartUsers] {
		k, err := keepLiveAccounts(ctx, d)
		if err != nil {
			return nil, nil, err
		}
		kept = append(kept, k...)
	}
	if chosen[PartLibrary] {
		k, err := keepLiveAddresses(ctx, d)
		if err != nil {
			return nil, nil, err
		}
		kept = append(kept, k...)
	}
	return kept, fresh, nil
}

func settingsOf(ctx context.Context, d *sql.DB, schema string) (map[string]string, error) {
	rows, err := d.QueryContext(ctx, `SELECT key, value FROM `+schema+`.settings WHERE key NOT LIKE 'schema.%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// keepLiveSettings: a setting left out keeps this machine's value; one the
// backup cleaned keeps this machine's when what the backup holds is exactly
// this machine's, cleaned (anything else changed since, and the backup's is
// what was asked for).
func keepLiveSettings(ctx context.Context, d *sql.DB, in *Inspection, o StageOptions) (kept, fresh []string, err error) {
	live, err := settingsOf(ctx, d, "cur")
	if err != nil {
		return nil, nil, err
	}
	arch, err := settingsOf(ctx, d, "main")
	if err != nil {
		return nil, nil, err
	}
	listed := map[string]bool{}
	for _, k := range o.SecretSettings {
		listed[k] = true
	}
	cleaned := func(k, v string) string {
		if f := o.ScrubSettings[k]; f != nil {
			return f(v)
		}
		return scrubAddress(v)
	}
	// What the backup left out, as it says (made by this version on), or else every secret this machine has.
	var names []string
	known := in.man != nil && in.man.Stripped != nil
	if known {
		names = in.man.Stripped
	} else {
		for k, v := range live {
			if isSecretSetting(k, listed) || cleaned(k, v) != v {
				names = append(names, k)
			}
		}
	}
	word := func(k string) string {
		if w := o.SecretWords[k]; w != "" {
			return w
		}
		return k
	}
	seen := map[string]bool{}
	for _, k := range names {
		if seen[k] {
			continue
		}
		seen[k] = true
		lv, haveLive := live[k]
		av, haveArch := arch[k]
		switch {
		case haveLive && lv != "" && (!haveArch || av == ""):
			// Left out: this machine's own.
		case haveLive && lv != "" && av != lv && cleaned(k, lv) == av:
			// Cleaned, and otherwise the same: this machine's own, secrets and all.
		case !haveLive || lv == "":
			if known && isSecretSetting(k, listed) {
				fresh = append(fresh, word(k))
			}
			continue
		default:
			continue
		}
		if _, err := d.ExecContext(ctx, `INSERT INTO main.settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, lv); err != nil {
			return nil, nil, err
		}
		kept = append(kept, word(k))
	}
	return kept, fresh, nil
}

// sameAccount is an account the backup has that this machine has too: the same
// number and the same name. Another account given the same number in between
// isn't it, and gets nothing of this one's.
const sameAccount = `EXISTS (SELECT 1 FROM cur.users c WHERE c.id = main.users.id AND c.name = main.users.name`

// keepLiveAccounts: two-factor sign-in and notification webhooks, for the
// accounts that are the same on both.
func keepLiveAccounts(ctx context.Context, d *sql.DB) ([]string, error) {
	var kept []string
	res, err := d.ExecContext(ctx, `UPDATE main.users SET
	    totp_secret = (SELECT c.totp_secret FROM cur.users c WHERE c.id = main.users.id),
	    totp_on = (SELECT c.totp_on FROM cur.users c WHERE c.id = main.users.id),
	    recovery_codes = (SELECT c.recovery_codes FROM cur.users c WHERE c.id = main.users.id)
	  WHERE (main.users.totp_secret IS NULL OR main.users.totp_secret = '') AND `+sameAccount+`
	    AND c.totp_secret IS NOT NULL AND c.totp_secret <> '')`)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		kept = append(kept, i18n.F("two-factor sign-in for {n} accounts", "n", strconv.FormatInt(n, 10)))
	}
	res, err = d.ExecContext(ctx, `UPDATE main.notify_channels SET
	    webhook_on = (SELECT c.webhook_on FROM cur.notify_channels c WHERE c.user_id = main.notify_channels.user_id),
	    webhook_url = (SELECT c.webhook_url FROM cur.notify_channels c WHERE c.user_id = main.notify_channels.user_id),
	    webhook_secret = (SELECT c.webhook_secret FROM cur.notify_channels c WHERE c.user_id = main.notify_channels.user_id)
	  WHERE main.notify_channels.webhook_url = ''
	    AND EXISTS (SELECT 1 FROM cur.notify_channels c WHERE c.user_id = main.notify_channels.user_id AND c.webhook_url <> '')
	    AND EXISTS (SELECT 1 FROM main.users u JOIN cur.users c ON c.id = u.id AND c.name = u.name WHERE u.id = main.notify_channels.user_id)`)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		kept = append(kept, i18n.F("notification webhooks for {n} accounts", "n", strconv.FormatInt(n, 10)))
	}
	return kept, nil
}

// keepLiveAddresses: an address the backup holds with its login taken out keeps
// this machine's, when it is otherwise the same.
func keepLiveAddresses(ctx context.Context, d *sql.DB) ([]string, error) {
	var kept []string
	for _, col := range scrubbedColumns {
		rows, err := d.QueryContext(ctx, `SELECT m.`+col.id+`, m.`+col.col+`, c.`+col.col+` FROM main.`+col.table+` m JOIN cur.`+col.table+` c ON c.`+col.id+` = m.`+col.id+
			` WHERE c.`+col.col+` LIKE '%://%' AND m.`+col.col+` <> c.`+col.col)
		if err != nil {
			return nil, err
		}
		type fix struct {
			id  any
			val string
		}
		var fixes []fix
		for rows.Next() {
			var id any
			var archived, live string
			if err := rows.Scan(&id, &archived, &live); err != nil {
				rows.Close()
				return nil, err
			}
			if scrubAddress(live) == archived {
				fixes = append(fixes, fix{id, live})
			}
		}
		rows.Close()
		for _, f := range fixes {
			if _, err := d.ExecContext(ctx, `UPDATE main.`+col.table+` SET `+col.col+` = ? WHERE `+col.id+` = ?`, f.val, f.id); err != nil {
				return nil, err
			}
		}
		if len(fixes) > 0 {
			kept = append(kept, i18n.F("the logins in {n} addresses ({table})", "n", strconv.Itoa(len(fixes)), "table", col.table))
		}
	}
	return kept, nil
}
