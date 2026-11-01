package config

import (
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A new data folder on a drive that lets every account in has to shut them out.
func TestNewDataFolderIsPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	c, err := Parse([]string{"-data", dir})
	if err != nil {
		t.Fatal(err)
	}
	if c.DataDir != dir {
		t.Fatalf("%s", c.DataDir)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	ctl, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	if ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Error("it still inherits from the folder above")
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if sid.Equals(me.User.Sid) {
			seen = true
		}
		if !sid.Equals(me.User.Sid) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			t.Errorf("open to %s", sid)
		}
	}
	if !seen {
		t.Error("the server's own account was left out")
	}
}
