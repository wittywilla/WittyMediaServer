package cores

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// Binary is what a file's first bytes say it is. These few fields are read by hand
// rather than with the standard library's full parsers: this file is hostile until
// proven otherwise, it is read inside the server, and nothing here needs more than a
// handful of fixed offsets.
type Binary struct {
	Format  string `json:"format"`  // pe | elf | macho
	OS      string `json:"os"`      // windows | linux | darwin; elf means any Unix but those two
	Arch    string `json:"arch"`    // amd64 | arm64 | 386 | arm | riscv64 | ...
	Library bool   `json:"library"` // a shared library rather than a program
	Matches bool   `json:"matches"` // it can load on this server
}

// Describe names it for people: "Windows, x86-64".
func (b Binary) Describe() string {
	name := map[string]string{"windows": "Windows", "linux": "Linux", "darwin": "macOS"}[b.OS]
	if name == "" {
		name = b.OS
	}
	arch := map[string]string{"amd64": "x86-64", "arm64": "ARM 64-bit", "386": "x86 32-bit", "arm": "ARM 32-bit"}[b.Arch]
	if arch == "" {
		arch = b.Arch
	}
	return name + ", " + arch
}

// ReadBinary looks at a file's headers. It says plainly when a file isn't a program
// library at all, and answers only for the file itself: Matches says whether its
// format and processor are this server's.
func ReadBinary(path string) (Binary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Binary{}, err
	}
	defer f.Close()
	return parseBinary(f, runtime.GOOS, runtime.GOARCH)
}

const notLibrary = "This isn't a program library"

func parseBinary(f io.ReaderAt, goos, goarch string) (Binary, error) {
	var head [64]byte
	n, err := f.ReadAt(head[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return Binary{}, err
	}
	h := head[:n]
	var out Binary
	switch {
	case n >= 2 && h[0] == 'M' && h[1] == 'Z':
		out, err = parsePE(f, h)
	case n >= 20 && string(h[:4]) == "\x7fELF":
		out, err = parseELF(h)
	case n >= 8 && isMachO(h):
		out, err = parseMachO(f, h, goarch)
	default:
		return Binary{}, errors.New(notLibrary + looksLike(h) + ".")
	}
	if err != nil {
		return Binary{}, err
	}
	switch out.Format {
	case "pe":
		out.Matches = goos == "windows"
	case "macho":
		out.Matches = goos == "darwin"
	case "elf":
		out.Matches = goos != "windows" && goos != "darwin"
		if out.OS == "" {
			out.OS = "linux"
		}
		if out.OS != "linux" && out.OS != goos {
			out.Matches = false
		}
	}
	out.Matches = out.Matches && out.Arch == goarch
	return out, nil
}

// looksLike is a guess that makes the error more useful: people do upload the wrong
// thing from a download page.
func looksLike(h []byte) string {
	s := strings.ToLower(strings.TrimLeft(string(h), " \t\r\n\xef\xbb\xbf"))
	switch {
	case len(h) == 0:
		return ": the file is empty"
	case strings.HasPrefix(s, "<!doctype") || strings.HasPrefix(s, "<html"):
		return ": it's a web page, so the address may be for the page about a core and not the core itself"
	case strings.HasPrefix(s, "#!"):
		return ": it's a script"
	case len(h) >= 4 && (string(h[:4]) == "PK\x03\x04"):
		return ": it's a zip archive"
	case len(h) >= 2 && h[0] == 0x1f && h[1] == 0x8b:
		return ": it's a gzip archive; unpack it first, or use a .zip"
	case len(h) >= 6 && string(h[:6]) == "7z\xbc\xaf\x27\x1c":
		return ": it's a 7-Zip archive; unpack it first, or use a .zip"
	}
	return ""
}

func parsePE(f io.ReaderAt, h []byte) (Binary, error) {
	if len(h) < 64 {
		return Binary{}, errors.New(notLibrary + ": it's too short to be one.")
	}
	off := int64(binary.LittleEndian.Uint32(h[60:]))
	var coff [24]byte
	if off <= 0 || off > 1<<24 {
		return Binary{}, errors.New(notLibrary + ": its Windows header is damaged.")
	}
	if _, err := f.ReadAt(coff[:], off); err != nil || string(coff[:4]) != "PE\x00\x00" {
		return Binary{}, errors.New(notLibrary + ": its Windows header is damaged.")
	}
	arch := map[uint16]string{0x8664: "amd64", 0xaa64: "arm64", 0x14c: "386", 0x1c4: "arm"}[binary.LittleEndian.Uint16(coff[4:])]
	if arch == "" {
		arch = fmt.Sprintf("machine %#x", binary.LittleEndian.Uint16(coff[4:]))
	}
	const fileDLL = 0x2000
	return Binary{Format: "pe", OS: "windows", Arch: arch, Library: binary.LittleEndian.Uint16(coff[22:])&fileDLL != 0}, nil
}

func parseELF(h []byte) (Binary, error) {
	var order binary.ByteOrder = binary.LittleEndian
	switch h[5] {
	case 1:
	case 2:
		order = binary.BigEndian
	default:
		return Binary{}, errors.New(notLibrary + ": its Linux header is damaged.")
	}
	const etDyn = 3
	osName := ""
	switch h[7] {
	case 9:
		osName = "freebsd"
	case 12:
		osName = "openbsd"
	case 2:
		osName = "netbsd"
	}
	machine := order.Uint16(h[18:])
	arch := map[uint16]string{62: "amd64", 183: "arm64", 3: "386", 40: "arm", 243: "riscv64", 8: "mips", 20: "ppc", 21: "ppc64", 22: "s390x"}[machine]
	if arch == "" {
		arch = fmt.Sprintf("machine %d", machine)
	}
	return Binary{Format: "elf", OS: osName, Arch: arch, Library: order.Uint16(h[16:]) == etDyn}, nil
}

func isMachO(h []byte) bool {
	m := binary.BigEndian.Uint32(h)
	return m == 0xfeedface || m == 0xfeedfacf || m == 0xcefaedfe || m == 0xcffaedfe || isFat(h)
}

// isFat tells a universal binary from a Java class, which starts with the same
// magic but with a much larger number where the architecture count goes.
func isFat(h []byte) bool {
	return binary.BigEndian.Uint32(h) == 0xcafebabe && binary.BigEndian.Uint32(h[4:]) > 0 && binary.BigEndian.Uint32(h[4:]) < 32
}

func machoArch(cpu uint32) string {
	switch cpu {
	case 0x01000007:
		return "amd64"
	case 0x0100000c:
		return "arm64"
	case 7:
		return "386"
	case 12:
		return "arm"
	}
	return fmt.Sprintf("machine %#x", cpu)
}

func parseMachO(f io.ReaderAt, h []byte, goarch string) (Binary, error) {
	const dylib, bundle = 6, 8
	if isFat(h) {
		// A universal file holds several; report the one this server would load.
		n := binary.BigEndian.Uint32(h[4:])
		out := Binary{Format: "macho", OS: "darwin"}
		for i := uint32(0); i < n; i++ {
			var e [20]byte
			if _, err := f.ReadAt(e[:], 8+int64(i)*20); err != nil {
				return Binary{}, errors.New(notLibrary + ": its macOS header is damaged.")
			}
			arch := machoArch(binary.BigEndian.Uint32(e[:]))
			if out.Arch == "" || arch == goarch {
				out.Arch = arch
				var inner [16]byte
				if _, err := f.ReadAt(inner[:], int64(binary.BigEndian.Uint32(e[8:]))); err == nil {
					ty := binary.LittleEndian.Uint32(inner[12:])
					out.Library = ty == dylib || ty == bundle
				}
			}
		}
		return out, nil
	}
	if len(h) < 16 {
		return Binary{}, errors.New(notLibrary + ": it's too short to be one.")
	}
	var order binary.ByteOrder = binary.BigEndian
	if h[0] == 0xce || h[0] == 0xcf {
		order = binary.LittleEndian
	}
	ty := order.Uint32(h[12:])
	return Binary{Format: "macho", OS: "darwin", Arch: machoArch(order.Uint32(h[4:])), Library: ty == dylib || ty == bundle}, nil
}

// Refusal says why a file can't be used on this server, or "" when it can.
func (b Binary) Refusal() string {
	if !b.Matches {
		return fmt.Sprintf("This core is built for %s, and this server is %s. Get the build for this server instead.", b.Describe(), Binary{OS: runtime.GOOS, Arch: runtime.GOARCH}.Describe())
	}
	if !b.Library {
		return "This is a program, not a library. A libretro core is a library that ends in " + libraryExt() + "."
	}
	return ""
}
