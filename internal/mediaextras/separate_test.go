// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package mediaextras

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestSeparatorCommand(t *testing.T) {
	prog, args, err := SeparatorCommand([]string{"demucs", "--two-stems", "vocals", "-o", "{output}", "{input}"}, "/music/$(rm -rf ~); x.flac", "/tmp/out dir")
	if err != nil || prog != "demucs" || !reflect.DeepEqual(args, []string{"--two-stems", "vocals", "-o", "/tmp/out dir", "/music/$(rm -rf ~); x.flac"}) {
		t.Errorf("%q %q %v", prog, args, err)
	}
	// A word can hold a placeholder among other text.
	if _, args, _ := SeparatorCommand([]string{"sep", "--in={input}", "--out={output}/parts"}, "a.flac", "/o"); !reflect.DeepEqual(args, []string{"--in=a.flac", "--out=/o/parts"}) {
		t.Errorf("%q", args)
	}
	for name, words := range map[string][]string{
		"nothing": nil, "no input": {"sep", "{output}"}, "no output": {"sep", "{input}"}, "the program itself is a placeholder": {"{input}", "{output}"},
	} {
		if _, _, err := SeparatorCommand(words, "a", "b"); !errors.Is(err, ErrSeparator) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestCollectStems(t *testing.T) {
	out, dst := t.TempDir(), filepath.Join(t.TempDir(), "stems")
	flac := append([]byte("fLaC\x00\x00\x00\x22"), make([]byte, 100)...)
	wav := append([]byte("RIFF\x24\x00\x00\x00WAVEfmt "), make([]byte, 100)...)
	write := func(rel string, b []byte) {
		p := filepath.Join(out, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("htdemucs/Song/vocals.wav", wav)
	write("htdemucs/Song/no_vocals.wav", wav)
	write("htdemucs/Song/drums.flac", flac)
	write("htdemucs/Song/notes.txt", []byte("hello"))
	write("htdemucs/Song/bass.wav", []byte("#!/bin/sh\necho not audio"))
	write("htdemucs/Song/other.wav", nil)
	write("htdemucs/Song/unknown-part.wav", wav)
	if err := os.Symlink("/etc/hosts", filepath.Join(out, "htdemucs", "Song", "accompaniment.wav")); err != nil {
		t.Skip("no symbolic links here")
	}
	// An older upload of another type is replaced, not left beside the new one.
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "vocals.flac"), flac, 0o644); err != nil {
		t.Fatal(err)
	}
	kept, err := CollectStems(out, dst)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(kept)
	if !reflect.DeepEqual(kept, []string{"drums", "instrumental", "vocals"}) {
		t.Fatalf("kept %v", kept)
	}
	names := []string{}
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !reflect.DeepEqual(names, []string{"drums.flac", "instrumental.wav", "vocals.wav"}) {
		t.Errorf("in the stems folder: %v", names)
	}
	// Nothing usable is an error that says what was looked for.
	empty := t.TempDir()
	if _, err := CollectStems(empty, dst); err == nil || !strings.Contains(err.Error(), "vocals") {
		t.Errorf("empty output: %v", err)
	}
}

func TestSeparatedList(t *testing.T) {
	dir := t.TempDir()
	if len(Separated(dir)) != 0 {
		t.Error("parts listed with no list")
	}
	if err := os.WriteFile(filepath.Join(dir, SeparatedList), []byte("vocals\nno_vocals\n../../etc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Separated(dir); !got["vocals"] || !got["instrumental"] || len(got) != 2 {
		t.Errorf("%v", got)
	}
}

func TestApproxArgsCancelTheCentre(t *testing.T) {
	args := strings.Join(ApproxArgs("out.m4a"), " ")
	// Not c1=c1-c0: the two channels would cancel on a mono speaker.
	if strings.Contains(args, "c1=c1-c0") {
		t.Error("the two channels are each other's opposite, and cancel when a speaker adds them")
	}
	for _, want := range []string{"c0=c0-c1", "c1=c0-c1", "lowpass=f=120", "-c:a aac", "out.m4a"} {
		if !strings.Contains(args, want) {
			t.Errorf("no %q in %s", want, args)
		}
	}
}
