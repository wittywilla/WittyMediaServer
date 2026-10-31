//go:build windows

package drives

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalDrives      = kernel32.NewProc("GetLogicalDrives")
	procGetDriveTypeW         = kernel32.NewProc("GetDriveTypeW")
	procGetVolumeInformationW = kernel32.NewProc("GetVolumeInformationW")
	procSetErrorMode          = kernel32.NewProc("SetErrorMode")
	procDeviceIoControl       = kernel32.NewProc("DeviceIoControl")
)

const (
	driveCDROM          = 5
	semFailCriticalErrs = 0x0001
	ioctlDiskLengthInfo = 0x0007405c // IOCTL_DISK_GET_LENGTH_INFO
	ioctlDVDReadStruct  = 0x00335140 // IOCTL_DVD_READ_STRUCTURE
)

// quietly stops Windows asking "there is no disk in the drive" in a dialog.
func quietly() { procSetErrorMode.Call(semFailCriticalErrs) }

func isOptical(root string) bool {
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return false
	}
	t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(p)))
	return t == driveCDROM
}

// volumeInfo reads a volume's label and says whether it has readable media.
func volumeInfo(root string) (label string, ok bool) {
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 261)
	r, _, _ := procGetVolumeInformationW.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0, 0, 0, 0)
	if r == 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf), true
}

// List finds the optical drives, with what is in each. Nothing here spins up a
// disc beyond what Windows already read when it was inserted.
func List() ([]Drive, error) {
	quietly()
	mask, _, _ := procGetLogicalDrives.Call()
	out := []Drive{}
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		if !isOptical(root) {
			continue
		}
		d := Drive{Path: root, Name: root[:2]}
		d.Label, d.Media = volumeInfo(root)
		if d.Media {
			d.Audio = isAudioCD(root)
		}
		out = append(out, d)
	}
	return out, nil
}

// IsDrive says a path is on an optical drive.
func IsDrive(p string) bool {
	quietly()
	abs, err := filepath.Abs(p)
	if err != nil || len(abs) < 3 || abs[1] != ':' {
		return false
	}
	return isOptical(strings.ToUpper(abs[:1]) + `:\`)
}

// HasMedia says the drive holding a path has a readable disc.
func HasMedia(p string) bool {
	quietly()
	abs, err := filepath.Abs(p)
	if err != nil || len(abs) < 3 {
		return false
	}
	_, ok := volumeInfo(strings.ToUpper(abs[:1]) + `:\`)
	return ok
}

// isAudioCD: Windows shows an audio CD as a folder of .cda tracks.
func isAudioCD(root string) bool {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) == 0 {
		return false
	}
	for _, e := range entries {
		if !strings.EqualFold(filepath.Ext(e.Name()), ".cda") {
			return false
		}
	}
	return true
}

func devicePath(root string) string { return `\\.\` + strings.ToUpper(root[:1]) + ":" }

// Copyright types a DVD drive reports for its disc.
const (
	ProtectionNone    = 0
	ProtectionCSS     = 1
	ProtectionUnknown = -1
)

// Protection asks the drive whether the DVD in it is marked as protected. This
// reads the disc's own label and nothing else: it unlocks nothing, it only lets
// the player say no sooner. Anything other than a clear answer is Unknown, and
// the sample reads decide instead. (Other discs, such as Blu-rays, don't answer.)
func Protection(root string) int {
	quietly()
	if !IsDriveRoot(strings.TrimRight(root, `\`) + `\`) {
		return ProtectionUnknown
	}
	h, err := syscall.CreateFile(syscall.StringToUTF16Ptr(devicePath(root)), syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return ProtectionUnknown
	}
	defer syscall.CloseHandle(h)
	// DVD_READ_STRUCTURE: BlockByteOffset (8), SessionId (4), Format (4), LayerNumber (4).
	in := make([]byte, 24)
	binary.LittleEndian.PutUint32(in[12:], 1) // DvdCopyrightDescriptor
	// DVD_DESCRIPTOR_HEADER (4) + DVD_COPYRIGHT_DESCRIPTOR (4).
	out := make([]byte, 8)
	var got uint32
	r, _, _ := procDeviceIoControl.Call(uintptr(h), ioctlDVDReadStruct, uintptr(unsafe.Pointer(&in[0])), uintptr(len(in)), uintptr(unsafe.Pointer(&out[0])), uintptr(len(out)), uintptr(unsafe.Pointer(&got)), 0)
	if r == 0 || got < 5 {
		return ProtectionUnknown
	}
	return int(out[4])
}

// RawDisc reads a whole disc, sector by sector.
type RawDisc struct {
	f    *os.File
	size int64
}

// OpenRaw opens the disc in a drive for reading sectors.
func OpenRaw(root string) (*RawDisc, error) {
	quietly()
	h, err := syscall.CreateFile(syscall.StringToUTF16Ptr(devicePath(root)), syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return nil, fmt.Errorf("Windows didn't allow reading this drive directly: %w", err)
		}
		return nil, err
	}
	var size int64
	var got uint32
	r, _, callErr := procDeviceIoControl.Call(uintptr(h), ioctlDiskLengthInfo, 0, 0, uintptr(unsafe.Pointer(&size)), 8, uintptr(unsafe.Pointer(&got)), 0)
	if r == 0 || size <= 0 {
		syscall.CloseHandle(h)
		return nil, fmt.Errorf("the size of the disc couldn't be read: %v", callErr)
	}
	return &RawDisc{f: os.NewFile(uintptr(h), devicePath(root)), size: size}, nil
}

// Size is the disc's length in bytes.
func (d *RawDisc) Size() int64 { return d.size }

// ReadAt reads whole sectors.
func (d *RawDisc) ReadAt(p []byte, off int64) (int, error) { return d.f.ReadAt(p, off) }

// Close releases the drive.
func (d *RawDisc) Close() error { return d.f.Close() }
