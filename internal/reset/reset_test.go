package reset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func install(t *testing.T) Folders {
	t.Helper()
	data := filepath.Join(t.TempDir(), "data")
	write(t, filepath.Join(data, "wms.db"), "accounts")
	write(t, filepath.Join(data, "games", "save.srm"), "a save")
	return Folders{Data: data}
}

func TestResetKeepsACopyAndRestoreBringsItBack(t *testing.T) {
	f := install(t)
	if err := Ask(f.Data, Request{Mode: ModeKeepCopy, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if read(t, filepath.Join(f.Data, "wms.db")) != "accounts" {
		t.Fatal("asking must change nothing")
	}
	res := Apply(f)
	if res == nil || !res.OK || res.Copy != "before-reset-20260102-030405" {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(f.Data, "wms.db")); err == nil {
		t.Fatal("the old database is still in place")
	}
	if read(t, filepath.Join(f.Data, res.Copy, "games", "save.srm")) != "a save" {
		t.Fatal("the copy is missing a file")
	}
	if Pending(f.Data) != nil || Apply(f) != nil {
		t.Fatal("a reset must happen once")
	}
	if l := Last(f.Data); l == nil || !l.OK {
		t.Fatalf("%+v", l)
	}

	// A new install happens, then the old one comes back, and the new one is kept too.
	write(t, filepath.Join(f.Data, "wms.db"), "fresh")
	if c := Copies(f.Data); len(c) != 1 || c[0].Bytes == 0 {
		t.Fatalf("%+v", c)
	}
	if err := Ask(f.Data, Request{Mode: ModeRestore, Copy: res.Copy, At: time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	back := Apply(f)
	if back == nil || !back.OK {
		t.Fatalf("%+v", back)
	}
	if read(t, filepath.Join(f.Data, "wms.db")) != "accounts" || read(t, filepath.Join(f.Data, "games", "save.srm")) != "a save" {
		t.Fatal("the copy didn't come back")
	}
	if read(t, filepath.Join(f.Data, "before-reset-20260103-030405", "wms.db")) != "fresh" {
		t.Fatal("what was there wasn't kept")
	}
	if _, err := os.Stat(filepath.Join(f.Data, res.Copy)); err == nil {
		t.Fatal("the restored copy's empty folder is still there")
	}
}

func TestDeleteRemovesEverythingWithEarlierCopies(t *testing.T) {
	f := install(t)
	write(t, filepath.Join(f.Data, "before-reset-20250101-000000", "wms.db"), "older")
	if err := Ask(f.Data, Request{Mode: ModeDelete, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	res := Apply(f)
	if res == nil || !res.OK || res.Copy != "" {
		t.Fatalf("%+v", res)
	}
	entries, err := os.ReadDir(f.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != resultFile {
			t.Errorf("left behind: %s", e.Name())
		}
	}
}

// A folder someone set elsewhere is moved aside, not deleted, even when asked to delete.
func TestFoldersElsewhereAreOnlyMovedAside(t *testing.T) {
	f := install(t)
	elsewhere := filepath.Join(t.TempDir(), "cache")
	write(t, filepath.Join(elsewhere, "thumb.jpg"), "picture")
	outside, err := Check(Folders{Data: f.Data, Others: []string{elsewhere, filepath.Join(f.Data, "cache")}}, nil)
	if err != nil || len(outside) != 1 {
		t.Fatalf("%v %v", outside, err)
	}
	if err := Ask(f.Data, Request{Mode: ModeDelete, Outside: outside, At: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	if res := Apply(f); res == nil || !res.OK || len(res.Notes) != 1 {
		t.Fatalf("%+v", res)
	}
	if read(t, filepath.Join(elsewhere, "before-reset-20260102-030405", "thumb.jpg")) != "picture" {
		t.Fatal("not moved aside")
	}
}

func TestChecksKeepMediaAndTheProgramOut(t *testing.T) {
	f := install(t)
	if _, err := Check(Folders{Data: f.Data}, []string{filepath.Join(f.Data, "movies")}); err == nil || !strings.Contains(err.Error(), "library") {
		t.Errorf("a library in the data folder: %v", err)
	}
	if _, err := Check(Folders{Data: f.Data, Program: f.Data}, nil); err == nil {
		t.Error("the program in the data folder")
	}
	if _, err := Check(Folders{Data: f.Data, Config: filepath.Join(f.Data, "wms.json")}, nil); err == nil {
		t.Error("the config file in the data folder")
	}
	// A cache folder holding a library, or holding the data folder, is left alone.
	media := t.TempDir()
	lib := filepath.Join(media, "movies")
	got, err := Check(Folders{Data: f.Data, Others: []string{media, filepath.Dir(f.Data)}}, []string{lib})
	if err != nil || len(got) != 0 {
		t.Errorf("%v %v", got, err)
	}
	// Apply checks again: the install may have changed since it was asked for.
	if err := Ask(f.Data, Request{Mode: ModeKeepCopy, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	f.Program = f.Data
	if res := Apply(f); res == nil || res.OK || read(t, filepath.Join(f.Data, "wms.db")) != "accounts" {
		t.Errorf("%+v", res)
	}
}

func TestBadRequestsChangeNothing(t *testing.T) {
	f := install(t)
	for _, name := range []string{"..", "../../etc", "wms.db", "before-reset-20990101-000000"} {
		if err := Ask(f.Data, Request{Mode: ModeRestore, Copy: name, At: time.Now()}); err == nil {
			t.Errorf("restore from %q", name)
		}
		if err := DeleteCopy(f.Data, name); err == nil {
			t.Errorf("delete %q", name)
		}
	}
	if err := Ask(f.Data, Request{Mode: "everything", At: time.Now()}); err == nil {
		t.Error("an unknown mode")
	}
	if Pending(f.Data) != nil {
		t.Error("a refused request was kept")
	}
	// One written by hand, or by something else, is refused when carried out.
	write(t, filepath.Join(f.Data, requestFile), `{"mode":"restore","copy":"../.."}`)
	if res := Apply(f); res == nil || res.OK || read(t, filepath.Join(f.Data, "wms.db")) != "accounts" {
		t.Errorf("%+v", res)
	}
	write(t, filepath.Join(f.Data, requestFile), `not json`)
	if res := Apply(f); res != nil || read(t, filepath.Join(f.Data, "wms.db")) != "accounts" {
		t.Errorf("%+v", res)
	}
	if found, _ := Cancel(f.Data); found {
		t.Error("nothing was waiting")
	}
}

func TestCancel(t *testing.T) {
	f := install(t)
	if err := Ask(f.Data, Request{Mode: ModeDelete, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if found, err := Cancel(f.Data); !found || err != nil {
		t.Fatalf("%v %v", found, err)
	}
	if res := Apply(f); res != nil || read(t, filepath.Join(f.Data, "wms.db")) != "accounts" {
		t.Errorf("%+v", res)
	}
}

// A start cut off partway carries on with the same copy.
func TestAStartCutOffCarriesOn(t *testing.T) {
	f := install(t)
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := Ask(f.Data, Request{Mode: ModeKeepCopy, At: at}); err != nil {
		t.Fatal(err)
	}
	partway := filepath.Join(f.Data, "before-reset-20260102-030405")
	if err := os.MkdirAll(partway, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(f.Data, "games"), filepath.Join(partway, "games")); err != nil {
		t.Fatal(err)
	}
	if res := Apply(f); res == nil || !res.OK {
		t.Fatalf("%+v", res)
	}
	if read(t, filepath.Join(partway, "wms.db")) != "accounts" || read(t, filepath.Join(partway, "games", "save.srm")) != "a save" {
		t.Fatal("the copy is incomplete")
	}
}
