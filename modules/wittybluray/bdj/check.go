package bdj

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

// MissingClasses reads a disc's JAR file and lists the platform classes its
// code names that the runtime does not have (class -> classes naming it), and
// how many classes the JAR holds. A disc that names a missing class fails only
// if it reaches that code, but this is the quickest way to learn what a retail
// disc needs. No class is run.
func MissingClasses(jar []byte) (map[string][]string, int, error) {
	have := map[string]bool{}
	rt, err := zip.NewReader(bytes.NewReader(runtimeJar), int64(len(runtimeJar)))
	if err != nil {
		return nil, 0, err
	}
	for _, f := range rt.File {
		have[strings.TrimSuffix(f.Name, ".class")] = true
	}
	z, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return nil, 0, err
	}
	own := map[string]bool{}
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, ".class") {
			own[strings.TrimSuffix(f.Name, ".class")] = true
		}
	}
	missing := map[string][]string{}
	n := 0
	for _, f := range z.File {
		if !strings.HasSuffix(f.Name, ".class") || f.UncompressedSize64 > 16<<20 {
			continue
		}
		r, err := f.Open()
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(r, 16<<20))
		r.Close()
		if err != nil {
			continue
		}
		names, err := classRefs(b)
		if err != nil {
			continue
		}
		n++
		for _, c := range names {
			if platformName(c) && !have[c] && !own[c] {
				missing[c] = append(missing[c], strings.TrimSuffix(f.Name, ".class"))
			}
		}
	}
	return missing, n, nil
}

func platformName(c string) bool {
	for _, p := range []string{"org/havi/", "org/dvb/", "org/davic/", "org/bluray/", "javax/tv/", "javax/media/", "javax/microedition/"} {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

// classRefs lists the classes a class file's constant pool names.
func classRefs(b []byte) ([]string, error) {
	bad := errors.New("damaged class file")
	if len(b) < 10 || binary.BigEndian.Uint32(b) != 0xCAFEBABE {
		return nil, bad
	}
	count := int(binary.BigEndian.Uint16(b[8:]))
	utf := make(map[int]string, count)
	var classes []int
	p := 10
	for i := 1; i < count; i++ {
		if p >= len(b) {
			return nil, bad
		}
		switch b[p] {
		case 1:
			if p+3 > len(b) {
				return nil, bad
			}
			l := int(binary.BigEndian.Uint16(b[p+1:]))
			if p+3+l > len(b) {
				return nil, bad
			}
			utf[i] = string(b[p+3 : p+3+l])
			p += 3 + l
		case 3, 4, 9, 10, 11, 12, 17, 18:
			p += 5
		case 5, 6:
			p += 9
			i++
		case 7:
			if p+3 > len(b) {
				return nil, bad
			}
			classes = append(classes, int(binary.BigEndian.Uint16(b[p+1:])))
			p += 3
		case 8, 16, 19, 20:
			p += 3
		case 15:
			p += 4
		default:
			return nil, bad
		}
	}
	out := make([]string, 0, len(classes))
	for _, c := range classes {
		if s := strings.TrimLeft(utf[c], "["); s != "" {
			out = append(out, strings.TrimSuffix(strings.TrimPrefix(s, "L"), ";"))
		}
	}
	return out, nil
}
