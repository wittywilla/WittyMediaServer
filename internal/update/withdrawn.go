package update

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wittywilla/wittymodule"
)

// A version of the server can be taken back, the same way a module's can: a signed list on a release
// tagged "withdrawals". A withdrawn version is never offered, and a server running one says so.
//
// The list's serial only goes up, and the highest seen is kept, so someone who controls what GitHub
// answers can't bring a withdrawn version back by replaying an older list, or by hiding the list.

// serverModuleName is the name inside the signed list, so a module's list can't pass as the server's.
const serverModuleName = "wittymediaserver"

// WithdrawalsAsset is the list's file on that release; its signature adds .sig.
func WithdrawalsAsset() string { return wittymodule.WithdrawalsName(serverModuleName) }

const maxWithdrawalsBytes = 256 << 10

// trustedKeysFor is a seam for tests.
var trustedKeysFor = TrustedKeys

// readWithdrawals looks for a newer signed list among rels. A list that is missing or stale changes
// nothing, and a bad one is reported and changes nothing either.
func (u *Updater) readWithdrawals(ctx context.Context, rels []ghRelease) error {
	var listURL, sigURL string
	for _, g := range rels {
		if g.TagName != wittymodule.WithdrawalsTag {
			continue
		}
		for _, a := range g.Assets {
			switch a.Name {
			case WithdrawalsAsset():
				listURL = a.URL
			case WithdrawalsAsset() + ".sig":
				sigURL = a.URL
			}
		}
	}
	if listURL == "" || sigURL == "" {
		return nil
	}
	list, err := u.fetchSmall(ctx, listURL, maxWithdrawalsBytes)
	if err != nil {
		return err
	}
	sig, err := u.fetchSmall(ctx, sigURL, maxSigBytes)
	if err != nil {
		return err
	}
	return u.takeWithdrawals(list, sig)
}

func (u *Updater) takeWithdrawals(list, sig []byte) error {
	if err := VerifySums(list, sig, trustedKeysFor()); err != nil {
		return fmt.Errorf("the list of withdrawn versions was ignored: %w", err)
	}
	w, err := wittymodule.ParseWithdrawals(list)
	if err != nil {
		return err
	}
	if w.Module != serverModuleName {
		return fmt.Errorf("the list of withdrawn versions is for %s", w.Module)
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.withdrawn != nil && w.Serial <= u.withdrawn.Serial {
		return nil
	}
	u.withdrawn = w
	return u.saveWithdrawals()
}

// LoadWithdrawals reads the list kept from earlier checks. Without a place to keep it, a list is
// only remembered until the server next starts.
func (u *Updater) LoadWithdrawals(file string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.withdrawnFile = file
	b, err := os.ReadFile(file)
	if err != nil {
		return
	}
	var w wittymodule.Withdrawals
	if json.Unmarshal(b, &w) == nil && w.Module == serverModuleName {
		u.withdrawn = &w
	}
}

func (u *Updater) saveWithdrawals() error {
	if u.withdrawnFile == "" {
		return nil
	}
	b, err := json.Marshal(u.withdrawn)
	if err != nil {
		return err
	}
	tmp := u.withdrawnFile + ".new"
	if err := os.MkdirAll(filepath.Dir(tmp), 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, u.withdrawnFile)
}

// withdrawnReason says why a version was taken back, or "". Call with u.mu held.
func (u *Updater) withdrawnReason(version string) string {
	if u.withdrawn == nil {
		return ""
	}
	if v := u.withdrawn.Find(version); v != nil {
		if v.Reason != "" {
			return v.Reason
		}
		return v.Severity
	}
	return ""
}
