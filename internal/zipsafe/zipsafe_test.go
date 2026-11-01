package zipsafe

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
)

func build(t *testing.T, comment string, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range names {
		if _, err := w.Create(n); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.SetComment(comment); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func check(b []byte, l Limits) error { return Check(bytes.NewReader(b), int64(len(b)), l) }

func TestCheck(t *testing.T) {
	small := build(t, "", "a.txt", "b/c.txt")
	if err := check(small, Limits{MaxEntries: 2, MaxDirectory: 1 << 10}); err != nil {
		t.Errorf("%v", err)
	}
	if err := check(small, Limits{MaxEntries: 1}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("entries: %v", err)
	}
	if err := check(small, Limits{MaxDirectory: 10}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("directory: %v", err)
	}
	for _, b := range [][]byte{nil, []byte("short"), bytes.Repeat([]byte("x"), 200)} {
		if err := check(b, Limits{}); !errors.Is(err, ErrDamaged) {
			t.Errorf("%q: %v", b, err)
		}
	}
}

// What it looks at has to be what the reader will use: a signature in the comment that the
// reader passes over mustn't hide the real numbers.
func TestADecoyInTheCommentDoesntHideTheDirectory(t *testing.T) {
	// Too close to the end to be a record: the reader goes on past it to the real one.
	b := build(t, "PK\x05\x06 too short", "a.txt", "b.txt", "c.txt")
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil || len(zr.File) != 3 {
		t.Fatalf("the reader sees %v %v", zr, err)
	}
	if err := check(b, Limits{MaxEntries: 2}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("the decoy passed: %v", err)
	}
	// Says a longer comment follows than there is: the reader refuses it, so this does too.
	b = build(t, "PK\x05\x06"+string(make([]byte, 16))+"\xff\xff", "a.txt", "b.txt", "c.txt")
	if _, err := zip.NewReader(bytes.NewReader(b), int64(len(b))); err == nil {
		t.Fatal("the reader has changed how it finds the end: this package has to follow it")
	}
	if err := check(b, Limits{}); !errors.Is(err, ErrDamaged) {
		t.Errorf("%v", err)
	}
	// A whole record in the comment is the one the reader uses, and it says nothing is there.
	b = build(t, "PK\x05\x06"+string(make([]byte, 18)), "a.txt", "b.txt", "c.txt")
	zr, err = zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil || len(zr.File) != 0 {
		t.Fatalf("the reader has changed: %v %v", zr, err)
	}
	if err := check(b, Limits{MaxEntries: 2}); err != nil {
		t.Errorf("%v", err)
	}
}

// A real zip64 file: more entries than the ordinary record can say.
func TestZip64(t *testing.T) {
	names := make([]string, 70000)
	for i := range names {
		names[i] = fmt.Sprintf("p%d.jpg", i)
	}
	b := build(t, "", names...)
	if err := check(b, Limits{MaxEntries: 100000}); err != nil {
		t.Errorf("%v", err)
	}
	if err := check(b, Limits{MaxEntries: 65535}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("the marker passed unread: %v", err)
	}
	// The ordinary record's markers with nothing behind them are as large as they say.
	plain := build(t, "", "a.txt")
	binary.LittleEndian.PutUint32(plain[len(plain)-endLen+12:], 0xffffffff)
	if err := check(plain, Limits{MaxDirectory: 1 << 24}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("no record: %v", err)
	}
}

func TestCheckNames(t *testing.T) {
	b := build(t, "", "a/b/c.txt", "a/d.txt")
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckNames(zr, Limits{MaxName: 9, MaxFolders: 3}); err != nil {
		t.Errorf("%v", err)
	}
	if err := CheckNames(zr, Limits{MaxName: 8}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("name: %v", err)
	}
	if err := CheckNames(zr, Limits{MaxFolders: 2}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("folders: %v", err)
	}
}

func FuzzCheck(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("PK\x05\x06"))
	f.Add(append(make([]byte, 30), "PK\x05\x06\x00\x00\x00\x00\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\x00\x00"...))
	f.Fuzz(func(t *testing.T, b []byte) {
		_ = check(b, Limits{MaxEntries: 10, MaxDirectory: 100})
	})
}
