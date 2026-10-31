//go:build !windows

package drives

import (
	"strings"
	"testing"
)

func TestParseMounts(t *testing.T) {
	list := parseMounts(strings.NewReader("/dev/sda1 / ext4 rw 0 0\n/dev/sr0 /media/me/MY\\040DISC udf ro 0 0\nproc /proc proc rw 0 0\n"))
	if len(list) != 1 || list[0].Path != "/media/me/MY DISC" || list[0].Name != "/dev/sr0" {
		t.Fatalf("%+v", list)
	}
}
