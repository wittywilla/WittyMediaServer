package modules

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"github.com/wittywilla/wittymodule"
	"golang.org/x/sys/windows"
)

// A running helper really has a low-integrity token and sits in a job.
func TestHelperIsSandboxedOnWindows(t *testing.T) {
	dir := t.TempDir()
	raw := manifestJSON("0.2.0", "0.4.0", "0.5.x", 1)
	man, err := wittymodule.ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, wittymodule.ManifestFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, entryName()), helperBytes(t), 0o755); err != nil {
		t.Fatal(err)
	}
	p := newProc(man, dir, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer p.Close()
	if _, err := p.Call("dimensions", pathParams{"x"}, nil); err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(p.cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	var tok windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok); err != nil {
		t.Fatal(err)
	}
	defer tok.Close()
	buf := make([]byte, 256)
	var n uint32
	if err := windows.GetTokenInformation(tok, windows.TokenIntegrityLevel, &buf[0], uint32(len(buf)), &n); err != nil {
		t.Fatal(err)
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buf[0]))
	sid := label.Label.Sid
	const lowIntegrity = 0x1000 // SECURITY_MANDATORY_LOW_RID
	if rid := sid.SubAuthority(uint32(sid.SubAuthorityCount()) - 1); rid != lowIntegrity {
		t.Errorf("integrity level %#x, want low", rid)
	}
	var inJob int32
	isInJob := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	if r, _, err := isInJob.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&inJob))); r == 0 {
		t.Fatal(err)
	}
	if inJob == 0 {
		t.Error("the helper isn't in a job")
	}
}
