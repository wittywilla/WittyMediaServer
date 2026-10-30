// Command wittymodule checks, builds and signs a release of a WittyMediaServer
// plugin, from the plugin's own folder. See docs/MODULES.md, "Releasing a plugin".
//
//	wittymodule check [-server version] <folder>...      check manifests
//	wittymodule pack [-out dir] [-targets list] <folder> build every platform and write the checksum list
//	wittymodule sign [-key file] <file>                  write <file>.sig
//
// The key file defaults to WMS_SIGNING_KEY, then ~/.wms-signing/release.key, the
// same one wms-sign uses. It must never be committed or shared.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymodule"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	server := fs.String("server", "", "also require this server version to be in range")
	out := fs.String("out", "release", "folder for the release files")
	targets := fs.String("targets", "", "platforms to build, such as linux/amd64,windows/amd64 (default: all)")
	key := fs.String("key", defaultKey(), "private key file")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "check":
		err = check(fs.Args(), *server)
	case "pack":
		err = pack(fs.Args(), *out, *targets)
	case "sign":
		err = sign(*key, fs.Args())
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wittymodule:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  wittymodule check [-server version] <folder>...
  wittymodule pack [-out dir] [-targets list] <folder>
  wittymodule sign [-key file] <file>`)
}

func defaultKey() string {
	if v := os.Getenv("WMS_SIGNING_KEY"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "release.key"
	}
	return filepath.Join(h, ".wms-signing", "release.key")
}

func readManifest(dir string) (*wittymodule.Manifest, []byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, wittymodule.ManifestFile))
	if err != nil {
		return nil, nil, err
	}
	m, err := wittymodule.ParseManifest(b)
	return m, b, err
}

// check: each manifest is sound and agrees with the folder around it.
func check(dirs []string, server string) error {
	if len(dirs) == 0 {
		return errors.New("which folders? wittymodule check <folder>...")
	}
	bad := 0
	for _, dir := range dirs {
		if err := checkOne(dir, server); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", dir, err)
			bad++
			continue
		}
		fmt.Println("ok", dir)
	}
	if bad > 0 {
		return fmt.Errorf("%d of %d manifests have problems", bad, len(dirs))
	}
	return nil
}

func checkOne(dir, server string) error {
	m, _, err := readManifest(dir)
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if base := strings.ToLower(filepath.Base(abs)); base != m.ID {
		return fmt.Errorf("the manifest says %q, in a folder named %q", m.ID, base)
	}
	if m.API > wittymodule.API || m.API < wittymodule.MinAPI {
		return fmt.Errorf("api %d is outside what this WittyModule knows (%d to %d)", m.API, wittymodule.MinAPI, wittymodule.API)
	}
	if m.Delivery == wittymodule.DeliveryHelper {
		if st, err := os.Stat(filepath.Join(dir, "cmd", m.Entry)); err != nil || !st.IsDir() {
			return fmt.Errorf("the helper's source, cmd/%s, is missing", m.Entry)
		}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
		var p struct{ Version string }
		if json.Unmarshal(b, &p) == nil && p.Version != m.Version {
			return fmt.Errorf("version %s differs from package.json's %s", m.Version, p.Version)
		}
	}
	if server != "" {
		return m.Supports(server, wittymodule.MinAPI, wittymodule.API)
	}
	return nil
}

// fixedTime is every file's date in an archive, so the same code packs the same.
var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func pack(dirs []string, out, targets string) error {
	if len(dirs) != 1 {
		return errors.New("which folder? wittymodule pack <folder>")
	}
	dir := dirs[0]
	m, raw, err := readManifest(dir)
	if err != nil {
		return err
	}
	if m.Delivery != wittymodule.DeliveryHelper {
		return fmt.Errorf("%s is delivered as %q; only helpers can be packed so far", m.ID, m.Delivery)
	}
	if err := checkOne(dir, ""); err != nil {
		return err
	}
	list := wittymodule.Targets
	if targets != "" {
		list = nil
		for _, t := range strings.Split(targets, ",") {
			goos, goarch, ok := strings.Cut(strings.TrimSpace(t), "/")
			if !ok {
				return fmt.Errorf("%q is not a platform such as linux/amd64", t)
			}
			list = append(list, wittymodule.Target{GOOS: goos, GOARCH: goarch})
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	sums := map[string]string{wittymodule.ManifestName(m.ID, m.Version): hash(raw)}
	if err := os.WriteFile(filepath.Join(out, wittymodule.ManifestName(m.ID, m.Version)), raw, 0o644); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "wittymodule-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	for _, t := range list {
		fmt.Println("building", m.ID, m.Version, t.GOOS+"/"+t.GOARCH)
		exe := filepath.Join(work, t.GOOS+"-"+t.GOARCH)
		if err := build(dir, m, t, exe); err != nil {
			return err
		}
		files := map[string][]byte{}
		for _, name := range []string{wittymodule.ManifestFile, "LICENSE", "NOTICE", "README.md"} {
			if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
				files[name] = b
			} else if name != "README.md" {
				return err
			}
		}
		if files[m.EntryFile(t.GOOS)], err = os.ReadFile(exe); err != nil {
			return err
		}
		top := wittymodule.TopFolder(m.ID, m.Version, t.GOOS, t.GOARCH)
		for name, b := range files {
			sums[top+"/"+name] = hash(b)
		}
		archive := wittymodule.ArchiveName(m.ID, m.Version, t.GOOS, t.GOARCH)
		b, err := archiveBytes(t.GOOS, top, m.EntryFile(t.GOOS), files)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, archive), b, 0o644); err != nil {
			return err
		}
		sums[archive] = hash(b)
	}
	names := make([]string, 0, len(sums))
	for n := range sums {
		names = append(names, n)
	}
	sort.Strings(names)
	var lines strings.Builder
	for _, n := range names {
		fmt.Fprintf(&lines, "%s  %s\n", sums[n], n)
	}
	path := filepath.Join(out, wittymodule.SumsName(m.ID, m.Version))
	if err := os.WriteFile(path, []byte(lines.String()), 0o644); err != nil {
		return err
	}
	fmt.Println("wrote", path, "(sign it next)")
	return nil
}

func build(dir string, m *wittymodule.Manifest, t wittymodule.Target, exe string) error {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", abs, "./cmd/"+m.Entry)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.GOOS, "GOARCH="+t.GOARCH, "GOARM=7")
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building for %s/%s: %w\n%s", t.GOOS, t.GOARCH, err, b)
	}
	return nil
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func archiveBytes(goos, top, entry string, files map[string][]byte) ([]byte, error) {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	mode := func(n string) int64 {
		if n == entry {
			return 0o755
		}
		return 0o644
	}
	var buf bytes.Buffer
	if goos == "windows" {
		zw := zip.NewWriter(&buf)
		for _, n := range names {
			h := &zip.FileHeader{Name: top + "/" + n, Method: zip.Deflate, Modified: fixedTime}
			h.SetMode(os.FileMode(mode(n)))
			w, err := zw.CreateHeader(h)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(files[n]); err != nil {
				return nil, err
			}
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		if err := tw.WriteHeader(&tar.Header{Name: top + "/" + n, Typeflag: tar.TypeReg, Mode: mode(n), Size: int64(len(files[n])), ModTime: fixedTime}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sign writes the signature the server checks: base64 of Ed25519 over the file's
// exact bytes, from a key file in wms-sign's format.
func sign(keyPath string, files []string) error {
	if len(files) != 1 {
		return errors.New("which file? wittymodule sign <file>")
	}
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("no signing key: %w", err)
	}
	lines := strings.Fields(string(b))
	if len(lines) == 0 {
		return fmt.Errorf("%s: empty key file", keyPath)
	}
	seed, err := base64.StdEncoding.DecodeString(lines[len(lines)-1])
	if err != nil || len(seed) != ed25519.SeedSize {
		return fmt.Errorf("%s: not a signing key", keyPath)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)) + "\n"
	if err := os.WriteFile(files[0]+".sig", []byte(sig), 0o644); err != nil {
		return err
	}
	fmt.Println("signed", files[0]+".sig")
	return nil
}
