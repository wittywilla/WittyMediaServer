package modules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/wittywilla/wittymodule"
)

// protocolCapability names what a program and this server's half of it speak to each
// other, when they are compiled apart.
const protocolCapability = "ipc"

// protocolRefusal says why a version can't work with the server's half of its
// protocol, in words for the owner; "" if it can. The built-in copy was compiled with
// the server, so its manifest says what the server speaks.
func protocolRefusal(e *entry, man *wittymodule.Manifest) string {
	want := e.builtin.Capabilities[protocolCapability]
	got := man.Capabilities[protocolCapability]
	if len(want) == 0 || slices.Equal(want, got) {
		return ""
	}
	if len(got) == 0 {
		return fmt.Sprintf("%s %s doesn't say which version of its protocol with the server it speaks, and this server needs version %s.", e.builtin.Name, man.Version, strings.Join(want, ", "))
	}
	return fmt.Sprintf("%s %s speaks version %s of its protocol with the server, and this server speaks version %s, so they couldn't work together.",
		e.builtin.Name, man.Version, strings.Join(got, ", "), strings.Join(want, ", "))
}

// ReportWorking clears a program's failures: it did its job, so they weren't in a row.
func (m *Manager) ReportWorking(id, ver string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil && e.man.Version == ver {
		e.fails = 0
	}
}
