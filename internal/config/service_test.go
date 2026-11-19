// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Every service keeps the folder its package gave it, whatever setup or Settings wrote for
// someone's own copy: the pointer is never read before WMS_DATA_DIR or -data. Each case is how
// a package starts its service (packaging/...), with the service's folder not made yet, and a
// pointer beside it that's fine, names a folder that isn't there, or can't be read.
func TestServicesKeepTheirFolder(t *testing.T) {
	services := []struct {
		name string
		env  string   // WMS_DATA_DIR as the package sets it, under the test's home
		args []string // what the service passes after "serve"
		from string
	}{
		{"macOS launchd (plist WMS_DATA_DIR)", "Library/Application Support/WittyMediaServer", nil, DataFromEnv},
		{"Linux systemd, .deb and .rpm (unit WMS_DATA_DIR)", "var/lib/wittymediaserver", nil, DataFromEnv},
		{"FreeBSD rc.d (WMS_DATA_DIR)", "var/db/wittymediaserver", nil, DataFromEnv},
		{"Snap (WMS_DATA_DIR=$SNAP_COMMON)", "snap/common", nil, DataFromEnv},
		{"Docker (WMS_DATA_DIR=/config)", "config", nil, DataFromEnv},
		{"Windows service (-data ProgramData)", "", []string{"-data", "ProgramData/WittyMediaServer"}, DataFromFlag},
	}
	pointers := map[string]func(home string){
		"no pointer": func(string) {},
		"a pointer to Documents": func(home string) {
			docs := filepath.Join(home, "Documents", "WittyMediaServer")
			if err := os.MkdirAll(docs, 0o750); err != nil {
				t.Fatal(err)
			}
			if err := WritePointer(Pointer{DataDir: docs, Choice: ChoiceDocuments}); err != nil {
				t.Fatal(err)
			}
		},
		"a pointer to a folder that isn't there": func(home string) {
			if err := WritePointer(Pointer{DataDir: filepath.Join(home, "unplugged"), Choice: ChoiceCustom}); err != nil {
				t.Fatal(err)
			}
		},
		"a pointer that can't be read": func(string) {
			if err := os.MkdirAll(filepath.Dir(PointerFile()), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(PointerFile(), []byte("not json"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for _, s := range services {
		for pname, pointer := range pointers {
			t.Run(s.name+", "+pname, func(t *testing.T) {
				home := isolate(t)
				pointer(home)
				want := ""
				args := append([]string(nil), s.args...)
				if s.env != "" {
					want = filepath.Join(home, filepath.FromSlash(s.env))
					t.Setenv("WMS_DATA_DIR", want)
				} else {
					want = filepath.Join(home, filepath.FromSlash(args[1]))
					args[1] = want
				}
				// On macOS the pointer sits in the folder the service uses, so writing it may have
				// made that folder already: only reading the settings mustn't.
				_, statErr := os.Stat(want)
				existed := statErr == nil
				c, err := ParseOnly(args)
				if err != nil {
					t.Fatalf("the service didn't start: %v", err)
				}
				if c.DataDir != want || c.DataFrom != s.from {
					t.Fatalf("data folder %s from %s, want %s from %s", c.DataDir, c.DataFrom, want, s.from)
				}
				// Setup asks where to keep the data only when nothing chose a folder.
				if c.DataFrom == DataFromDefault {
					t.Fatal("a service would be asked where to keep its data")
				}
				if _, err := os.Stat(want); err == nil && !existed {
					t.Fatal("reading the settings made the service's folder")
				}
			})
		}
	}
}
