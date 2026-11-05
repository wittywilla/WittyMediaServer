//go:build windows

package appcontrol

import (
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const canVerify = true

// Where Windows keeps Smart App Control's state: 0 off, 1 on, 2 evaluation, in Microsoft's
// "Test your app with Smart App Control". Reading it needs no special rights.
const (
	policyKey   = `SYSTEM\CurrentControlSet\Control\CI\Policy`
	policyValue = "VerifiedAndReputablePolicyState"
)

func state() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, policyKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(policyValue)
	if err != nil {
		return ""
	}
	switch v {
	case 0:
		return Off
	case 1:
		return On
	case 2:
		return Evaluation
	}
	return ""
}

// verify asks Windows whether the file carries a valid Authenticode signature from a publisher
// it trusts. Revocation isn't checked, so the answer never waits on the network.
func verify(path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	data := &windows.WinTrustData{
		Size:             uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice:         windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE,
		UnionChoice:      windows.WTD_CHOICE_FILE,
		StateAction:      windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&windows.WinTrustFileInfo{
			Size:     uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})),
			FilePath: p,
		}),
	}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	return err
}

// Windows' answers when application control refuses a file: 4551, "An Application Control policy
// has blocked this file", is what starting a program blocked by Smart App Control returns. The
// reputation answers are its other ways of saying no.
var policyErrors = []windows.Errno{
	4551,       // ERROR_SYSTEM_INTEGRITY_POLICY_VIOLATION
	4553, 4554, // ERROR_SYSTEM_INTEGRITY_REPUTATION_MALICIOUS, _PUA
	4556,             // ERROR_SYSTEM_INTEGRITY_REPUTATION_OFFLINE
	4580, 4581, 4582, // ERROR_SYSTEM_INTEGRITY_REPUTATION_UNFRIENDLY_FILE, _UNATTAINABLE, _EXPLICIT_DENY_FILE
}

func policyBlock(err error) bool {
	var e windows.Errno
	if !errors.As(err, &e) {
		return false
	}
	for _, p := range policyErrors {
		if e == p {
			return true
		}
	}
	return false
}
