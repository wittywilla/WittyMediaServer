package hostsandbox

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// A core that prints must not reach the protocol pipe. The test starts itself again,
// claims stdout there and then prints the way C code does.
func TestPrintingCannotReachTheProtocol(t *testing.T) {
	if os.Getenv("WITTYPLAY_STDOUT_CHILD") == "1" {
		out, err := ClaimStdout()
		if err != nil {
			fmt.Fprintln(os.Stderr, "claim:", err)
			os.Exit(1)
		}
		printFromC("noise")
		if _, err := out.WriteString("protocol"); err != nil {
			fmt.Fprintln(os.Stderr, "write:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestPrintingCannotReachTheProtocol")
	cmd.Env = append(os.Environ(), "WITTYPLAY_STDOUT_CHILD=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	if stdout.String() != "protocol" {
		t.Fatalf("the pipe received %q", stdout.String())
	}
}
