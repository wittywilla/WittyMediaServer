package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func doctorData(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "wms.json")
	if err := os.WriteFile(cfg, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WMS_CONFIG", cfg)
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o750); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDoctorExitCodes(t *testing.T) {
	data := doctorData(t)
	if got := cmdDoctor([]string{"-data", data, "--only", "data,metadata"}); got != doctorClean {
		t.Fatalf("a new install: exit %d", got)
	}
	if got := cmdDoctor([]string{"-data", data, "--only", "no-such-check"}); got != doctorFailed {
		t.Fatalf("an unknown check: exit %d", got)
	}
	if got := cmdDoctor([]string{"remap", "-data", data}); got != doctorFailed {
		t.Fatalf("remap with nothing to go on: exit %d", got)
	}
	// Plain `wms doctor` runs every check, not none of them.
	stray := filepath.Join(data, "reset-request.json")
	if err := os.WriteFile(stray, []byte("not a request"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cmdDoctor([]string{"-data", data, "--only", "reset-state"}); got != doctorProblems {
		t.Fatalf("a problem: exit %d", got)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("looking changed something")
	}
	if got := cmdDoctor([]string{"-data", data, "--only", "reset-state", "--fix", "--yes"}); got != doctorClean {
		t.Fatalf("after fixing: exit %d", got)
	}
	if _, err := os.Stat(stray); err == nil {
		t.Fatal("it wasn't fixed")
	}
}

func TestDoctorRunsWhenTheConfigFileIsBad(t *testing.T) {
	data := doctorData(t)
	cfg := os.Getenv("WMS_CONFIG")
	if err := os.WriteFile(cfg, []byte(`{"lisen": ":1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cmdDoctor([]string{"-data", data, "--only", "config-file"}); got != doctorProblems {
		t.Fatalf("exit %d", got)
	}
}

func TestDoctorRepairsAFileThatIsDamaged(t *testing.T) {
	data := doctorData(t)
	d, err := db.Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO settings (key, value) VALUES ('keepMe', 'yes')`); err != nil {
		t.Fatal(err)
	}
	// Enough rows to spread across pages, and a table with a page to lose.
	for n := range 3000 {
		if _, err := d.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)`, "filler-"+string(rune('a'+n%26))+string(rune('a'+(n/26)%26))+string(rune('a'+(n/676)%26)), "words words words words words words words words"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	p := filepath.Join(data, "wms.db")
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	fh, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	bad := make([]byte, 4096*3)
	for i := range bad {
		bad[i] = 0xCD
	}
	if _, err := fh.WriteAt(bad, (st.Size()/2)/4096*4096); err != nil {
		t.Fatal(err)
	}
	fh.Close()
	if got := cmdDoctor([]string{"-data", data, "--only", "data"}); got != doctorProblems {
		t.Fatalf("looking: exit %d", got)
	}
	if got := cmdDoctor([]string{"-data", data, "--only", "data", "--fix", "--yes"}); got != doctorClean {
		t.Fatalf("after fixing: exit %d", got)
	}
	d, err = db.Open(data)
	if err != nil {
		t.Fatalf("the server couldn't open the result: %v", err)
	}
	defer d.Close()
	var ok string
	if err := d.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("integrity: %s %v", ok, err)
	}
}
