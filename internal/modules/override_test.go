// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package modules

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/update"
	"github.com/wittywilla/wittymediaserver/pkg/sdui"
	"github.com/wittywilla/wittymodule"
)

// archiveOf writes a release archive into dir and, with a key, its signed checksum list. It
// answers the archive's path and the list and signature (nil without a key).
func archiveOf(t *testing.T, dir string, man wittymodule.Manifest, files map[string][]byte, key ed25519.PrivateKey) (path string, sums, sig []byte) {
	t.Helper()
	raw, _ := json.Marshal(man)
	all := map[string][]byte{wittymodule.ManifestFile: raw}
	for n, b := range files {
		all[n] = b
	}
	top := man.Top(runtime.GOOS, runtime.GOARCH)
	name := top + ".tar.gz"
	if runtime.GOOS == "windows" {
		name = top + ".zip"
	}
	path = filepath.Join(dir, name)
	_ = os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(path, packFiles(top, all), 0o644); err != nil {
		t.Fatal(err)
	}
	if key != nil {
		var lines []string
		for n, b := range all {
			lines = append(lines, fmt.Sprintf("%s  %s/%s", sha(b), top, n))
		}
		sort.Strings(lines)
		sums = []byte(strings.Join(lines, "\n") + "\n")
		sig = update.Sign(key, sums)
	}
	return path, sums, sig
}

const newUI = `{"format":"wittymodule-sdui","formatVersion":1,"module":"wittynew","catalog":2,"theme":{"scope":"wittynew"},
"pages":[{"id":"wittynew","kind":"custom","title":"New things","sections":[{"id":"recent","component":"Shelf","source":{"type":"recentlyAdded"}}]}],
"navigation":[{"id":"wittynew","type":"page","page":"wittynew"}]}`

func newBundle() (wittymodule.Manifest, map[string][]byte) {
	man := wittymodule.Manifest{ID: "wittynew", Name: "Witty New", Version: "0.1.0", Repo: "someone/WittyNew", Delivery: wittymodule.DeliveryWeb,
		Entry: "index.js", API: SDUIAPI, MinServer: "0.4.0", MaxServer: "0.5.x", Permissions: []string{wittymodule.PermissionNetwork}}
	return man, map[string][]byte{"index.js": []byte("export default 1"), sdui.ModuleFile: []byte(newUI)}
}

func stage(t *testing.T, m *Manager, path string, sums, sig []byte, key string) *UploadInfo {
	t.Helper()
	info, err := m.StageUpload(Upload{Archive: path, ArchiveName: filepath.Base(path), Sums: sums, Sig: sig, Key: key, By: 1})
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// A module this server doesn't know, unsigned, from a file: the most it takes to install anything.
// It needs every go-ahead named, the password (checked by the server's route) and its name typed;
// it then runs as someone else's code, is kept across a restart, refused once a file changes, and
// goes altogether when the override is taken back.
func TestAnUnsignedModuleFromAFile(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	man, files := newBundle()
	path, _, _ := archiveOf(t, t.TempDir(), man, files, nil)
	info := stage(t, m, path, nil, nil, "")
	want := []string{OverrideLocalFile, OverrideCustom, OverrideUnsigned}
	if info.Refusal != "" || info.Token == "" || info.Signed || info.Known || !slices.Equal(info.NeedsOverride.Kinds, want) || info.NeedsOverride.Severity != 4 {
		t.Fatalf("staged: %+v %+v", info, info.NeedsOverride)
	}
	for _, k := range want {
		if !slices.ContainsFunc(info.Problems, func(p UploadProblem) bool { return p.Kind == k && p.Text != "" }) {
			t.Errorf("no words for %s: %+v", k, info.Problems)
		}
	}
	for name, a := range map[string]*OverrideAccept{
		"nothing":                      nil,
		"some of it":                   {Accept: []string{OverrideLocalFile, OverrideCustom}, Typed: "wittynew"},
		"without the name":             {Accept: want},
		"the wrong name":               {Accept: want, Typed: "wittyold"},
		"a permission it doesn't want": {Accept: want, Typed: "wittynew", Grants: []string{wittymodule.PermissionDevices}},
	} {
		if err := m.InstallUpload(info.Token, 1, a); err == nil {
			t.Errorf("%s: installed", name)
		}
	}
	if err := m.InstallUpload(info.Token, 2, &OverrideAccept{Accept: want, Typed: "wittynew"}); !errors.Is(err, ErrUnknownUpload) {
		t.Errorf("another administrator's upload: %v", err)
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: want, Typed: "WittyNew", Grants: []string{wittymodule.PermissionNetwork}, By: "owner"}); err != nil {
		t.Fatal(err)
	}
	s := statusOf(t, m, "wittynew")
	if s.Version != "0.1.0" || s.Override == nil || !slices.Equal(s.Override.Kinds, want) || !slices.Equal(s.Override.Grants, []string{wittymodule.PermissionNetwork}) || s.Override.Source != "upload" {
		t.Fatalf("installed: %+v %+v", s, s.Override)
	}
	if s.UIAdds == nil || len(s.UIAdds.Pages) != 1 || s.UIAdds.Navigation[0] != "New things" {
		t.Errorf("what it adds: %+v", s.UIAdds)
	}
	if uis, _ := m.UIs(); len(uis) != 1 || uis[0].Module != "wittynew" {
		t.Errorf("its SDUI isn't in use: %v", uis)
	}
	m.Close()

	// Back after a restart, still overridden.
	m2 := w.manager()
	s = statusOf(t, m2, "wittynew")
	if s.Version != "0.1.0" || s.Override == nil {
		t.Fatalf("after a restart: %+v", s)
	}
	m2.Close()

	// A file changed afterwards is refused, as for any module.
	ver := filepath.Join(w.dir, "wittynew", "versions", "0.1.0", "index.js")
	if err := os.WriteFile(ver, []byte("export default 2"), 0o644); err != nil {
		t.Fatal(err)
	}
	m3 := w.manager()
	if s := statusOf(t, m3, "wittynew"); s.Version == "0.1.0" {
		t.Fatalf("a changed file is in use: %+v", s)
	}
	_ = os.WriteFile(ver, files["index.js"], 0o644)
	m3.Close()

	m4 := w.manager()
	defer m4.Close()
	if err := m4.RemoveOverride("wittynew", nil); err != nil {
		t.Fatal(err)
	}
	for _, st := range m4.Report().Modules {
		if st.ID == "wittynew" {
			t.Fatal("still listed after the override was taken back")
		}
	}
	if _, err := os.Stat(filepath.Join(w.dir, "wittynew")); !os.IsNotExist(err) {
		t.Errorf("its files are still there: %v", err)
	}
}

func statusOf(t *testing.T, m *Manager, id string) Status {
	t.Helper()
	for range 6000 {
		for _, st := range m.Report().Modules {
			if st.ID == id && st.Phase != PhaseInstalling {
				return st
			}
		}
		sleepBriefly()
	}
	t.Fatalf("no %s", id)
	return Status{}
}

// Signed by a key this server doesn't trust for it: installable as that publisher's, with the
// fingerprint to compare, and checked against that key from then on.
func TestAModuleSignedByAnotherKey(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	man, files := newBundle()
	path, sums, sig := archiveOf(t, t.TempDir(), man, files, key)
	if info := stage(t, m, path, sums, sig, ""); info.Refusal == "" || !strings.Contains(info.Refusal, "public key") {
		t.Errorf("a signature from nobody known, with no key: %+v", info)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if info := stage(t, m, path, sums, sig, base64.StdEncoding.EncodeToString(other)); info.Refusal == "" {
		t.Errorf("a signature that isn't the given key's: %+v", info)
	}
	b64 := base64.StdEncoding.EncodeToString(pub)
	info := stage(t, m, path, sums, sig, b64)
	want := []string{OverrideLocalFile, OverrideCustom, OverrideUntrustedKey}
	if info.Refusal != "" || !info.Signed || info.Trusted || info.Fingerprint != Fingerprint(pub) || !slices.Equal(info.NeedsOverride.Kinds, want) {
		t.Fatalf("staged: %+v", info)
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: want, Typed: "wittynew", By: "owner"}); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, m, "wittynew"); s.Override == nil || s.Override.Fingerprint != Fingerprint(pub) {
		t.Fatalf("installed: %+v", s.Override)
	}
}

// This project's own signed release, from a file: only that it came from a file needs saying, and
// it keeps the official rules.
func TestAnOfficialReleaseFromAFile(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	man := wittymodule.Manifest{}
	_ = json.Unmarshal(manifestJSON("0.4.0", "0.4.0", "0.5.x", MinAPI), &man)
	path, sums, sig := archiveOf(t, t.TempDir(), man, map[string][]byte{entryName(): helperBytes(t), "LICENSE": []byte("l")}, w.key)
	info := stage(t, m, path, sums, sig, "")
	if info.Refusal != "" || !info.Trusted || !info.Known || !slices.Equal(info.NeedsOverride.Kinds, []string{OverrideLocalFile}) || info.NeedsOverride.Severity != 2 {
		t.Fatalf("staged: %+v %+v", info, info.NeedsOverride)
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: []string{OverrideLocalFile}, Grants: []string{wittymodule.PermissionNetwork}}); err == nil {
		t.Error("granted a permission to an official module, which keeps the official rules")
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: []string{OverrideLocalFile}, By: "owner"}); err != nil {
		t.Fatal(err)
	}
	s := statusOf(t, m, "wittyphotos")
	if s.Version != "0.4.0" || s.Override == nil || s.Override.Kinds[0] != OverrideLocalFile {
		t.Fatalf("installed: %+v", s)
	}
	usesHelper(t, m, "0.4.0")
}

// What no override passes: refused with the reason, and nothing held.
func TestUploadsThatCantBeInstalled(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	dir := t.TempDir()
	man, files := newBundle()
	tooOld := man
	tooOld.API = MinAPI - 1
	tooNew := man
	tooNew.API = API + 1
	library := man
	library.Delivery, library.Entry = wittymodule.DeliveryLibrary, ""
	for name, c := range map[string]struct {
		man  wittymodule.Manifest
		want string
	}{"an API too old": {tooOld, "module API"}, "an API too new": {tooNew, "module API"}, "a library": {library, "nothing to install"}} {
		path, _, _ := archiveOf(t, filepath.Join(dir, strings.ReplaceAll(name, " ", "-")), c.man, files, nil)
		if info := stage(t, m, path, nil, nil, ""); info.Refusal == "" || !strings.Contains(info.Refusal, c.want) || info.Token != "" {
			t.Errorf("%s: %+v", name, info)
		}
	}
	notAnArchive := filepath.Join(dir, "module.json")
	_ = os.WriteFile(notAnArchive, []byte("{}"), 0o644)
	if info := stage(t, m, notAnArchive, nil, nil, ""); info.Refusal == "" {
		t.Errorf("not an archive: %+v", info)
	}
	// Whoever runs the machine keeps other publishers off: no override passes that.
	t.Setenv(LockForks, "1")
	path, _, _ := archiveOf(t, filepath.Join(dir, "locked"), man, files, nil)
	if info := stage(t, m, path, nil, nil, ""); info.Refusal == "" || !strings.Contains(info.Refusal, LockForks) {
		t.Errorf("with %s: %+v", LockForks, info)
	}
}

// Outside its range with an API this server speaks: the go-ahead is "out-of-range", and the
// version runs in compatibility mode, kept from updating by itself.
func TestOutOfRangeFromAFile(t *testing.T) {
	w := newWorld(t)
	m := w.manager()
	defer m.Close()
	man, files := newBundle()
	man.MinServer, man.MaxServer = "0.9.0", "0.9.x"
	path, _, _ := archiveOf(t, t.TempDir(), man, files, nil)
	info := stage(t, m, path, nil, nil, "")
	if info.Refusal != "" || !slices.Contains(info.NeedsOverride.Kinds, OverrideOutOfRange) {
		t.Fatalf("staged: %+v", info)
	}
	if err := m.InstallUpload(info.Token, 1, &OverrideAccept{Accept: info.NeedsOverride.Kinds, Typed: "wittynew"}); err != nil {
		t.Fatal(err)
	}
	if s := statusOf(t, m, "wittynew"); s.Version != "0.1.0" || !slices.Contains(s.Override.Kinds, OverrideOutOfRange) {
		t.Fatalf("installed: %+v", s)
	}
}

func sleepBriefly() { time.Sleep(10 * time.Millisecond) }

// forkAccept is the go-ahead choosing a fork needs where the sandbox for it is a Mac's.
func forkAccept(id string) *OverrideAccept {
	return &OverrideAccept{Accept: []string{OverrideWeakSandbox}, Typed: id, By: "admin"}
}
