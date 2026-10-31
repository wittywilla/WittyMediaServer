package modules

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wittywilla/wittymodule"
)

func playManifest(ver string, ipc ...string) []byte {
	m := wittymodule.Manifest{ID: "wittyplay", Name: "WittyPlay", Version: ver, Repo: "wittywilla/WittyPlay", Delivery: wittymodule.DeliveryProgram,
		Entry: "wittyplay-host", API: 2, MinServer: "0.1.0", MaxServer: "0.x"}
	if len(ipc) > 0 {
		m.Capabilities = map[string][]string{protocolCapability: ipc}
	}
	b, _ := json.Marshal(m)
	return b
}

// A runner speaking another protocol can't be put to use, and the owner is told why.
func TestProgramProtocolRefused(t *testing.T) {
	for _, c := range []struct {
		name string
		ipc  []string
		ok   bool
		says string
	}{
		{"the same", []string{"1"}, true, ""},
		{"another", []string{"2"}, false, "speaks version 2"},
		{"none named", nil, false, "doesn't say"},
	} {
		w := newWorld(t)
		m := withInstalled(t, w, playManifest("0.1.0", "1"), playManifest("0.2.0", c.ipc...), []byte("not run here"), nil)
		path, ver, ok := m.Program("wittyplay")
		st := m.Report().Modules[0]
		if ok != c.ok {
			t.Errorf("%s: %q %q %v", c.name, path, ver, ok)
		}
		if c.ok {
			if ver != "0.2.0" || st.Notice != "" {
				t.Errorf("%s: %q %+v", c.name, ver, st)
			}
		} else if st.Version != "0.1.0" || !strings.Contains(st.Notice, c.says) {
			t.Errorf("%s: %+v", c.name, st)
		}
		m.mu.Lock()
		e := m.mods["wittyplay"]
		man, _ := wittymodule.ParseManifest(playManifest("0.2.0", c.ipc...))
		refused, _ := m.judge(e, man)
		m.mu.Unlock()
		if (refused == "") != c.ok || !strings.Contains(refused, c.says) {
			t.Errorf("%s: choosing it says %q", c.name, refused)
		}
		m.Close()
	}
	// A module that names no protocol of its own has nothing to compare.
	w := newWorld(t)
	m := withInstalled(t, w, playManifest("0.1.0"), playManifest("0.2.0"), []byte("x"), nil)
	defer m.Close()
	if _, _, ok := m.Program("wittyplay"); !ok {
		t.Fatal(m.Report().Modules[0].Notice)
	}
}

// Failures only count while they are in a row.
func TestReportWorkingClearsFailures(t *testing.T) {
	w := newWorld(t)
	m := withInstalled(t, w, playManifest("0.1.0", "1"), playManifest("0.2.0", "1"), []byte("x"), nil)
	defer m.Close()
	for range failLimit * 2 {
		_ = m.ReportFailure("wittyplay", "0.2.0", "it stopped")
		_ = m.ReportFailure("wittyplay", "0.2.0", "it stopped")
		m.ReportWorking("wittyplay", "0.2.0")
	}
	if st := m.Report().Modules[0]; st.Version != "0.2.0" {
		t.Fatalf("%+v", st)
	}
}
