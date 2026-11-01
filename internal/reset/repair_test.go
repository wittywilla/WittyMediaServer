package reset

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRepairSwapsAndKeepsTheOldFile(t *testing.T) {
	data := t.TempDir()
	if err := os.WriteFile(filepath.Join(data, "wms.db"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "wms.db-wal"), []byte("oldwal"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := NewRepairStage(data, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, stage, "wms.db"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join("backups", "before-repair-1")
	if err := AskRepair(data, RepairRequest{Stage: stage, KeepIn: keep, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if PendingRepair(data) == nil {
		t.Fatal("nothing waiting")
	}
	r := ApplyRepair(data)
	if r == nil || !r.OK {
		t.Fatalf("result: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(data, "wms.db")); string(b) != "new" {
		t.Fatalf("db: %s", b)
	}
	if _, err := os.Stat(filepath.Join(data, "wms.db-wal")); err == nil {
		t.Fatal("the old journal would corrupt the new file")
	}
	if b, _ := os.ReadFile(filepath.Join(data, keep, "replaced", "wms.db")); string(b) != "old" {
		t.Fatalf("kept: %s", b)
	}
	if PendingRepair(data) != nil {
		t.Fatal("still waiting")
	}
	if _, err := os.Stat(filepath.Join(data, stage)); err == nil {
		t.Fatal("the stage is still there")
	}
	if ApplyRepair(data) != nil {
		t.Fatal("nothing was waiting")
	}
	if l := LastRepair(data); l == nil || !l.OK {
		t.Fatalf("last: %+v", l)
	}
}

func TestRepairRequestIsChecked(t *testing.T) {
	data := t.TempDir()
	stage, err := NewRepairStage(data, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, stage, "wms.db"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AskRepair(data, RepairRequest{Stage: "../evil", KeepIn: filepath.Join("backups", "x")}); err == nil {
		t.Fatal("a stage from anywhere was accepted")
	}
	if err := AskRepair(data, RepairRequest{Stage: stage, KeepIn: filepath.Join("..", "elsewhere")}); err == nil {
		t.Fatal("keeping the old file outside the backups was accepted")
	}
	// A request edited by hand gets the same answer at the next start, and nothing moves.
	if err := os.WriteFile(filepath.Join(data, "wms.db"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := `{"stage":"../../etc","keepIn":"backups/x"}`
	if err := os.WriteFile(filepath.Join(data, repairRequestFile), []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	r := ApplyRepair(data)
	if r == nil || r.OK {
		t.Fatalf("result: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(data, "wms.db")); string(b) != "old" {
		t.Fatalf("db: %s", b)
	}
}

func TestRepairCancel(t *testing.T) {
	data := t.TempDir()
	stage, err := NewRepairStage(data, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, stage, "wms.db"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AskRepair(data, RepairRequest{Stage: stage, KeepIn: filepath.Join("backups", "x")}); err != nil {
		t.Fatal(err)
	}
	if err := CancelRepair(data); err != nil {
		t.Fatal(err)
	}
	if PendingRepair(data) != nil {
		t.Fatal("still waiting")
	}
	if _, err := os.Stat(filepath.Join(data, stage)); err == nil {
		t.Fatal("what was built is still there")
	}
}
