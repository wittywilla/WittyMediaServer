// Command wms-sign makes and uses the key that signs WittyMediaServer
// releases. The in-app updater installs a release only when its checksum
// list (wittymediaserver-<version>-SHA256SUMS.txt) has a .sig made with
// this key. See docs/PUBLISHING.md, "Signing releases".
//
//	wms-sign keygen [-key file]        make the key (once); prints the public key
//	wms-sign pubkey [-key file]        print the public key again
//	wms-sign sign   [-key file] <file> write <file>.sig
//	wms-sign verify <file>             check <file>.sig against the keys built into wms
//
// The key file defaults to WMS_SIGNING_KEY, then ~/.wms-signing/release.key.
// It must never be committed or shared.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wittywilla/wittymediaserver/internal/update"
)

const header = "WittyMediaServer release signing key. PRIVATE: never share or commit this file; keep a backup somewhere safe.\n"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	keyPath := fs.String("key", defaultKey(), "private key file")
	_ = fs.Parse(os.Args[2:])
	var err error
	switch os.Args[1] {
	case "keygen":
		err = keygen(*keyPath)
	case "pubkey":
		var k ed25519.PrivateKey
		if k, err = load(*keyPath); err == nil {
			fmt.Println(base64.StdEncoding.EncodeToString(k.Public().(ed25519.PublicKey)))
		}
	case "sign":
		err = sign(*keyPath, fs.Args())
	case "verify":
		err = verify(fs.Args())
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wms-sign:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  wms-sign keygen [-key file]        make the release signing key (once)
  wms-sign pubkey [-key file]        print its public key
  wms-sign sign [-key file] <file>   write <file>.sig
  wms-sign verify <file>             check <file>.sig with the keys built into wms`)
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

func keygen(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists; a new key would make earlier installs refuse your updates", path)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body := header + base64.StdEncoding.EncodeToString(priv.Seed()) + "\n"
	// O_EXCL: never replace a key that appeared meanwhile.
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	fmt.Printf("Private key written to %s\n\n", path)
	fmt.Printf("Public key (put it in internal/update/keys.go as releaseKey):\n\n  %s\n\n", base64.StdEncoding.EncodeToString(pub))
	fmt.Println("Back the private key file up now (a password manager or a USB stick kept safe).")
	fmt.Println("Without it you cannot sign updates that existing installs will accept.")
	return nil
}

func load(path string) (ed25519.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no signing key at %s (make one with: wms-sign keygen)", path)
		}
		return nil, err
	}
	lines := strings.Fields(string(bytes.TrimPrefix(b, []byte(header))))
	if len(lines) == 0 {
		return nil, fmt.Errorf("%s: empty key file", path)
	}
	seed, err := base64.StdEncoding.DecodeString(lines[len(lines)-1])
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("%s: not a signing key", path)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

func sign(path string, files []string) error {
	if len(files) == 0 {
		return errors.New("which file? wms-sign sign <file>")
	}
	k, err := load(path)
	if err != nil {
		return err
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := os.WriteFile(f+".sig", update.Sign(k, b), 0o644); err != nil {
			return err
		}
		fmt.Println("signed", f+".sig")
	}
	return nil
}

func verify(files []string) error {
	if len(files) == 0 {
		return errors.New("which file? wms-sign verify <file>")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		sig, err := os.ReadFile(f + ".sig")
		if err != nil {
			return err
		}
		if err := update.VerifySums(b, sig, update.TrustedKeys()); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		fmt.Println("good signature:", f)
	}
	return nil
}
