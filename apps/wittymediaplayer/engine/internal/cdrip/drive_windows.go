//go:build windows

package cdrip

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	procDeviceIoControl = kernel32.NewProc("DeviceIoControl")
	procSetErrorMode    = kernel32.NewProc("SetErrorMode")
)

const (
	ioctlCdromReadTOC = 0x00024000 // IOCTL_CDROM_READ_TOC
	ioctlCdromRawRead = 0x0002403E // IOCTL_CDROM_RAW_READ
	trackModeCDDA     = 2          // CDDA in RAW_READ_INFO
	maxRawSectors     = 27         // what drives reliably take in one request
	errNotReady       = 21         // ERROR_NOT_READY
	errNoMedia        = 1112       // ERROR_NO_MEDIA_IN_DRIVE
	semFailCritical   = 1
)

// driveSource reads a physical audio CD by sector, as any CD player would.
type driveSource struct {
	mu   sync.Mutex
	h    syscall.Handle
	info *Info
}

func openDrive(root string) (Source, error) {
	procSetErrorMode.Call(semFailCritical)
	dev := `\\.\` + strings.ToUpper(root[:1]) + ":"
	p, err := syscall.UTF16PtrFromString(dev)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("Windows didn't allow reading this drive: %w", err)
	}
	info, err := readTOC(h)
	if err != nil {
		syscall.CloseHandle(h)
		return nil, err
	}
	info.Path = strings.ToUpper(root[:1]) + `:\`
	return &driveSource{h: h, info: info}, nil
}

// readTOC asks the drive for the table of contents: 4 bytes of header, then 8 per track.
func readTOC(h syscall.Handle) (*Info, error) {
	buf := make([]byte, 804)
	var got uint32
	r, _, e := procDeviceIoControl.Call(uintptr(h), ioctlCdromReadTOC, 0, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&got)), 0)
	if r == 0 {
		if errno, ok := e.(syscall.Errno); ok && (errno == errNotReady || errno == errNoMedia) {
			return nil, ErrDiscGone
		}
		return nil, ErrNoRead
	}
	return parseTOC(buf[:got])
}

func (d *driveSource) Info() *Info { return d.info }
func (d *driveSource) Exact() bool { return false }

func (d *driveSource) Close() error { return syscall.CloseHandle(d.h) }

// ReadSectors reads raw audio. Sectors before the disc's first or past its last can't be
// asked for, and offset correction can reach for them, so those are silence.
func (d *driveSource) ReadSectors(lba int, p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	end := 0
	for i := range d.info.Tracks {
		end = max(end, d.info.Tracks[i].End)
	}
	done := 0
	for done < len(p) {
		if lba < 0 || lba >= end {
			n := min(len(p)-done, SectorBytes)
			clear(p[done : done+n])
			done += n
			lba++
			continue
		}
		count := min((len(p)-done)/SectorBytes, maxRawSectors, end-lba)
		if count == 0 {
			break
		}
		// RAW_READ_INFO: a 2048-byte offset, a sector count and a mode.
		in := make([]byte, 16)
		binary.LittleEndian.PutUint64(in, uint64(lba)*2048)
		binary.LittleEndian.PutUint32(in[8:], uint32(count))
		binary.LittleEndian.PutUint32(in[12:], trackModeCDDA)
		var got uint32
		r, _, e := procDeviceIoControl.Call(uintptr(d.h), ioctlCdromRawRead, uintptr(unsafe.Pointer(&in[0])), uintptr(len(in)), uintptr(unsafe.Pointer(&p[done])), uintptr(count*SectorBytes), uintptr(unsafe.Pointer(&got)), 0)
		if r == 0 {
			if errno, ok := e.(syscall.Errno); ok && (errno == errNotReady || errno == errNoMedia) {
				return done, ErrDiscGone
			}
			return done, fmt.Errorf("the drive couldn't read sector %d: %v", lba, e)
		}
		if got == 0 {
			return done, fmt.Errorf("the drive gave nothing at sector %d", lba)
		}
		done += int(got)
		lba += int(got) / SectorBytes
	}
	return done, nil
}
