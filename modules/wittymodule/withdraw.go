package wittymodule

import (
	"encoding/json"
	"fmt"
)

// A release can't be taken back from everyone who has seen it, and GitHub is not
// trusted to say which ones are bad. So a plugin publishes a signed list of the
// versions it has withdrawn, as files on one more release of its repository.
const (
	// WithdrawalsTag is that release's tag. It isn't a version, so nothing mistakes
	// it for something to install.
	WithdrawalsTag = "withdrawals"
	// SeverityBroken: warn, and never install it automatically.
	SeverityBroken = "broken"
	// SeveritySecurity: refuse it, and stop using it where it is in use.
	SeveritySecurity = "security"
)

// WithdrawalsName is the list's file on that release; its signature adds .sig.
func WithdrawalsName(id string) string { return id + "-withdrawn.json" }

// Withdrawn is one version taken back, and why.
type Withdrawn struct {
	Version  string `json:"version"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
}

// Withdrawals is a plugin's list. Serial only ever goes up, and a server keeps the
// highest it has seen, so nobody can bring a withdrawn version back by replaying an
// older list. The module's name is inside what is signed, so one plugin's list
// can't be passed off as another's.
type Withdrawals struct {
	Module   string      `json:"module"`
	Serial   int         `json:"serial"`
	Versions []Withdrawn `json:"versions"`
}

// ParseWithdrawals reads and checks a list.
func ParseWithdrawals(b []byte) (*Withdrawals, error) {
	var w Withdrawals
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("unreadable list of withdrawn versions: %w", err)
	}
	if !idRe.MatchString(w.Module) || w.Serial < 1 || len(w.Versions) > 1000 {
		return nil, fmt.Errorf("the list of withdrawn versions of %q doesn't make sense", w.Module)
	}
	for _, v := range w.Versions {
		if !Valid(v.Version) || (v.Severity != SeverityBroken && v.Severity != SeveritySecurity) || len(v.Reason) > maxText {
			return nil, fmt.Errorf("the list of withdrawn versions of %s has an unreadable entry for %q", w.Module, v.Version)
		}
	}
	return &w, nil
}

// Find looks a version up.
func (w *Withdrawals) Find(version string) *Withdrawn {
	if w == nil {
		return nil
	}
	for i := range w.Versions {
		if Compare(w.Versions[i].Version, version) == 0 {
			return &w.Versions[i]
		}
	}
	return nil
}

// Withdraw adds or replaces an entry, and raises the serial.
func (w *Withdrawals) Withdraw(version, severity, reason string) error {
	if !Valid(version) {
		return fmt.Errorf("%q is not a version", version)
	}
	if severity != SeverityBroken && severity != SeveritySecurity {
		return fmt.Errorf("severity must be %s or %s", SeverityBroken, SeveritySecurity)
	}
	if reason == "" {
		return fmt.Errorf("say why %s is being withdrawn; people will read it", version)
	}
	w.Serial++
	if v := w.Find(version); v != nil {
		v.Severity, v.Reason = severity, reason
		return nil
	}
	w.Versions = append(w.Versions, Withdrawn{Version: version, Severity: severity, Reason: reason})
	return nil
}

// Restore takes a version off the list, and raises the serial.
func (w *Withdrawals) Restore(version string) error {
	for i := range w.Versions {
		if Compare(w.Versions[i].Version, version) == 0 {
			w.Versions = append(w.Versions[:i], w.Versions[i+1:]...)
			w.Serial++
			return nil
		}
	}
	return fmt.Errorf("%s is not on the list", version)
}
