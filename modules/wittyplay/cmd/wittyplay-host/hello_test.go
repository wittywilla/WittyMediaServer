package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/wittywilla/wittyplay"
)

// The hello a server checks a new version by, and leaving when the pipe closes.
func TestHello(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "wittyplay-host")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if b, err := exec.Command("go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	cmd := exec.Command(exe, "--wms-helper")
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"SystemRoot=" + os.Getenv("SystemRoot")}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var hello, man struct {
		Module  string `json:"module"`
		ID      string `json:"id"`
		Version string `json:"version"`
		API     int    `json:"api"`
	}
	if err := json.Unmarshal(line, &hello); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wittyplay.ManifestJSON, &man); err != nil {
		t.Fatal(err)
	}
	if hello.Module != man.ID || hello.Version != man.Version || hello.API != man.API {
		t.Fatalf("it said %s", line)
	}
	stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("it didn't leave when the pipe closed")
	}
}
