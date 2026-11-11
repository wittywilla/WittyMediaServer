// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package reset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// A repair that has to replace the database can't do it while the server has the file
// open, so it works like a reset: a request is written, and the next start carries it out
// before anything opens the database.

const (
	repairRequestFile = "repair-request.json"
	repairResultFile  = "repair-result.json"
	repairStagePrefix = "repair-stage-"
)

var repairStageName = regexp.MustCompile(`^` + repairStagePrefix + `\d{8}-\d{6}$`)

// RepairRequest is a new database waiting to replace the one in use.
type RepairRequest struct {
	Stage  string    `json:"stage"`  // the folder in the data folder holding the new wms.db
	KeepIn string    `json:"keepIn"` // the folder in the data folder the old files move to
	At     time.Time `json:"at"`
}

// RepairResult says how the last one went.
type RepairResult struct {
	OK    bool      `json:"ok"`
	Kept  string    `json:"kept,omitempty"`
	Error string    `json:"error,omitempty"`
	At    time.Time `json:"at"`
}

// NewRepairStage makes the folder a new database is built in.
func NewRepairStage(data string, at time.Time) (name string, err error) {
	name = repairStagePrefix + at.UTC().Format(stampFormat)
	return name, os.Mkdir(filepath.Join(data, name), 0o750)
}

// AskRepair writes the request for the next start.
func AskRepair(data string, r RepairRequest) error {
	if !repairStageName.MatchString(r.Stage) {
		return errors.New("no such repair")
	}
	if !within(filepath.Join(data, "backups"), filepath.Join(data, r.KeepIn)) {
		return errors.New("the old database has to be kept in the backups folder")
	}
	if _, err := os.Stat(filepath.Join(data, r.Stage, "wms.db")); err != nil {
		return errors.New("the new database isn't there")
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(data, repairRequestFile+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(data, repairRequestFile))
}

// PendingRepair is the request waiting, or nil.
func PendingRepair(data string) *RepairRequest {
	b, err := os.ReadFile(filepath.Join(data, repairRequestFile))
	if err != nil {
		return nil
	}
	var r RepairRequest
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// CancelRepair takes back a waiting repair, and removes what was built for it.
func CancelRepair(data string) error {
	if r := PendingRepair(data); r != nil && repairStageName.MatchString(r.Stage) {
		_ = os.RemoveAll(filepath.Join(data, r.Stage))
	}
	err := os.Remove(filepath.Join(data, repairRequestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// LastRepair says how the last one went, or nil.
func LastRepair(data string) *RepairResult {
	b, err := os.ReadFile(filepath.Join(data, repairResultFile))
	if err != nil {
		return nil
	}
	var r RepairResult
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

// ApplyRepair carries out a waiting repair, if there is one, and returns nil if there
// wasn't. Nothing else may have the database open. If anything can't move, every file goes
// back where it was and the old database stays in use.
func ApplyRepair(data string) *RepairResult {
	r := PendingRepair(data)
	if r == nil {
		if _, err := os.Stat(filepath.Join(data, repairRequestFile)); err == nil {
			_ = os.Remove(filepath.Join(data, repairRequestFile)) // not one this version can read
		}
		return nil
	}
	res := &RepairResult{At: time.Now().UTC()}
	if err := swapDatabase(data, r); err != nil {
		res.Error = err.Error()
	} else {
		res.OK = true
		res.Kept = r.KeepIn
		_ = os.RemoveAll(filepath.Join(data, r.Stage))
	}
	_ = os.Remove(filepath.Join(data, repairRequestFile))
	if b, err := json.MarshalIndent(res, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(data, repairResultFile), b, 0o600)
	}
	return res
}

func swapDatabase(data string, r *RepairRequest) error {
	if !repairStageName.MatchString(r.Stage) || !within(filepath.Join(data, "backups"), filepath.Join(data, r.KeepIn)) {
		return errors.New("that repair request doesn't look right, so it was ignored")
	}
	stage := filepath.Join(data, r.Stage)
	if _, err := os.Stat(filepath.Join(stage, "wms.db")); err != nil {
		return errors.New("the new database isn't there any more")
	}
	keep := filepath.Join(data, r.KeepIn, "replaced")
	if err := os.MkdirAll(keep, 0o750); err != nil {
		return err
	}
	var moved []string
	undo := func() {
		for _, name := range moved {
			_ = os.Rename(filepath.Join(keep, name), filepath.Join(data, name))
		}
	}
	for _, name := range []string{"wms.db", "wms.db-wal", "wms.db-shm"} {
		if err := rename(filepath.Join(data, name), filepath.Join(keep, name)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			undo()
			return fmt.Errorf("%s couldn't be moved: %w", name, err)
		}
		moved = append(moved, name)
	}
	if err := rename(filepath.Join(stage, "wms.db"), filepath.Join(data, "wms.db")); err != nil {
		undo()
		return fmt.Errorf("the new database couldn't be moved into place: %w", err)
	}
	return nil
}
