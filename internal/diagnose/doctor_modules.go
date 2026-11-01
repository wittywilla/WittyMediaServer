package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/modules"
)

func moduleChecks() []Check {
	return []Check{
		{ID: "module-state", Area: AreaModules, Title: "Can the list of installed modules be read?", Run: checkModuleState, Fix: fixModuleState},
		{ID: "module-versions", Area: AreaModules, Title: "Is each installed module whole, signed and right for this server?", Run: checkModuleVersions, Fix: fixModuleVersion},
		{ID: "module-updates", Area: AreaModules, Title: "Did a module update stop partway?", Run: checkModuleWork, Fix: fixModuleWork},
		{ID: "module-helpers", Area: AreaModules, Title: "Are any module helpers still running with no server?", Run: checkHelpers, Fix: fixProcesses},
	}
}

func (e *DoctorEnv) modulesDir() string { return filepath.Join(e.Data, "modules") }

// --------------------------------------------------------------- state.json

func checkModuleState(_ context.Context, e *DoctorEnv) []Issue {
	if _, err := modules.ReadInstalledState(e.modulesDir()); err != nil {
		return []Issue{{Key: "module-state", Severity: SevWarn, Title: "The list of installed modules can't be read", Safety: SafetySafe, Exclusive: e.Modules == nil,
			What: "modules/state.json is damaged: " + err.Error() + ". Every module is using the copy built into the server until this is fixed, and your choices of version can't be remembered.",
			Fix:  "Moves the damaged file to the safety copy. Modules carry on with their built-in copies, and later updates install as usual."}}
	}
	return nil
}

func fixModuleState(_ context.Context, e *DoctorEnv, _ *Issue, f *FixContext) (string, error) {
	if err := f.Remove(filepath.Join(e.modulesDir(), "state.json")); err != nil {
		return "", err
	}
	return "The damaged list was moved to the safety copy. Modules use their built-in copies until you install an update.", nil
}

// --------------------------------------------------------------- each installed version

type badVersion struct {
	id, version string
	inUse       bool
}

func checkModuleVersions(_ context.Context, e *DoctorEnv) []Issue {
	states, err := modules.ReadInstalledState(e.modulesDir())
	if err != nil {
		return nil // the check above says so
	}
	ids := make([]string, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []Issue
	for _, id := range ids {
		st := states[id]
		seen := map[string]bool{}
		for _, ver := range []string{st.Active, st.Previous, st.Pin} {
			if ver == "" || seen[ver] {
				continue
			}
			seen[ver] = true
			p := modules.CheckInstalled(e.modulesDir(), id, ver, e.Version, st)
			if p == nil {
				continue
			}
			inUse := ver == st.Active
			sev := SevInfo
			if inUse {
				sev = SevProblem
			}
			var title, what string
			switch p.Kind {
			case modules.ProblemSignature:
				title = id + " " + ver + " fails its signature check"
				what = "Its files don't match what its publisher signed: " + p.Error() + ". The server refuses to run it."
			case modules.ProblemVersion:
				title = id + " " + ver + " isn't for this server"
				what = p.Error() + ". The server won't run it."
				if inUse {
					sev = SevWarn
				}
			default:
				title = id + " " + ver + " is incomplete"
				what = "It's listed as installed, but " + p.Error() + "."
			}
			if inUse {
				what += " Until this is fixed the server falls back to the version before it or its built-in copy."
			}
			iss := Issue{Key: "module-versions:" + id + ":" + ver, Severity: sev, Title: title, What: what, Safety: SafetySafe, Exclusive: e.Modules == nil, data: badVersion{id, ver, inUse},
				Fix: "Takes this version out of use, and moves its files to the safety copy. The module goes back to the version before it, or to its built-in copy."}
			if inUse {
				iss.Options = []FixOption{{ID: "reinstall", Label: "Reinstall it", Network: true, What: "Also downloads this module's newest compatible version from its publisher. Nothing is downloaded unless you choose this."}}
				iss.Steps = []string{"To reinstall afterwards: wms doctor reinstall " + id + ", or use the Modules page in Settings."}
			}
			out = append(out, iss)
		}
		for ver, why := range st.Blocked {
			out = append(out, Issue{Key: "module-versions:" + id + ":blocked:" + ver, Severity: SevInfo, Title: id + " " + ver + " was taken out of use", Safety: SafetyManual,
				What: "The server stopped using it, because: " + why + ". A newer version clears this when it installs."})
		}
	}
	return out
}

func fixModuleVersion(_ context.Context, e *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	b, ok := i.data.(badVersion)
	if !ok {
		return "", errors.New("run the check again first")
	}
	said := ""
	if e.Modules != nil {
		// The running server holds its choices in memory, and has to make this change itself.
		if b.inUse {
			if err := e.Modules.Rollback(b.id); err != nil {
				if err := e.Modules.UseBuiltin(b.id); err != nil {
					return "", err
				}
				said = b.id + " is using its built-in copy."
			} else {
				said = b.id + " went back to the version before."
			}
		}
	} else {
		now, err := modules.ForgetVersion(e.modulesDir(), b.id, b.version, e.Version)
		if err != nil {
			return "", err
		}
		if b.inUse {
			if now != "" {
				said = b.id + " went back to " + now + "."
			} else {
				said = b.id + " is using its built-in copy."
			}
		}
	}
	if err := f.Remove(filepath.Join(e.modulesDir(), b.id, "versions", b.version)); err != nil {
		return "", err
	}
	if said == "" {
		said = b.id + " " + b.version + " was taken out."
	}
	if f.Option == "reinstall" {
		if e.Modules == nil {
			return said + " To reinstall it: wms doctor reinstall " + b.id, nil
		}
		if err := e.Modules.Install(b.id); err != nil {
			return said + " Reinstalling it didn't work: " + err.Error() + ". The Modules page can try again.", nil
		}
		said += " Its reinstall has started: the Modules page shows how it's going."
	}
	return said, nil
}

// --------------------------------------------------------------- interrupted updates

const moduleWorkAge = time.Hour

type moduleLeftover struct{ paths []string }

func checkModuleWork(_ context.Context, e *DoctorEnv) []Issue {
	entries, err := os.ReadDir(e.modulesDir())
	if err != nil {
		return nil
	}
	states, stateErr := modules.ReadInstalledState(e.modulesDir())
	now := e.now()
	var found []string
	var names []string
	for _, d := range entries {
		p := filepath.Join(e.modulesDir(), d.Name())
		info, err := d.Info()
		if err != nil {
			continue
		}
		if !d.IsDir() {
			if strings.HasPrefix(d.Name(), ".state-") && olderThan(info, now, moduleWorkAge) {
				found = append(found, p)
				names = append(names, d.Name())
			}
			continue
		}
		subs, err := os.ReadDir(p)
		if err != nil {
			continue
		}
		for _, s := range subs {
			if !s.IsDir() || !strings.HasPrefix(s.Name(), ".work-") {
				continue
			}
			if si, err := s.Info(); err == nil && olderThan(si, now, moduleWorkAge) {
				found = append(found, filepath.Join(p, s.Name()))
				names = append(names, d.Name()+": an update that stopped partway")
			}
		}
		// A version folder nothing refers to, and no signed list in it: an install cut off
		// between unpacking and choosing it. A server removes these as it starts, but only if
		// it starts.
		if stateErr != nil {
			continue
		}
		vers, err := os.ReadDir(filepath.Join(p, "versions"))
		if err != nil {
			continue
		}
		st := states[d.Name()]
		for _, v := range vers {
			n := v.Name()
			if n == st.Active || n == st.Previous || n == st.Pin {
				continue
			}
			if vi, err := v.Info(); err == nil && olderThan(vi, now, moduleWorkAge) {
				found = append(found, filepath.Join(p, "versions", n))
				names = append(names, d.Name()+" "+n+": installed files nothing uses")
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	return []Issue{{Key: "module-updates", Severity: SevWarn, Title: "A module update stopped partway", Safety: SafetySafe, Count: len(found), Names: names, data: moduleLeftover{found},
		What: countOf(len(found), "leftover", "leftovers") + " from an install or update that didn't finish. The module itself carries on with the version it had.",
		Fix:  "Moves them to the safety copy. No version in use is touched, and the update can be tried again."}}
}

func fixModuleWork(_ context.Context, _ *DoctorEnv, i *Issue, f *FixContext) (string, error) {
	l, ok := i.data.(moduleLeftover)
	if !ok {
		return "", errors.New("run the check again first")
	}
	for _, p := range l.paths {
		if err := f.Remove(p); err != nil {
			return "", err
		}
	}
	return countOf(len(l.paths), "leftover was", "leftovers were") + " moved to the safety copy.", nil
}

// --------------------------------------------------------------- helpers with no server

type ourProcesses struct {
	list []ownedProcess
}

type ownedProcess struct {
	pid  int
	exe  string
	root string // the folder that proves it's ours
}

// helpersOf finds processes running a program out of this data folder's modules.
func helpersOf(list []procInfo, dir string) []ownedProcess {
	var out []ownedProcess
	for _, p := range list {
		if p.exe != "" && pathInside(p.exe, dir) {
			out = append(out, ownedProcess{pid: p.pid, exe: p.exe, root: dir})
		}
	}
	return out
}

func checkHelpers(ctx context.Context, e *DoctorEnv) []Issue {
	if e.ServerUp() {
		return nil // they're that server's, and it looks after them
	}
	list, err := listProcesses(ctx)
	if err != nil {
		return notChecked("module-helpers:failed", "module helpers", "The list of running programs couldn't be read: "+err.Error()+".")
	}
	found := helpersOf(list, e.modulesDir())
	if len(found) == 0 {
		return nil
	}
	var names []string
	for _, p := range found {
		names = append(names, fmt.Sprintf("process %d: %s", p.pid, filepath.Base(p.exe)))
	}
	return []Issue{{Key: "module-helpers", Severity: SevWarn, Title: "Module helpers are running with no server", Safety: SafetySafe, Count: len(found), Names: names, data: ourProcesses{found},
		What: countOf(len(found), "helper is", "helpers are") + " left from a server that stopped. They use memory, and can hold files open.",
		Fix:  "Ends them. Only programs running from this data folder's modules are touched, and each is checked again just before."}}
}
