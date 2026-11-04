package diagnose

import (
	"context"
	"path/filepath"
	"sort"

	"github.com/wittywilla/wittymediaserver/internal/appcontrol"
	"github.com/wittywilla/wittymediaserver/internal/modules"
)

// checkSmartAppControl says whether Windows' Smart App Control is on and, when it is, which of
// the installed modules' programs it blocks. It never suggests turning it off: Windows can't
// turn it on again without being reinstalled.
func checkSmartAppControl(_ context.Context, e *DoctorEnv) []Issue {
	const key = "smart-app-control"
	switch appcontrol.State() {
	case "":
		return nil
	case appcontrol.Off:
		return []Issue{{Key: key, Severity: SevInfo, Title: "Smart App Control is off", Safety: SafetyManual,
			What: "Windows doesn't require this server's programs, its modules' or game cores to be signed."}}
	case appcontrol.Evaluation:
		return []Issue{{Key: key, Severity: SevInfo, Title: "Smart App Control is in evaluation", Safety: SafetyManual,
			What: "Windows is watching which programs run, and will turn Smart App Control on or off by itself. It blocks nothing yet. Once it's on, programs and libraries that aren't signed by a publisher Windows trusts can't run, and that includes modules and game cores that aren't signed yet."}}
	}
	// Only the versions in use: one kept for going back can't be blocked from running.
	var files []string
	states, _ := modules.ReadInstalledState(e.modulesDir())
	for id, st := range states {
		if st.Active == "" {
			continue
		}
		dir := filepath.Join(e.modulesDir(), id, "versions", st.Active)
		found, err := appcontrol.Unsigned(dir)
		if err != nil {
			continue
		}
		for _, f := range found {
			files = append(files, id+" "+st.Active+": "+f)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return []Issue{{Key: key, Severity: SevInfo, Title: "Smart App Control is on", Safety: SafetyManual,
			What: "Windows runs only programs and libraries signed by a publisher it trusts, or that it knows. No installed module has one it would block."}}
	}
	names, more := files, 0
	if len(names) > 20 {
		names, more = names[:20], len(names)-20
	}
	return []Issue{{Key: key, Severity: SevProblem, Safety: SafetyManual, Count: len(files), Names: names, More: more,
		Title: plural(len(files), "module program isn't signed, so Smart App Control blocks it", "module programs aren't signed, so Smart App Control blocks them"),
		What:  "Smart App Control is on: Windows runs only programs and libraries signed by a publisher it trusts. These are in the data folder's modules folder, and the modules they belong to can't start.",
		Fix:   "Install a signed version of each module from Settings, Modules when there is one. A fork's author would need to sign theirs. Turning Smart App Control off isn't the answer: Windows can't turn it back on without being reinstalled."}}
}
