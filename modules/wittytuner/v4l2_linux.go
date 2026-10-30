package wittytuner

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"
)

// v4l2Frequency is struct v4l2_frequency: 44 bytes.
type v4l2Frequency struct {
	tuner    uint32
	typ      uint32
	freq     uint32
	reserved [8]uint32
}

// setV4L2 chooses the input and, for a tuner, the frequency in units of
// 62.5 kHz (cards that report the low-frequency unit are rare for TV).
func setV4L2(device string, input int, mhz float64) error {
	f, err := os.OpenFile(device, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	in := int32(input)
	const vidiocSInput = 0xc0045627
	if err := ioctl(f.Fd(), vidiocSInput, unsafe.Pointer(&in)); err != nil {
		return err
	}
	if mhz <= 0 {
		return nil
	}
	const (
		vidiocSFrequency = 0x402c5639
		tunerAnalogTV    = 2
	)
	fr := v4l2Frequency{typ: tunerAnalogTV, freq: uint32(mhz*16 + 0.5)}
	return ioctl(f.Fd(), vidiocSFrequency, unsafe.Pointer(&fr))
}

func discoverV4L2(context.Context, Env) []Found {
	dirs, _ := filepath.Glob("/sys/class/video4linux/video*")
	sort.Strings(dirs)
	var out []Found
	for _, d := range dirs {
		// Capture nodes only: index 0 of a device is its picture.
		if idx, err := os.ReadFile(d + "/index"); err == nil && strings.TrimSpace(string(idx)) != "0" {
			continue
		}
		name, _ := os.ReadFile(d + "/name")
		dev := "/dev/" + filepath.Base(d)
		out = append(out, Found{Driver: "v4l2", Name: firstNonEmpty(strings.TrimSpace(string(name)), dev), Detail: dev, Options: map[string]string{"device": dev}})
	}
	return out
}
