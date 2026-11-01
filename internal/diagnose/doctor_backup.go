package diagnose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/backup"
	"github.com/wittywilla/wittymediaserver/internal/metafolder"
)

// checkMetaFiles reads the folder of text files for fixing titles by hand, if someone has
// made it. A person has to fix these: the doctor only says which file and which line.
func checkMetaFiles(_ context.Context, e *DoctorEnv) []Issue {
	root := filepath.Join(e.Data, metafolder.DirName)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return nil
	}
	probs := metafolder.CheckDir(root)
	if !metafolder.HasErrors(probs) {
		return nil
	}
	metafolder.SortProblems(probs)
	var details []string
	more := 0
	for _, p := range probs {
		if p.Severity != metafolder.SevError {
			continue
		}
		if len(details) >= 50 {
			more++
			continue
		}
		details = append(details, p.String())
	}
	return []Issue{{Key: "metadata-files:problems", Severity: SevWarn, Title: "Some of your metadata files can't be used", Safety: SafetyManual, Count: len(details) + more, More: more, Details: details,
		What: "These files in the metadata folder have mistakes, so they're left out when you apply the folder. Everything else carries on as usual.",
		Fix:  "Open each file in a text editor and put right the line it names. Settings, Repair, Metadata files shows them too, and has examples.",
		Steps: []string{"Open the file named below in any text editor.", "Go to the line it says. A missing comma or quotation mark is the usual cause.", "Save it, and check again."}}}
}

// checkRestoreState looks for a restore that's waiting, stuck or cut off.
func checkRestoreState(_ context.Context, e *DoctorEnv) []Issue {
	f := backup.Folders{Data: e.Data, Images: e.Images}
	var out []Issue
	now := e.now()
	if b, err := os.Stat(filepath.Join(e.Data, "restore-request.json")); err == nil && now.Sub(b.ModTime()) > staleRequest {
		what := "A backup was checked and unpacked, and goes in at the next start, replacing what's here."
		if backup.Pending(f) == nil {
			what = "A request to restore a backup is here, and can't be read."
		}
		out = append(out, Issue{Key: "restore-state:stale", Severity: SevWarn, Title: "A restore is still waiting from " + b.ModTime().Format("2 Jan 2006 15:04"), Safety: SafetyCareful, data: "restore",
			What: what,
			Fix:  "Takes the request back and removes what was unpacked, so the next start is an ordinary one. If you do want the restore, leave this and restart the server."})
	}
	if name := backup.Interrupted(f); name != "" {
		out = append(out, Issue{Key: "restore-state:cut-off", Severity: SevProblem, Title: "A restore was cut off part way", Safety: SafetyManual,
			What:  "The server stopped while it was swapping a backup in. Everything is in its safety copy, " + name + ".",
			Fix:   "Start the server. It finds this, and puts everything back as it was before it does anything else.",
			Steps: []string{"Start the server, or run a command that does, such as wms backup cancel followed by a start.", "Check the Backup page in Settings, which says what happened."}})
	}
	if l := backup.Last(f); l != nil && !l.OK && l.Error != "" && now.Sub(l.At) < 7*24*time.Hour {
		out = append(out, Issue{Key: "restore-state:failed", Severity: SevInfo, Title: "The last restore didn't work", Safety: SafetyManual,
			What: "It stopped with: " + l.Error, Fix: "Try it again in Settings, Backup. Nothing is lost: what was there went back."})
	}
	return out
}

func fixRestoreState(_ context.Context, e *DoctorEnv, _ *Issue, _ *FixContext) (string, error) {
	found, err := backup.Cancel(backup.Folders{Data: e.Data, Images: e.Images})
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("no restore was waiting")
	}
	return "The waiting restore was taken back. Nothing else changed.", nil
}
