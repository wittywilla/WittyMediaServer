package bdj

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"strings"
	"time"
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

// JDKClasses lists the classes a disc's JAR names from the Java library itself:
// what Refused then checks against the sandbox. No class is run.
func JDKClasses(jar []byte) ([]string, error) {
	z, err := zip.NewReader(bytes.NewReader(jar), int64(len(jar)))
	if err != nil {
		return nil, err
	}
	own := map[string]bool{}
	for _, f := range z.File {
		own[strings.TrimSuffix(f.Name, ".class")] = true
	}
	seen := map[string]bool{}
	var out []string
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
		for _, c := range names {
			if !own[c] && !seen[c] && !platformName(c) && (strings.HasPrefix(c, "java/") || strings.HasPrefix(c, "javax/") || strings.HasPrefix(c, "org/w3c/") || strings.HasPrefix(c, "org/xml/")) {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

// Refused says which of the named JDK classes the sandbox would not let a disc
// link. The list lives in the runtime, so Java answers.
func (j *Java) Refused(ctx context.Context, cacheDir string, names []string) ([]string, error) {
	jar, err := writeRuntime(cacheDir)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, j.Path, append(jvmArgs(jar), "witty.bdj.Main", ".", ".", ".", "refused")...)
	hide(cmd)
	cmd.Stdin = strings.NewReader(strings.Join(names, "\n") + "\n")
	b, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if n, ok := strings.CutPrefix(strings.TrimSpace(l), "refused "); ok {
			out = append(out, n)
		}
	}
	return out, nil
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
