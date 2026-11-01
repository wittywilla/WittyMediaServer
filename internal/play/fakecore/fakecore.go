// Package fakecore builds real shared libraries for tests of what happens when a
// core is added: one that works, and several that misbehave in the ways a hostile or
// broken file can. It is only imported by tests.
//
// The working one is the test core (package testcore) as a libretro library: the
// same game, the same .wittytest "ROM". With a C compiler it comes from fake_core.c.txt, on
// any platform. Windows machines rarely have one, so there this package writes the
// DLL byte by byte, which needs no tools and gives a real file for LoadLibrary to
// open. That one reports and loads like the others, but draws nothing.
package fakecore

import (
	_ "embed"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

//go:embed fake_core.c.txt
var source []byte

// Variant is a way for a core to behave.
type Variant string

const (
	Good           Variant = "good"
	CrashAtInit    Variant = "crash-init"    // crashes when it is started
	CrashAtOpen    Variant = "crash-open"    // crashes the moment the library opens
	HangAtInit     Variant = "hang-init"     // never comes back from starting
	HangAtOpen     Variant = "hang-open"     // never finishes opening
	MissingSymbols Variant = "missing"       // a library, but not a libretro core
	WrongAPI       Variant = "wrong-api"     // a libretro version nobody speaks
	ChattyName     Variant = "hostile-names" // reports control characters and absurd lengths
)

// ErrNoCompiler means this machine can't build a core for this test.
var ErrNoCompiler = errors.New("no C compiler here, and no hand-written core for this platform")

// Ext is this platform's library extension.
func Ext() string {
	switch runtime.GOOS {
	case "windows":
		return ".dll"
	case "darwin":
		return ".dylib"
	}
	return ".so"
}

// Build writes a core into dir, named name plus this platform's extension.
func Build(dir, name string, v Variant) (string, error) {
	out := filepath.Join(dir, name+Ext())
	if runtime.GOOS == "windows" && runtime.GOARCH == "amd64" {
		return out, os.WriteFile(out, buildPE(v), 0o755)
	}
	gcc, err := exec.LookPath("cc")
	if err != nil {
		if gcc, err = exec.LookPath("gcc"); err != nil {
			return "", ErrNoCompiler
		}
	}
	src := filepath.Join(dir, name+".c")
	if err := os.WriteFile(src, source, 0o644); err != nil {
		return "", err
	}
	defer os.Remove(src)
	args := []string{"-shared", "-fPIC", "-O1", "-o", out, src, "-DVARIANT_" + cName(v)}
	if b, err := exec.Command(gcc, args...).CombinedOutput(); err != nil {
		return "", errors.New(string(b))
	}
	return out, nil
}

func cName(v Variant) string {
	switch v {
	case CrashAtInit:
		return "CRASH_INIT"
	case CrashAtOpen:
		return "CRASH_OPEN"
	case HangAtInit:
		return "HANG_INIT"
	case HangAtOpen:
		return "HANG_OPEN"
	case MissingSymbols:
		return "MISSING"
	case WrongAPI:
		return "WRONG_API"
	case ChattyName:
		return "HOSTILE_NAMES"
	}
	return "GOOD"
}

// Foreign writes a library for a platform this server isn't, with a header real
// enough to be believed. It holds no code: the server must refuse it on its header.
func Foreign(dir, name string) (string, error) {
	out := filepath.Join(dir, name+Ext())
	var b []byte
	switch {
	case runtime.GOOS == "windows":
		b = elfHeader(62)
	case runtime.GOOS == "darwin":
		b = elfHeader(62)
	case runtime.GOARCH == "amd64":
		b = elfHeader(183) // the right format, the wrong processor
	default:
		b = elfHeader(62)
	}
	return out, os.WriteFile(out, b, 0o755)
}

func elfHeader(machine uint16) []byte {
	b := make([]byte, 64)
	copy(b, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(b[16:], 3) // a shared library
	binary.LittleEndian.PutUint16(b[18:], machine)
	return b
}

// The code of the hand-written DLL, in x86-64. Arguments arrive in rcx.
var (
	ret0   = []byte{0x31, 0xc0, 0xc3}       // xor eax,eax; ret
	ret1   = []byte{0xb8, 1, 0, 0, 0, 0xc3} // mov eax,1; ret
	ret9   = []byte{0xb8, 9, 0, 0, 0, 0xc3} // mov eax,9; ret
	ud2    = []byte{0x0f, 0x0b}             // an illegal instruction
	spin   = []byte{0xeb, 0xfe}             // jump to itself
	avInfo = []byte{
		0xc7, 0x01, 0, 1, 0, 0, // base width 256
		0xc7, 0x41, 0x04, 224, 0, 0, 0, // base height 224
		0xc7, 0x41, 0x08, 0, 1, 0, 0, // max width
		0xc7, 0x41, 0x0c, 224, 0, 0, 0, // max height
		0xc7, 0x41, 0x10, 0, 0, 0, 0, // aspect
		0x48, 0xb8, 0, 0, 0, 0, 0, 0, 0x4e, 0x40, 0x48, 0x89, 0x41, 0x18, // fps = 60.0
		0x48, 0xb8, 0, 0, 0, 0, 0x80, 0x88, 0xe5, 0x40, 0x48, 0x89, 0x41, 0x20, // rate = 44100.0
		0xc3,
	}
)

// sysInfo fills retro_system_info with pointers to three strings that sit right after
// the code, addressed relative to where the code itself is.
func sysInfo(name, version, exts string) []byte {
	const leaLen = 11
	tail := []byte{0x31, 0xc0, 0x66, 0x89, 0x41, 0x18, 0xc3} // xor eax,eax; mov [rcx+24],ax; ret
	end := int32(3*leaLen + len(tail))
	var code []byte
	lea := func(i int, target int32, field byte) {
		rel := target - int32((i+1)*leaLen-4) // from the end of this lea
		code = append(code, 0x48, 0x8d, 0x05)
		code = binary.LittleEndian.AppendUint32(code, uint32(rel))
		code = append(code, 0x48, 0x89, 0x41, field)
	}
	lea(0, end, 0)
	lea(1, end+int32(len(name))+1, 8)
	lea(2, end+int32(len(name)+len(version))+2, 16)
	code = append(code, tail...)
	code = append(code, name...)
	code = append(code, 0)
	code = append(code, version...)
	code = append(code, 0)
	code = append(code, exts...)
	return append(code, 0)
}

var allExports = []string{
	"retro_api_version", "retro_deinit", "retro_get_memory_data", "retro_get_memory_size", "retro_get_system_av_info", "retro_get_system_info",
	"retro_init", "retro_load_game", "retro_reset", "retro_run", "retro_serialize", "retro_serialize_size", "retro_set_audio_sample",
	"retro_set_audio_sample_batch", "retro_set_controller_port_device", "retro_set_environment", "retro_set_input_poll", "retro_set_input_state",
	"retro_set_video_refresh", "retro_unload_game", "retro_unserialize",
}

// buildPE writes a minimal 64-bit DLL: one section holding the code, the strings and
// the export table. It imports nothing, so it needs nothing beside it to load.
func buildPE(v Variant) []byte {
	const (
		fileAlign = 0x200
		sectRVA   = 0x1000
	)
	code := []byte{}
	place := func(b []byte) uint32 {
		for len(code)%16 != 0 {
			code = append(code, 0xcc)
		}
		at := uint32(sectRVA + len(code))
		code = append(code, b...)
		return at
	}
	name, version := "WittyPlay fake core", "1.2.3"
	if v == ChattyName {
		long := make([]byte, 3000)
		for i := range long {
			long[i] = 'A'
		}
		name = "Evil\x07\x1b[31m‮" + string(long)
	}
	sys := place(sysInfo(name, version, "wittytest|fake"))
	zero := place(ret0)
	one := place(ret1)
	av := place(avInfo)
	funcs := map[string]uint32{}
	for _, e := range allExports {
		funcs[e] = zero
	}
	funcs["retro_api_version"] = one
	funcs["retro_load_game"] = one
	funcs["retro_get_system_info"] = sys
	funcs["retro_get_system_av_info"] = av
	var entry uint32
	switch v {
	case CrashAtInit:
		funcs["retro_init"] = place(ud2)
	case HangAtInit:
		funcs["retro_init"] = place(spin)
	case CrashAtOpen:
		entry = place(ud2)
	case HangAtOpen:
		entry = place(spin)
	case WrongAPI:
		funcs["retro_api_version"] = place(ret9)
	case MissingSymbols:
		delete(funcs, "retro_run")
		delete(funcs, "retro_get_system_info")
	}
	if entry == 0 {
		entry = one // DllMain answering "yes"
	}

	names := make([]string, 0, len(funcs))
	for n := range funcs {
		names = append(names, n)
	}
	sort.Strings(names)
	for len(code)%16 != 0 {
		code = append(code, 0xcc)
	}
	dirRVA := uint32(sectRVA + len(code))
	const dirLen = 40
	n := uint32(len(names))
	funcsRVA := dirRVA + dirLen
	namesRVA := funcsRVA + 4*n
	ordsRVA := namesRVA + 4*n
	strRVA := ordsRVA + 2*n
	var exportName = []byte("fakecore.dll\x00")
	nameRVA := strRVA
	strs := append([]byte{}, exportName...)
	var fn, nm, ord []byte
	for i, s := range names {
		fn = binary.LittleEndian.AppendUint32(fn, funcs[s])
		nm = binary.LittleEndian.AppendUint32(nm, strRVA+uint32(len(strs)))
		ord = binary.LittleEndian.AppendUint16(ord, uint16(i))
		strs = append(append(strs, s...), 0)
	}
	dir := make([]byte, dirLen)
	binary.LittleEndian.PutUint32(dir[12:], nameRVA)
	binary.LittleEndian.PutUint32(dir[16:], 1) // first ordinal
	binary.LittleEndian.PutUint32(dir[20:], n)
	binary.LittleEndian.PutUint32(dir[24:], n)
	binary.LittleEndian.PutUint32(dir[28:], funcsRVA)
	binary.LittleEndian.PutUint32(dir[32:], namesRVA)
	binary.LittleEndian.PutUint32(dir[36:], ordsRVA)
	exportLen := uint32(dirLen) + uint32(len(fn)+len(nm)+len(ord)+len(strs))
	code = append(code, dir...)
	code = append(code, fn...)
	code = append(code, nm...)
	code = append(code, ord...)
	code = append(code, strs...)

	raw := (uint32(len(code)) + fileAlign - 1) &^ (fileAlign - 1)
	image := sectRVA + ((uint32(len(code)) + 0xfff) &^ 0xfff)
	out := make([]byte, fileAlign+raw)
	copy(out, "MZ")
	binary.LittleEndian.PutUint32(out[60:], 0x40)
	copy(out[0x40:], "PE\x00\x00")
	coff := out[0x44:]
	binary.LittleEndian.PutUint16(coff[0:], 0x8664)
	binary.LittleEndian.PutUint16(coff[2:], 1)
	binary.LittleEndian.PutUint16(coff[16:], 240)
	binary.LittleEndian.PutUint16(coff[18:], 0x2022) // a DLL, executable, large addresses
	opt := out[0x58:]
	binary.LittleEndian.PutUint16(opt[0:], 0x20b)
	binary.LittleEndian.PutUint32(opt[4:], raw)
	binary.LittleEndian.PutUint32(opt[16:], entry)
	binary.LittleEndian.PutUint32(opt[20:], sectRVA)
	binary.LittleEndian.PutUint64(opt[24:], 0x180000000)
	binary.LittleEndian.PutUint32(opt[32:], 0x1000)
	binary.LittleEndian.PutUint32(opt[36:], fileAlign)
	binary.LittleEndian.PutUint16(opt[40:], 6)
	binary.LittleEndian.PutUint16(opt[48:], 6)
	binary.LittleEndian.PutUint32(opt[56:], image)
	binary.LittleEndian.PutUint32(opt[60:], fileAlign)
	binary.LittleEndian.PutUint16(opt[68:], 3)
	binary.LittleEndian.PutUint64(opt[72:], 1<<20)
	binary.LittleEndian.PutUint64(opt[80:], 1<<12)
	binary.LittleEndian.PutUint64(opt[88:], 1<<20)
	binary.LittleEndian.PutUint64(opt[96:], 1<<12)
	binary.LittleEndian.PutUint32(opt[108:], 16)
	binary.LittleEndian.PutUint32(opt[112:], dirRVA)
	binary.LittleEndian.PutUint32(opt[116:], exportLen)
	sect := out[0x58+240:]
	copy(sect, ".text")
	binary.LittleEndian.PutUint32(sect[8:], uint32(len(code)))
	binary.LittleEndian.PutUint32(sect[12:], sectRVA)
	binary.LittleEndian.PutUint32(sect[16:], raw)
	binary.LittleEndian.PutUint32(sect[20:], fileAlign)
	binary.LittleEndian.PutUint32(sect[36:], 0xe0000060) // code, data, run, read, write
	copy(out[fileAlign:], code)
	return out
}
