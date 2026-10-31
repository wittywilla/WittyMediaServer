package libretro

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/cpu"
)

// canReadVarargs: a C variadic call's integer arguments arrive where a plain call's
// would on these platforms. On Apple's arm64 they are on the stack instead, and
// the 32-bit platforms split wide ones, so the format is shown unexpanded there.
var canReadVarargs = runtime.GOARCH == "amd64" || (runtime.GOARCH == "arm64" && runtime.GOOS != "darwin")

// floatsInWords: Windows passes a variadic double in the same place as an integer.
// Elsewhere it travels in a register a callback can't see.
var floatsInWords = runtime.GOOS == "windows"

// formatLog expands a printf format from a core, because "failed to open %s" says
// little without the name.
func formatLog(format string, args [6]uintptr) string {
	if !canReadVarargs || !strings.ContainsRune(format, '%') {
		return format
	}
	var b strings.Builder
	next := 0
	arg := func() (uintptr, bool) {
		if next >= len(args) {
			return 0, false
		}
		next++
		return args[next-1], true
	}
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			b.WriteByte(format[i])
			continue
		}
		start := i
		i++
		spec := "%"
		for i < len(format) && strings.IndexByte("-+ #0", format[i]) >= 0 {
			spec += string(format[i])
			i++
		}
		for i < len(format) && (format[i] >= '0' && format[i] <= '9' || format[i] == '.' || format[i] == '*') {
			if format[i] == '*' {
				v, ok := arg()
				if !ok {
					b.WriteString(format[start:])
					return b.String()
				}
				spec += fmt.Sprint(int32(v))
			} else {
				spec += string(format[i])
			}
			i++
		}
		long := false
		for i < len(format) && strings.IndexByte("hlLzjtqI", format[i]) >= 0 {
			if format[i] != 'h' {
				long = true
			}
			i++
		}
		if i >= len(format) {
			b.WriteString(format[start:])
			break
		}
		conv := format[i]
		if conv == '%' {
			b.WriteByte('%')
			continue
		}
		if strings.IndexByte("fFeEgGaA", conv) >= 0 {
			if !floatsInWords {
				b.WriteByte('?')
				continue
			}
			v, ok := arg()
			if !ok {
				b.WriteByte('?')
				continue
			}
			fmt.Fprintf(&b, spec+string(conv), math.Float64frombits(uint64(v)))
			continue
		}
		v, ok := arg()
		if !ok {
			b.WriteString(format[start : i+1])
			continue
		}
		switch conv {
		case 's':
			if v < 4096 {
				b.WriteString("(null)")
			} else {
				fmt.Fprintf(&b, spec+"s", cString((*byte)(ptr(v))))
			}
		case 'd', 'i':
			if long {
				fmt.Fprintf(&b, spec+"d", int64(v))
			} else {
				fmt.Fprintf(&b, spec+"d", int32(v))
			}
		case 'u', 'x', 'X', 'o':
			c := conv
			if c == 'u' {
				c = 'd'
			}
			if long {
				fmt.Fprintf(&b, spec+string(c), uint64(v))
			} else {
				fmt.Fprintf(&b, spec+string(c), uint32(v))
			}
		case 'c':
			fmt.Fprintf(&b, spec+"c", rune(byte(v)))
		case 'p':
			fmt.Fprintf(&b, "%#x", v)
		default:
			b.WriteString(format[start : i+1])
		}
	}
	return b.String()
}

// ---- retro_perf_callback: cores use it for timing and to pick their SIMD paths ----

var perfStart = time.Now()

func perfTimeUsec() uintptr { return uintptr(time.Since(perfStart).Microseconds()) }

func perfCounter() uintptr { return uintptr(time.Since(perfStart).Nanoseconds()) }

func perfNothing(unsafe.Pointer) uintptr { return 0 }

// perfCPUFeatures answers with libretro's RETRO_SIMD_* bits.
func perfCPUFeatures() uintptr {
	var f uintptr
	set := func(have bool, bit uint) {
		if have {
			f |= 1 << bit
		}
	}
	set(cpu.X86.HasSSE2, 0) // anything with SSE2 has SSE
	set(cpu.X86.HasSSE2, 1)
	set(cpu.X86.HasAVX, 4)
	set(cpu.ARM.HasNEON || cpu.ARM64.HasASIMD, 5)
	set(cpu.X86.HasSSE3, 6)
	set(cpu.X86.HasSSSE3, 7)
	set(cpu.X86.HasSSE41, 10)
	set(cpu.X86.HasSSE42, 11)
	set(cpu.X86.HasAVX2, 12)
	return f
}
