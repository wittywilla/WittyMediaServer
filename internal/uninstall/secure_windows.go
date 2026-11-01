//go:build windows

package uninstall

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// refuseLinks walks a folder, itself included, and says if anything in it is a link or a junction.
// Nothing in it should be, and each of those could lead an administrator's change somewhere else.
func refuseLinks(dir string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, err := os.Lstat(p)
		if err != nil {
			return err
		}
		if st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("%s is a link, which nothing here makes. Someone may have put it there to change the rules of another folder: remove it, and try again", p)
		}
		return nil
	})
}

// SecureServiceData makes the service's folder in ProgramData, where every account can read
// new folders and add to them. It holds password hashes and sessions, so only the service and
// administrators get in. Any account could have made the folder first, and would then own it
// and keep what it granted itself, so ownership comes first and nothing stays of the old rules.
func SecureServiceData(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("%s isn't a full path", dir)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	// This runs as an administrator, on a folder any account could have made first. As a link to
	// somewhere else, it would hand the service full control of wherever that leads.
	if err := refuseLinks(dir); err != nil {
		return err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	service, err := windows.CreateWellKnownSid(windows.WinLocalServiceSid)
	if err != nil {
		return err
	}
	var entries []windows.EXPLICIT_ACCESS
	for _, sid := range []*windows.SID{system, admins, service} {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		admins, nil, acl, nil); err != nil {
		return fmt.Errorf("limiting who can open %s: %w", dir, err)
	}
	// What's already inside, from an earlier install or someone else: its rules come from the
	// folder now, and it belongs to administrators.
	sys, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	icacls := filepath.Join(sys, "icacls.exe")
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		if err := refuseLinks(dir); err != nil {
			return err
		}
		if _, err := runProgram(icacls, filepath.Join(dir, "*"), "/setowner", "*S-1-5-32-544", "/T", "/C", "/Q", "/L"); err != nil {
			return fmt.Errorf("taking over what's in %s: %w", dir, err)
		}
		if _, err := runProgram(icacls, filepath.Join(dir, "*"), "/reset", "/T", "/C", "/Q", "/L"); err != nil {
			return fmt.Errorf("resetting what's in %s: %w", dir, err)
		}
	}
	return nil
}
