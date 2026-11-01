package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

// A protected archive is an ordinary ZIP holding a manifest, a short README and one
// file, payload.wmsenc: the whole inner archive, sealed with AES-256-GCM in 1 MB chunks.
// Each chunk's nonce holds its number and whether it's the last, so chunks can't be
// reordered, dropped or cut off. The key comes from the password through Argon2id. ZIP's
// own passwords aren't used: the old kind can be broken in minutes, and the newer kinds
// don't cover file names, or work everywhere.

const (
	chunkSize = 1 << 20
	nonceLen  = 12
	prefixLen = 7

	// What a new key costs: enough to hurt guessing, and possible on a small box.
	kdfTime    = 3
	kdfMemory  = 64 << 10 // KiB
	kdfThreads = 2

	// The most an archive may ask for: its numbers are read before anything is proven.
	kdfMaxTime    = 10
	kdfMaxMemory  = 512 << 10
	kdfMaxThreads = 8

	// MinPassword is the shortest password accepted.
	MinPassword = 8
)

// CryptHeader is how a payload is protected, and isn't secret.
type CryptHeader struct {
	Alg       string `json:"alg"`
	KDF       string `json:"kdf"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memoryKiB"`
	Threads   uint8  `json:"threads"`
	Salt      string `json:"salt"`
	Prefix    string `json:"noncePrefix"`
	Chunk     int    `json:"chunk"`
}

// Key is what a password turned into, for use later without keeping the password.
// A schedule keeps one. It opens archives made with it, so it's a secret like any other.
type Key struct {
	Secret    []byte `json:"secret"`
	Salt      []byte `json:"salt"`
	Time      uint32 `json:"time"`
	MemoryKiB uint32 `json:"memoryKiB"`
	Threads   uint8  `json:"threads"`
}

// DeriveKey makes a key from a password with a new salt.
func DeriveKey(password string) (*Key, error) {
	if len(password) < MinPassword {
		return nil, fmt.Errorf("use a password of at least %d characters", MinPassword)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return &Key{Secret: argon2.IDKey([]byte(password), salt, kdfTime, kdfMemory, kdfThreads, 32), Salt: salt, Time: kdfTime, MemoryKiB: kdfMemory, Threads: kdfThreads}, nil
}

func (k *Key) header() (*CryptHeader, error) {
	prefix := make([]byte, prefixLen)
	if _, err := rand.Read(prefix); err != nil {
		return nil, err
	}
	return &CryptHeader{Alg: "AES-256-GCM", KDF: "argon2id", Time: k.Time, MemoryKiB: k.MemoryKiB, Threads: k.Threads,
		Salt: base64.StdEncoding.EncodeToString(k.Salt), Prefix: base64.StdEncoding.EncodeToString(prefix), Chunk: chunkSize}, nil
}

// checkHeader refuses a header that asks for something odd, before any work is done.
func (h *CryptHeader) check() error {
	if h.Alg != "AES-256-GCM" || h.KDF != "argon2id" || h.Chunk != chunkSize {
		return Refused(PayloadName, "it's protected in a way this version doesn't know")
	}
	if h.Time < 1 || h.Time > kdfMaxTime || h.MemoryKiB < 8<<10 || h.MemoryKiB > kdfMaxMemory || h.Threads < 1 || h.Threads > kdfMaxThreads {
		return Refused(PayloadName, "it asks for an unreasonable amount of work to open")
	}
	salt, err := base64.StdEncoding.DecodeString(h.Salt)
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return Refused(PayloadName, "its salt can't be read")
	}
	prefix, err := base64.StdEncoding.DecodeString(h.Prefix)
	if err != nil || len(prefix) != prefixLen {
		return Refused(PayloadName, "its nonce can't be read")
	}
	return nil
}

// keyFor works the key out from a password and a checked header.
func (h *CryptHeader) keyFor(password string) []byte {
	salt, _ := base64.StdEncoding.DecodeString(h.Salt)
	return argon2.IDKey([]byte(password), salt, h.Time, h.MemoryKiB, h.Threads, 32)
}

// aad ties every chunk to this header, so its numbers can't be changed afterwards.
func (h *CryptHeader) aad() []byte {
	b, _ := json.Marshal(h)
	sum := sha256.Sum256(b)
	return sum[:]
}

func (h *CryptHeader) gcm(key []byte) (cipher.AEAD, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	a, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	prefix, err := base64.StdEncoding.DecodeString(h.Prefix)
	if err != nil {
		return nil, nil, err
	}
	return a, prefix, nil
}

func chunkNonce(prefix []byte, n uint32, last bool) []byte {
	nonce := make([]byte, nonceLen)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[prefixLen:], n)
	if last {
		nonce[nonceLen-1] = 1
	}
	return nonce
}

// sealWriter encrypts as it's written. Close writes the last chunk, which is what marks
// the end, so a payload cut short can't pass for a whole one.
type sealWriter struct {
	w      io.Writer
	aead   cipher.AEAD
	prefix []byte
	aad    []byte
	buf    []byte
	n      uint32
	closed bool
}

func newSealWriter(w io.Writer, h *CryptHeader, key []byte) (*sealWriter, error) {
	a, prefix, err := h.gcm(key)
	if err != nil {
		return nil, err
	}
	return &sealWriter{w: w, aead: a, prefix: prefix, aad: h.aad(), buf: make([]byte, 0, chunkSize)}, nil
}

func (s *sealWriter) flush(last bool) error {
	if s.n == ^uint32(0) {
		return errors.New("the archive is too large to protect")
	}
	out := s.aead.Seal(nil, chunkNonce(s.prefix, s.n, last), s.buf, s.aad)
	s.buf = s.buf[:0]
	s.n++
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(out)))
	if _, err := s.w.Write(size[:]); err != nil {
		return err
	}
	_, err := s.w.Write(out)
	return err
}

func (s *sealWriter) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		if len(s.buf) == chunkSize {
			if err := s.flush(false); err != nil {
				return 0, err
			}
		}
		n := min(chunkSize-len(s.buf), len(p))
		s.buf = append(s.buf, p[:n]...)
		p = p[n:]
	}
	return total, nil
}

func (s *sealWriter) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.flush(true)
}

// errWrongPassword is what a first chunk that won't open means.
var errWrongPassword = errors.New("that password doesn't open this backup")

// openPayload decrypts r into w, at most limit bytes. It stops at the first chunk that
// doesn't prove itself: a wrong password, or anything changed, dropped or cut off.
func openPayload(w io.Writer, r io.Reader, h *CryptHeader, password string, limit int64, tick func(int64)) error {
	a, prefix, err := h.gcm(h.keyFor(password))
	if err != nil {
		return err
	}
	aad := h.aad()
	var size [4]byte
	var n uint32
	var total int64
	buf := make([]byte, 0, chunkSize+a.Overhead())
	plain := make([]byte, 0, chunkSize)
	for {
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return Refused(PayloadName, "it ends too soon, so part of it is missing")
		}
		l := binary.BigEndian.Uint32(size[:])
		if int(l) > chunkSize+a.Overhead() || int(l) < a.Overhead() {
			return Refused(PayloadName, "a part of it has an impossible size")
		}
		buf = buf[:l]
		if _, err := io.ReadFull(r, buf); err != nil {
			return Refused(PayloadName, "it ends too soon, so part of it is missing")
		}
		// Which of the two it is depends on what follows, and the seal says which.
		last := false
		out, err := a.Open(plain[:0], chunkNonce(prefix, n, false), buf, aad)
		if err != nil {
			last = true
			if out, err = a.Open(plain[:0], chunkNonce(prefix, n, true), buf, aad); err != nil {
				if n == 0 {
					return errWrongPassword
				}
				return Refused(PayloadName, "part of it has been changed, or is damaged")
			}
		}
		total += int64(len(out))
		if total > limit {
			return Refused(PayloadName, "it's larger than a backup may be")
		}
		if _, err := w.Write(out); err != nil {
			return err
		}
		if tick != nil {
			tick(int64(l) + 4)
		}
		n++
		if last {
			var extra [1]byte
			if m, _ := r.Read(extra[:]); m != 0 {
				return Refused(PayloadName, "there's something after its end")
			}
			return nil
		}
	}
}
