// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// Package backup makes a ZIP of a server's data and metadata, and puts one back. An
// archive is treated as hostile the whole way through, even one this server made: it
// may have been changed since, or come from someone else.
//
// Making one needs nothing stopped: the database is copied with VACUUM INTO. Bringing
// one back happens in two steps, as with a reset: it's checked and unpacked beside the
// data while the server runs, and the swap is made as the server next starts, before
// anything has these files open. What was there before is moved aside first, and is
// moved back if anything goes wrong.
//
// Code is never part of a backup. Modules, game cores and helpers come from their signed
// releases, so an archive can't put a program on this machine.
package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// FormatVersion is the layout of an archive. An archive with a higher one is refused.
const FormatVersion = 1

// File names inside an archive.
const (
	ManifestName = "manifest.json"
	ReadmeName   = "README.txt"
	PayloadName  = "payload.wmsenc" // the whole inner archive, when protected with a password
	dbEntry      = "database/wms.db"
)

// DirName is the folder in the data folder that holds backups. A reset leaves it alone.
const DirName = "backups"

// Folders are where the things a backup covers live.
type Folders struct {
	Data   string
	Images string // artwork; usually <data>/cache/images
}

// Dir is the backups folder.
func (f Folders) Dir() string { return filepath.Join(f.Data, DirName) }

// Part is one of the things a backup can hold.
type Part struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Desc      string   `json:"desc"`
	Default   bool     `json:"default"`
	Needs     []string `json:"needs,omitempty"` // parts it makes no sense without
	HasTables bool     `json:"hasTables,omitempty"`
}

// The parts, in the order they're offered.
const (
	PartLibrary    = "library"
	PartSettings   = "settings"
	PartUsers      = "users"
	PartHistory    = "history"
	PartChannels   = "channels"
	PartArtwork    = "artwork"
	PartSaves      = "saves"
	PartModules    = "modules"
	PartMetaFiles  = "metadatafiles"
	PartLyrics     = "lyrics"
	PartRecordings = "recordings"
)

// Parts lists them. Lyrics corrections and recordings are left out unless asked for.
func Parts() []Part {
	return []Part{
		{ID: PartLibrary, Label: "Libraries and metadata", Desc: "Your libraries, and everything known about what's in them: titles, matches, edits, locks and history. The media files themselves are never in a backup.", Default: true, HasTables: true},
		{ID: PartSettings, Label: "Settings", Desc: "Everything under Settings, your server's name and logo, and your own themes.", Default: true, HasTables: true},
		{ID: PartUsers, Label: "Accounts and profiles", Desc: "Everyone's name, profile and password (kept only as a hash). Sign-ins are never in a backup: after a restore, everyone signs in again.", Default: true, HasTables: true},
		{ID: PartHistory, Label: "Watch history and personal data", Desc: "Progress, what's been watched, favourites, ratings, playlists, collections, notes and share links.", Default: true, Needs: []string{PartLibrary, PartUsers}, HasTables: true},
		{ID: PartChannels, Label: "Channels and TV", Desc: "WittyTV channels, tuners, the guide, scheduled recordings, bumpers and channel logos.", Default: true, Needs: []string{PartLibrary}, HasTables: true},
		{ID: PartArtwork, Label: "Artwork", Desc: "Posters, backdrops and logos the server has saved. They can be fetched again, so leave this out if space is tight.", Default: true},
		{ID: PartSaves, Label: "Game saves", Desc: "Saves, saved states, game settings, bezels and backgrounds for every account.", Default: true, Needs: []string{PartLibrary, PartUsers}, HasTables: true},
		{ID: PartModules, Label: "Module settings", Desc: "Which modules are on and how each one updates. Their programs aren't kept: they install again from their signed releases.", Default: true},
		{ID: PartMetaFiles, Label: "Metadata files", Desc: "The folder of text files for fixing titles by hand, if you've made one. It stays readable inside the ZIP unless you set a password.", Default: true},
		{ID: PartLyrics, Label: "Lyrics corrections", Desc: "Lyrics someone corrected on this server. Left out unless you ask, because they're the words of songs.", Default: false},
		{ID: PartRecordings, Label: "TV recordings", Desc: "Recordings kept in the data folder. These can be very large.", Default: false, Needs: []string{PartChannels}},
	}
}

// partByID finds one.
func partByID(id string) (Part, bool) {
	for _, p := range Parts() {
		if p.ID == id {
			return p, true
		}
	}
	return Part{}, false
}

// CleanParts checks a list of part IDs against what exists, and returns it in order.
func CleanParts(ids []string) ([]string, error) {
	want := map[string]bool{}
	for _, id := range ids {
		if _, ok := partByID(id); !ok {
			return nil, fmt.Errorf("there's no such part of a backup: %q", id)
		}
		want[id] = true
	}
	var out []string
	for _, p := range Parts() {
		if want[p.ID] {
			out = append(out, p.ID)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("choose at least one thing to include")
	}
	return out, nil
}

// DefaultParts are those on unless someone says otherwise.
func DefaultParts() []string {
	var out []string
	for _, p := range Parts() {
		if p.Default {
			out = append(out, p.ID)
		}
	}
	return out
}

// tableParts says which part each database table belongs to. A table nobody has chosen
// for isn't in any backup, which is the safe direction, and TestEveryTableIsClassified
// fails until someone chooses.
var tableParts = map[string]string{
	"libraries": PartLibrary, "library_paths": PartLibrary, "items": PartLibrary, "shows": PartLibrary, "seasons": PartLibrary,
	"title_meta": PartLibrary, "meta_history": PartLibrary, "title_brands": PartLibrary, "people": PartLibrary, "credits": PartLibrary, "credits_read": PartLibrary, "logos": PartLibrary, "item_links": PartLibrary,
	"library_moves": PartLibrary, "categories": PartLibrary, "match_rules": PartLibrary, "listing_titles": PartLibrary, "listings": PartLibrary,
	"photo_meta": PartLibrary, "creators": PartLibrary, "video_meta": PartLibrary, "podcasts": PartLibrary, "radio_meta": PartLibrary,
	"game_archives": PartLibrary, "game_meta": PartLibrary, "game_meta_sources": PartLibrary,
	"game_files": PartLibrary, "game_review": PartLibrary, "game_decisions": PartLibrary, "game_discs": PartLibrary,
	"ltc_rules": PartLibrary, "music_recording_ids": PartLibrary, "music_video_links": PartLibrary,
	"media_markers": PartLibrary, "watch_offers_cache": PartLibrary,

	"settings": PartSettings,
	"users":    PartUsers,

	"progress": PartHistory, "play_log": PartHistory, "item_stats": PartHistory, "item_reactions": PartHistory, "item_comments": PartHistory,
	"annotations": PartHistory, "collections": PartHistory, "collection_items": PartHistory, "collection_shows": PartHistory,
	"photo_albums": PartHistory, "photo_album_items": PartHistory, "saved_views": PartHistory, "creator_subs": PartHistory,
	"lyrics_offsets": PartHistory, "tv_favourites": PartHistory, "tv_reminders": PartHistory, "tv_sets": PartHistory, "tv_set_channels": PartHistory, "tv_set_order": PartHistory, "share_links": PartHistory,
	"screen_time": PartHistory, "profile_watch": PartHistory, "profile_events": PartHistory,

	"channels": PartChannels, "channel_days": PartChannels, "channel_history": PartChannels, "channel_bumpers": PartChannels,
	"tuners": PartChannels, "tuner_channels": PartChannels, "epg_sources": PartChannels, "epg_channels": PartChannels,
	"epg_programmes": PartChannels, "recordings": PartChannels, "recording_rules": PartChannels,

	"wittyplay_backgrounds": PartSaves, "wittyplay_bezels": PartSaves, "wittyplay_controls": PartSaves, "wittyplay_core_options": PartSaves,
	"wittyplay_game_meta": PartSaves, "wittyplay_patches": PartSaves, "wittyplay_profiles": PartSaves, "wittyplay_settings": PartSaves,
	"wittyplay_presence": PartSaves, "wittyplay_kids_friends": PartSaves,
}

// isCoreCache: what a core worked out for itself beside a save, such as compiled shaders. It's made
// again for nothing, can run to gigabytes, and is only worth having on the machine that made it.
func isCoreCache(rel string) bool {
	parts := strings.Split(rel, "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == "system" && parts[i+2] == "cache" {
			return true
		}
	}
	return false
}

// neverTables stay out of every backup, for a reason:
//   - sign-ins: a backup must not let anyone in, and nobody expects a sign-in to come back.
//   - devices that may connect peer to peer: the same. They add themselves again at home.
//   - custom cores: their programs aren't kept, so the rows would point at nothing.
//   - game PCs: pairing keys live beside them, on this machine.
//   - measurements and finished jobs: of this machine, at that time.
var neverTables = map[string]bool{
	"schema": true, "sqlite_sequence": true, "sessions": true, "p2p_devices": true,
	"custom_cores": true, "custom_core_versions": true, "custom_consoles": true, "game_core_choice": true, "game_core_plays": true, "game_core_audit": true,
	"wittycouch_hosts": true, "wittyrigs_rigs": true, "wittyrigs_apps": true,
	"metrics_hourly": true, "stream_bytes": true, "task_history": true,
	"ltc_runs": true, "ltc_files": true, "ltc_memo": true,
}

// FileEntry is one file in an archive.
type FileEntry struct {
	Path   string `json:"path"`
	Part   string `json:"part"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// PartSummary is what the manifest says about one part.
type PartSummary struct {
	ID    string `json:"id"`
	Files int    `json:"files"`
	Bytes int64  `json:"bytes"`
}

// Manifest describes an archive: what made it, what's in it, and a hash of each file.
// The hashes catch damage. They can't prove nobody changed the archive on purpose, as
// anyone can write new ones: only a password can do that.
type Manifest struct {
	Format        int           `json:"format"`
	App           string        `json:"app"`
	ServerVersion string        `json:"serverVersion"`
	Created       time.Time     `json:"created"`
	Schema        db.Versions   `json:"schema"`
	Protected     bool          `json:"protected"`       // made with a password, and holds secrets
	Auto          bool          `json:"auto,omitempty"`  // made by the schedule
	Parts         []PartSummary `json:"parts"`           // what was asked for and found
	Files         []FileEntry   `json:"files,omitempty"` // left out of the outer manifest of a protected archive
	DataDir       string        `json:"dataDir,omitempty"`
	Crypt         *CryptHeader  `json:"crypt,omitempty"` // how the payload is protected
}

// Info is a backup kept on this server.
type Info struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Created   time.Time `json:"created"`
	Auto      bool      `json:"auto"`
	Protected bool      `json:"protected"`
	Version   string    `json:"version,omitempty"`
}

// Names of backups this package makes. Anything else in the folder isn't listed, can't be
// downloaded or deleted through here, and so a path can't be passed in as a name.
const (
	namePrefix = "wms-backup-"
	autoPrefix = "wms-backup-auto-"
	nameSuffix = ".zip"
	stampForm  = "20060102-150405"
)

// ValidName says whether a name is one of this folder's backups, and nothing cleverer.
func ValidName(name string) bool {
	if !strings.HasPrefix(name, namePrefix) || !strings.HasSuffix(name, nameSuffix) || len(name) > 120 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return false
		}
	}
	return !strings.Contains(name, "..") && name == filepath.Base(name)
}

// PathOf finds a backup by name.
func (f Folders) PathOf(name string) (string, error) {
	if !ValidName(name) {
		return "", errors.New("no such backup")
	}
	p := filepath.Join(f.Dir(), name)
	st, err := os.Lstat(p)
	if err != nil || !st.Mode().IsRegular() {
		return "", errors.New("no such backup")
	}
	return p, nil
}

// List returns the backups in the folder, newest first.
func List(f Folders) []Info {
	out := []Info{}
	entries, err := os.ReadDir(f.Dir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !ValidName(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		in := Info{Name: e.Name(), Size: fi.Size(), Created: fi.ModTime(), Auto: strings.HasPrefix(e.Name(), autoPrefix)}
		if m, err := peekManifest(filepath.Join(f.Dir(), e.Name())); err == nil {
			in.Created, in.Protected, in.Version = m.Created, m.Crypt != nil, m.ServerVersion
		}
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// CountUploads says how many files with this start to their name are waiting in the folder.
func CountUploads(f Folders, prefix string) int {
	entries, err := os.ReadDir(f.Dir())
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			n++
		}
	}
	return n
}

// Delete removes a backup from the folder.
func Delete(f Folders, name string) error {
	p, err := f.PathOf(name)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

// Prune keeps the newest few scheduled backups. Ones someone made by hand are theirs.
func Prune(f Folders, keep int) (removed []string) {
	if keep < 1 {
		keep = 1
	}
	n := 0
	for _, b := range List(f) {
		if !b.Auto {
			continue
		}
		if n++; n > keep {
			if err := Delete(f, b.Name); err == nil {
				removed = append(removed, b.Name)
			}
		}
	}
	return removed
}

// fileSource says where one kind of file comes from, and where it goes in an archive.
type fileSource struct {
	Part   string
	Prefix string // in the archive, with the slash
	Root   func(f Folders) string
	Check  func(rel string, head []byte) error
	Skip   func(rel string) bool
}

// sources are every folder a backup takes from. Nothing else in the data folder is read:
// not logs, reports, caches, modules, cores, BIOS files, keys or certificates.
func sources() []fileSource {
	data := func(parts ...string) func(Folders) string {
		return func(f Folders) string { return filepath.Join(append([]string{f.Data}, parts...)...) }
	}
	return []fileSource{
		{Part: PartArtwork, Prefix: "artwork/", Root: func(f Folders) string { return f.Images }, Check: mustBePicture},
		{Part: PartSettings, Prefix: "branding/", Root: data("branding"), Check: mustBePicture},
		{Part: PartChannels, Prefix: "channel-logos/", Root: data("channel-logos"), Check: mustBePicture},
		{Part: PartChannels, Prefix: "bumpers/", Root: data("bumpers"), Check: notAProgram},
		{Part: PartSaves, Prefix: "saves/", Root: data("gaming", "saves"), Check: notAProgram, Skip: isCoreCache},
		{Part: PartSaves, Prefix: "bezels/", Root: data("gaming", "bezels"), Check: mustBePicture},
		{Part: PartSaves, Prefix: "backgrounds/", Root: data("gaming", "backgrounds"), Check: mustBePicture},
		{Part: PartMetaFiles, Prefix: "metadata/", Root: data("metadata"), Check: metadataFile},
		{Part: PartLyrics, Prefix: "lyrics-corrections/", Root: data("lyrics-corrections"), Check: mustBeJSONFile},
		{Part: PartRecordings, Prefix: "recordings/", Root: data("recordings"), Check: notAProgram},
		{Part: PartSettings, Prefix: "themes/", Root: data("themes"), Check: mustBeZipName, Skip: func(rel string) bool { return strings.Contains(rel, "/") }},
		{Part: PartModules, Prefix: "modules/", Root: data("modules"), Check: moduleSettingsFile, Skip: func(rel string) bool { return !moduleSettings[rel] }},
	}
}

// moduleSettings are the only files taken from the modules folder.
var moduleSettings = map[string]bool{"state.json": true, "library-names.json": true}

// walkRegular calls fn for each ordinary file under root. Links and other special
// files are skipped: they would lead somewhere else, and a restore wouldn't take them.
func walkRegular(root string, fn func(rel string, fi fs.FileInfo) error) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root || errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil || !fi.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		return fn(filepath.ToSlash(rel), fi)
	})
}
