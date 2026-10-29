//go:build windows

package desktop

import (
	"context"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The modern Explorer folder picker (IFileOpenDialog with FOS_PICKFOLDERS),
// called through COM directly: no cgo, no PowerShell, no extra files. The
// old tree-view "Browse for folder" dialog is never used.

type guid struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}

	ole32                   = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx      = ole32.NewProc("CoInitializeEx")
	procCoUninitialize      = ole32.NewProc("CoUninitialize")
	procCoCreateInstance    = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
)

const (
	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1
	fosPickFolders          = 0x20
	fosForceFileSystem      = 0x40
	fosPathMustExist        = 0x800
	sigdnFileSysPath        = 0x80058000
	hrCancelled             = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)
	hrChangedMode           = 0x80010106 // RPC_E_CHANGED_MODE: COM already set up on this thread
)

// Vtable slots (IUnknown 0-2, IModalWindow 3, IFileDialog 4-26).
const (
	vtRelease        = 2
	vtShow           = 3
	vtSetOptions     = 9
	vtGetOptions     = 10
	vtSetTitle       = 17
	vtGetResult      = 20
	vtGetDisplayName = 5 // IShellItem
)

// comObj is a COM interface pointer: its first word points at the vtable.
type comObj struct{ vtbl *[32]uintptr }

func call(obj *comObj, slot int, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(obj.vtbl[slot], append([]uintptr{uintptr(unsafe.Pointer(obj))}, args...)...)
	return r
}

// PickFolder shows the folder picker and returns the chosen path.
func PickFolder(ctx context.Context, title string) (string, error) {
	if headless() {
		return "", ErrUnavailable
	}
	type result struct {
		path string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		// COM dialogs need a single-threaded apartment on one OS thread.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p, err := pick(title)
		ch <- result{p, err}
	}()
	select {
	case r := <-ch:
		return r.path, r.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func pick(title string) (string, error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	if int32(hr) < 0 && hr != hrChangedMode {
		return "", ErrUnavailable
	}
	if int32(hr) >= 0 {
		defer procCoUninitialize.Call()
	}
	var dlg *comObj
	hr, _, _ = procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dlg)))
	if int32(hr) < 0 || dlg == nil {
		return "", ErrUnavailable
	}
	defer call(dlg, vtRelease)

	var opts uint32
	call(dlg, vtGetOptions, uintptr(unsafe.Pointer(&opts)))
	call(dlg, vtSetOptions, uintptr(opts|fosPickFolders|fosForceFileSystem|fosPathMustExist))
	if t, err := windows.UTF16PtrFromString(title); err == nil {
		call(dlg, vtSetTitle, uintptr(unsafe.Pointer(t)))
	}
	// Owned by the window the viewer just clicked in (the browser), so the
	// dialog opens in front of it instead of behind.
	owner, _, _ := procGetForegroundWindow.Call()
	hr = call(dlg, vtShow, owner)
	if hr == hrCancelled {
		return "", ErrCancelled
	}
	if int32(hr) < 0 {
		return "", ErrUnavailable
	}
	var item *comObj
	if hr = call(dlg, vtGetResult, uintptr(unsafe.Pointer(&item))); int32(hr) < 0 || item == nil {
		return "", ErrCancelled
	}
	defer call(item, vtRelease)
	var name *uint16
	if hr = call(item, vtGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name))); int32(hr) < 0 || name == nil {
		return "", ErrUnavailable
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(name)))
	return windows.UTF16PtrToString(name), nil
}

// headless: services run in session 0, which has no visible desktop.
func headless() bool {
	var session uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &session); err == nil && session == 0 {
		return true
	}
	return false
}
