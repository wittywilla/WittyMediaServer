// Command wittymodule checks, builds and signs a release of a WittyMediaServer
// plugin, from the plugin's own folder. See docs/MODULES.md, "Releasing a plugin".
//
//	wittymodule check [-server version] <folder>...      check manifests
//	wittymodule pack [-out dir] [-targets list] <folder> build every platform and write the checksum list
//	wittymodule pack [-out dir] -web <built> <folder>    pack a built web bundle and write the checksum list
//	wittymodule sign [-key file] <file>                  write <file>.sig
//	wittymodule keygen -key file                         make a signing key, for a fork; prints the public half
//	wittymodule bump [-part p] [-note text] <folder>     raise its version everywhere; a bump isn't a release
//	wittymodule needs-bump <folder>                      fail if it changed since its last tag with no new version
//	wittymodule withdraw -list file [-severity s] -reason text <version>
//	                                                     add a version to the signed list of withdrawn ones
//	wittymodule restore -list file <version>             take one off it
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
	web := fs.String("web", "", "pack this built web bundle instead of building programs")
	gcflagsFreeBSD := fs.String("freebsd-gcflags", "", "-gcflags for the FreeBSD builds, which some cgo-free libraries need")
	list := fs.String("list", "", "the list of withdrawn versions")
	module := fs.String("module", "", "the plugin the list is for, when starting a new list")
	severity := fs.String("severity", wittymodule.SeverityBroken, "broken warns; security refuses it, and stops it where it is in use")
	reason := fs.String("reason", "", "why, in words people will read")
	expect := fs.String("expect", "", "for sign: refuse a checksum list naming anything that doesn't start with this, such as wittydvd-0.2.1")
	listKey := fs.String("list-key", "", "for withdraw: the signing key; the list already there must carry its signature, or nothing is signed on top of it")
	part := fs.String("part", "patch", "for bump: patch, minor or major")
	to := fs.String("to", "", "for bump: this version exactly")
	note := fs.String("note", "", "for bump: what changed, for the changelog")
	changelog := fs.String("changelog", "", "for bump: the changelog to add the note to")
	consumer := fs.String("consumer", "", "for bump: the server's folder, to raise what it requires")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "check":
		err = check(fs.Args(), *server)
	case "pack":
		if *web != "" {
			err = packWeb(fs.Args(), *out, *web)
		} else {
			err = pack(fs.Args(), *out, *targets, *gcflagsFreeBSD)
		}
	case "sign":
		err = sign(*key, *expect, fs.Args())
	case "keygen":
		err = keygen(*key)
	case "bump":
		err = bump(*part, *to, *note, *changelog, *consumer, fs.Args())
	case "needs-bump":
		err = needsBump(fs.Args())
	case "withdraw":
		err = withdraw(*list, *module, *severity, *reason, *listKey, fs.Args())
	case "restore":
		err = restore(*list, fs.Args())
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
  wittymodule pack [-out dir] [-targets list] [-freebsd-gcflags flags] <folder>
  wittymodule pack [-out dir] -web <built folder> <folder>
  wittymodule sign [-key file] <file>
  wittymodule keygen -key file
  wittymodule bump [-part patch|minor|major] [-to version] [-note text] [-changelog file] [-consumer folder] <folder>
  wittymodule needs-bump <folder>
  wittymodule withdraw -list file [-module id] [-severity broken|security] -reason text <version>
  wittymodule restore -list file <version>`)
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
	if m.PerPlatform() {
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

func pack(dirs []string, out, targets, gcflagsFreeBSD string) error {
	if len(dirs) != 1 {
		return errors.New("which folder? wittymodule pack <folder>")
	}
	dir := dirs[0]
	m, raw, err := readManifest(dir)
	if err != nil {
		return err
	}
	if !m.PerPlatform() {
		return fmt.Errorf("%s is delivered as %q, which has no programs to build", m.ID, m.Delivery)
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
		if err := build(dir, m, t, exe, gcflagsFreeBSD); err != nil {
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
	return writeSums(out, m, sums)
}

// writeSums writes the list to sign.
func writeSums(out string, m *wittymodule.Manifest, sums map[string]string) error {
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

// webKinds are what a bundle may hold. The server serves these and nothing else, so a
// stray file fails here, where someone can do something about it.
var webKinds = map[string]bool{".js": true, ".mjs": true, ".css": true, ".wasm": true, ".json": true, ".svg": true, ".png": true, ".woff2": true, ".map": true}

// packWeb packs a bundle someone has already built (vite build, say): one archive for
// every platform. Scripts and stylesheets get a compressed copy beside them, so a small
// server sends them with no compressing work of its own.
func packWeb(dirs []string, out, built string) error {
	if len(dirs) != 1 {
		return errors.New("which folder? wittymodule pack -web <built> <folder>")
	}
	dir := dirs[0]
	m, raw, err := readManifest(dir)
	if err != nil {
		return err
	}
	if m.Delivery != wittymodule.DeliveryWeb {
		return fmt.Errorf("%s is delivered as %q, not as a web bundle", m.ID, m.Delivery)
	}
	if err := m.ValidateRelease(); err != nil {
		return err
	}
	if err := checkOne(dir, ""); err != nil {
		return err
	}
	files := map[string][]byte{wittymodule.ManifestFile: raw}
	err = filepath.WalkDir(built, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(built, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is a link or special file, which a bundle can't hold", rel)
		}
		ext := strings.ToLower(filepath.Ext(rel))
		if ext == ".gz" {
			return nil // made again below, so they always match
		}
		if !webKinds[ext] {
			return fmt.Errorf("%s: a bundle can't hold %q files", rel, ext)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files[rel] = b
		if len(b) > 1024 && (ext == ".js" || ext == ".mjs" || ext == ".css") {
			var gz bytes.Buffer
			w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
			if _, err := w.Write(b); err != nil {
				return err
			}
			if err := w.Close(); err != nil {
				return err
			}
			files[rel+".gz"] = gz.Bytes()
		}
		return nil
	})
	if err != nil {
		return err
	}
	if _, ok := files[m.Entry]; !ok {
		return fmt.Errorf("the built bundle has no %s, which the manifest names", m.Entry)
	}
	if m.Style != "" {
		if _, ok := files[m.Style]; !ok {
			return fmt.Errorf("the built bundle has no %s, which the manifest names", m.Style)
		}
	}
	for _, name := range []string{"LICENSE", "NOTICE"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		files[name] = b
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, wittymodule.ManifestName(m.ID, m.Version)), raw, 0o644); err != nil {
		return err
	}
	sums := map[string]string{wittymodule.ManifestName(m.ID, m.Version): hash(raw)}
	top := wittymodule.WebTopFolder(m.ID, m.Version)
	for name, b := range files {
		sums[top+"/"+name] = hash(b)
	}
	b, err := archiveBytes("linux", top, "", files)
	if err != nil {
		return err
	}
	archive := wittymodule.WebArchiveName(m.ID, m.Version)
	if err := os.WriteFile(filepath.Join(out, archive), b, 0o644); err != nil {
		return err
	}
	sums[archive] = hash(b)
	return writeSums(out, m, sums)
}

// withdraw adds a version to a plugin's list of withdrawn versions. Sign the list
// next, and attach both files to the release tagged "withdrawals".
func withdraw(list, module, severity, reason, listKey string, args []string) error {
	if list == "" || len(args) != 1 {
		return errors.New("wittymodule withdraw -list file [-module id] [-severity broken|security] -reason text <version>")
	}
	w := wittymodule.Withdrawals{Module: module}
	if b, err := os.ReadFile(list); err == nil {
		// Whoever can write to a release's files could have left a list here, and the next
		// signature would vouch for it.
		if listKey != "" {
			if err := checkListSignature(listKey, list, b); err != nil {
				return err
			}
		}
		parsed, err := wittymodule.ParseWithdrawals(b)
		if err != nil {
			return err
		}
		if module != "" && parsed.Module != module {
			return fmt.Errorf("%s is the list of %s", list, parsed.Module)
		}
		w = *parsed
	} else if !os.IsNotExist(err) {
		return err
	} else if module == "" {
		return errors.New("a new list needs -module")
	}
	if err := w.Withdraw(strings.TrimPrefix(args[0], "v"), severity, reason); err != nil {
		return err
	}
	return writeWithdrawals(list, &w)
}

func restore(list string, args []string) error {
	if list == "" || len(args) != 1 {
		return errors.New("wittymodule restore -list file <version>")
	}
	b, err := os.ReadFile(list)
	if err != nil {
		return err
	}
	w, err := wittymodule.ParseWithdrawals(b)
	if err != nil {
		return err
	}
	if err := w.Restore(strings.TrimPrefix(args[0], "v")); err != nil {
		return err
	}
	return writeWithdrawals(list, w)
}

// checkListSignature checks a list against the public half of the key about to sign over it.
func checkListSignature(keyPath, list string, b []byte) error {
	key, err := readKey(keyPath)
	if err != nil {
		return err
	}
	sig, err := os.ReadFile(list + ".sig")
	if err != nil {
		return fmt.Errorf("%s has no signature beside it, so it can't be trusted: %w", list, err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize || !ed25519.Verify(key.Public().(ed25519.PublicKey), b, raw) {
		return fmt.Errorf("%s isn't signed with this key, so it can't be trusted", list)
	}
	return nil
}

func writeWithdrawals(list string, w *wittymodule.Withdrawals) error {
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(list, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (serial %d, %d withdrawn; sign it next)\n", list, w.Serial, len(w.Versions))
	return nil
}

func build(dir string, m *wittymodule.Manifest, t wittymodule.Target, exe, gcflagsFreeBSD string) error {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return err
	}
	args := []string{"build", "-trimpath", "-ldflags=-s -w", "-o", abs}
	if t.GOOS == "freebsd" && gcflagsFreeBSD != "" {
		args = append(args, "-gcflags="+gcflagsFreeBSD)
	}
	cmd := exec.Command("go", append(args, "./cmd/"+m.Entry)...)
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

// readKey reads a key file in wms-sign's format.
func readKey(keyPath string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("no signing key: %w", err)
	}
	lines := strings.Fields(string(b))
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: empty key file", keyPath)
	}
	seed, err := base64.StdEncoding.DecodeString(lines[len(lines)-1])
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s: not a signing key", keyPath)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// keygen makes a signing key for someone publishing their own fork. This project's own
// key is never made here. It refuses to replace a key, since nothing signed with the old
// one could be trusted again.
func keygen(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists, and replacing a key can't be undone", path)
	}
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(priv.Seed())+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Println("the private key is in", path, "- never commit or share it")
	fmt.Println("publish this public key, which people paste into their server:")
	fmt.Println(base64.StdEncoding.EncodeToString(pub))
	return nil
}

// sign writes the signature the server checks: base64 of Ed25519 over the file's
// exact bytes, from a key file in wms-sign's format.
func sign(keyPath, expect string, files []string) error {
	if len(files) != 1 {
		return errors.New("which file? wittymodule sign <file>")
	}
	key, err := readKey(keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		return err
	}
	// A build gone wrong, or a compromised one, could offer anything here. A signature
	// only ever covers files of this module and this version.
	if expect != "" {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			_, name, _ := strings.Cut(line, " ")
			name = strings.TrimPrefix(strings.TrimSpace(name), "*")
			if !strings.HasPrefix(name, expect) || strings.Contains(name, "..") || strings.ContainsAny(name, "\\:") {
				return fmt.Errorf("%s names %q, which isn't a file of %s; nothing was signed", files[0], name, expect)
			}
		}
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, data)) + "\n"
	if err := os.WriteFile(files[0]+".sig", []byte(sig), 0o644); err != nil {
		return err
	}
	fmt.Println("signed", files[0]+".sig")
	return nil
}
