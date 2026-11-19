// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package datahome

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// readmeFirst starts the README the server writes: one that starts otherwise is someone's own, and stays.
const readmeFirst = "WittyMediaServer's data folder"

// readme says what each part of the data folder is, for someone who opens it. docs/DATA-FOLDER.md
// says the same at length.
var readme = readmeFirst + `
==============================

This folder holds WittyMediaServer's settings, database and everything it makes.
Stop the server before you copy, move or delete anything in it. To move the whole
folder, use Settings, Data folder: it copies, checks and switches.

Settings and library (back these up; Settings, Backup does it for you)
  wms.db, wms.db-wal, wms.db-shm   your libraries, accounts, watch history and settings
  settings.ini                     the port, as you or Settings set it
  certs                            HTTPS certificates
  ui_config.json, ui_config.*      the layout of pages and menus, and earlier ones

Things you made or added (keep)
  branding, themes, bumpers, channel-logos, lyrics-corrections, metadata
  recordings                       recorded TV, when it's kept here
  backups                          backups the server made

Modules
  modules                          modules you added, and what each one keeps

Logs and reports (safe to delete; useful when something goes wrong)
  logs, crash, diagnostics

Safe to delete while the server is stopped (made again when needed)
  cache                            artwork, subtitles, lyrics and lists, downloaded again
  transcode                        temporary video for players that need it converted
  separator                        songs split for karaoke, which takes a while to redo

The server's own bookkeeping (leave alone)
  instance.lock, instance.json, setup-code.txt, *-request.json, *-result.json,
  moved-from.json, update-withdrawn.json, restore-staged, before-reset-*
`

// WriteReadme puts the README in the data folder, or brings the server's own up to date.
func WriteReadme(data string) {
	p := filepath.Join(data, "README.txt")
	text := readme
	if runtime.GOOS == "windows" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	want := []byte(text)
	if b, err := os.ReadFile(p); err == nil {
		if !bytes.HasPrefix(b, []byte(readmeFirst)) || bytes.Equal(b, want) {
			return
		}
	}
	_ = os.WriteFile(p, want, 0o600)
}
