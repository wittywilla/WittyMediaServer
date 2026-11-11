// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build windows

package desktop

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows' task dialog: real buttons with their own words, Enter for the first, Escape to
// close, and a proper title and description for screen readers. It lives in version 6 of
// the common controls, which a program only gets by asking for it in a manifest. The
// server's own program has none, so the request is made here, for the length of the call,
// with an activation context. If anything about that fails, a plain message box asks
// instead, with its standard buttons and the choices spelt out.

var (
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	procCreateActCtxW      = kernel32.NewProc("CreateActCtxW")
	procActivateActCtx     = kernel32.NewProc("ActivateActCtx")
	procDeactivateActCtx   = kernel32.NewProc("DeactivateActCtx")
	procReleaseActCtx      = kernel32.NewProc("ReleaseActCtx")
	procPostMessageW       = user32.NewProc("PostMessageW")
	taskDialogCallbackOnce sync.Once
	taskDialogCallback     uintptr
)

const commonControls6 = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <dependency><dependentAssembly>
    <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
  </dependentAssembly></dependency>
</assembly>
`

type actCtx struct {
	cbSize                 uint32
	dwFlags                uint32
	lpSource               *uint16
	wProcessorArchitecture uint16
	wLangID                uint16
	lpAssemblyDirectory    *uint16
	lpResourceName         *uint16
	lpApplicationName      *uint16
	hModule                windows.Handle
}

const (
	firstButtonID = 100
	idCancel      = 2

	tdfAllowDialogCancellation = 0x0008
	tdfPositionRelativeToWin   = 0x1000
	tdInformationIcon          = 0xFFFD // MAKEINTRESOURCEW(-3)
	tdnTimer                   = 4
	tdfCallbackTimer           = 0x0800
	tdmClickButton             = 0x0400 + 102
)

// testClick, set only by tests, presses a button once the dialog is up.
var testClick = -1

// answer is where the dialog writes the button chosen. Not on the stack: the dialog calls
// back into Go while it's open, which can grow the goroutine's stack and move it, and the
// dialog would then write to where the stack used to be.
// What the dialog's structures point at is held here for the same reason.
var answer struct {
	sync.Mutex
	button *int32
	held   []any
}

func ask(title, text string, buttons []string) (int, error) {
	runtime.LockOSThread() // an activation context belongs to the thread
	defer runtime.UnlockOSThread()
	if i, err := taskDialog(title, text, buttons); err == nil {
		return i, nil
	}
	return messageBox(title, text, buttons), nil
}

// withCommonControls6 runs fn with version 6 of the common controls loaded, and gives it
// TaskDialogIndirect from there.
func withCommonControls6(fn func(proc uintptr) error) error {
	dir, err := os.MkdirTemp("", "wms-dialog-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	manifest := filepath.Join(dir, "dialog.manifest")
	if err := os.WriteFile(manifest, []byte(commonControls6), 0o600); err != nil {
		return err
	}
	src, err := windows.UTF16PtrFromString(manifest)
	if err != nil {
		return err
	}
	ac := actCtx{lpSource: src}
	ac.cbSize = uint32(unsafe.Sizeof(ac))
	h, _, e := procCreateActCtxW.Call(uintptr(unsafe.Pointer(&ac)))
	if windows.Handle(h) == windows.InvalidHandle {
		return e
	}
	defer procReleaseActCtx.Call(h)
	var cookie uintptr
	if r, _, e := procActivateActCtx.Call(h, uintptr(unsafe.Pointer(&cookie))); r == 0 {
		return e
	}
	defer procDeactivateActCtx.Call(0, cookie)
	mod, err := windows.LoadLibrary("comctl32.dll")
	if err != nil {
		return err
	}
	proc, err := windows.GetProcAddress(mod, "TaskDialogIndirect")
	if err != nil {
		return err // version 5 after all: no task dialog
	}
	return fn(proc)
}

// packed builds the task dialog's structures, which are packed to one byte: no Go struct
// lays out the same.
type packed struct{ b []byte }

func (p *packed) u32(v uint32) { p.b = binary.LittleEndian.AppendUint32(p.b, v) }
func (p *packed) ptr(v uintptr) {
	if unsafe.Sizeof(v) == 8 {
		p.b = binary.LittleEndian.AppendUint64(p.b, uint64(v))
	} else {
		p.b = binary.LittleEndian.AppendUint32(p.b, uint32(v))
	}
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(strings.ReplaceAll(s, "\x00", ""))
	if err != nil {
		return nil
	}
	return p
}

func taskDialog(title, text string, buttons []string) (int, error) {
	taskDialogCallbackOnce.Do(func() {
		taskDialogCallback = windows.NewCallback(func(hwnd, msg, wParam, lParam, data uintptr) uintptr {
			if msg == tdnTimer && data != 0 {
				procPostMessageW.Call(hwnd, tdmClickButton, data, 0)
			}
			return 0
		})
	})
	keep := []*uint16{} // what the structures point at, held until the call returns
	str := func(s string) uintptr {
		p := utf16(s)
		keep = append(keep, p)
		return uintptr(unsafe.Pointer(p))
	}
	var list packed // TASKDIALOG_BUTTON: int id, PCWSTR text
	for i, b := range buttons {
		list.u32(uint32(firstButtonID + i))
		list.ptr(str(b))
	}
	click := uintptr(0)
	if testClick >= 0 {
		click = uintptr(firstButtonID + testClick)
	}
	var c packed // TASKDIALOGCONFIG
	c.u32(0)     // cbSize, below
	c.ptr(0)     // hwndParent
	c.ptr(0)     // hInstance
	flags := uint32(tdfAllowDialogCancellation)
	if click != 0 {
		flags |= tdfCallbackTimer
	}
	c.u32(flags)
	c.u32(0) // dwCommonButtons
	c.ptr(str("WittyMediaServer"))
	c.ptr(tdInformationIcon)
	c.ptr(str(title))
	c.ptr(str(text))
	c.u32(uint32(len(buttons)))
	c.ptr(uintptr(unsafe.Pointer(&list.b[0])))
	c.u32(firstButtonID) // nDefaultButton
	c.u32(0)             // cRadioButtons
	c.ptr(0)             // pRadioButtons
	c.u32(0)             // nDefaultRadioButton
	for range 6 {        // verification, expanded text and controls, footer icon and text
		c.ptr(0)
	}
	c.ptr(taskDialogCallback)
	c.ptr(click) // lpCallbackData
	c.u32(0)     // cxWidth: as wide as it needs
	binary.LittleEndian.PutUint32(c.b, uint32(len(c.b)))

	answer.Lock()
	defer answer.Unlock()
	if answer.button == nil {
		answer.button = new(int32)
	}
	*answer.button = 0
	answer.held = []any{keep, list.b, c.b}
	defer func() { answer.held = nil }()
	err := withCommonControls6(func(proc uintptr) error {
		hr, _, _ := syscall.SyscallN(proc, uintptr(unsafe.Pointer(&c.b[0])), uintptr(unsafe.Pointer(answer.button)), 0, 0)
		if int32(hr) < 0 {
			return windows.Errno(hr)
		}
		return nil
	})
	if err != nil {
		return -1, err
	}
	if i := int(*answer.button) - firstButtonID; i >= 0 && i < len(buttons) {
		return i, nil
	}
	return -1, nil // closed, or Escape
}

// messageBox is the fallback: standard buttons, with what each one means written out.
func messageBox(title, text string, buttons []string) int {
	flags := uint32(windows.MB_OK)
	ids := []int32{1} // IDOK
	switch len(buttons) {
	case 2:
		flags, ids = windows.MB_OKCANCEL, []int32{1, idCancel}
	case 3:
		flags, ids = windows.MB_YESNOCANCEL, []int32{6, 7, idCancel} // IDYES, IDNO, IDCANCEL
	}
	if len(buttons) > 1 {
		names := map[int]string{1: "OK", 2: "Cancel", 6: "Yes", 7: "No"}
		text += "\n"
		for i, b := range buttons {
			if i < len(ids) {
				text += "\n" + names[int(ids[i])] + ": " + strings.TrimSuffix(b, "…")
			}
		}
	}
	r, _ := windows.MessageBox(0, utf16(text), utf16(title), flags|windows.MB_ICONINFORMATION|windows.MB_SETFOREGROUND)
	for i, id := range ids {
		if r == id && i < len(buttons) {
			if id == idCancel && len(buttons) > 1 {
				return len(buttons) - 1
			}
			return i
		}
	}
	return -1
}
