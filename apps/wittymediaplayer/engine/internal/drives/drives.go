// Package drives finds the computer's optical drives and what is in them.
package drives

import (
	"errors"
	"regexp"
	"strings"
)

// Drive is one optical drive.
type Drive struct {
	// Path is where the disc's files are: D:\ on Windows, a mount point elsewhere.
	Path string `json:"path"`
	// Name is what to call the drive: "D:" or /dev/sr0.
	Name  string `json:"name"`
	Label string `json:"label,omitempty"`
	// Media: a disc is in the drive and readable as files.
	Media bool `json:"media"`
	// Audio: the disc is an audio CD, which has no files to read.
	Audio bool `json:"audio,omitempty"`
}

// ErrNoRawAccess is returned where reading a whole disc isn't built yet.
var ErrNoRawAccess = errors.New("reading a whole disc isn't supported on this system yet")

var driveLetter = regexp.MustCompile(`^[A-Za-z]:[\\/]?$`)

// IsDriveRoot says a path is the root of a drive letter, like D: or D:\.
func IsDriveRoot(p string) bool { return driveLetter.MatchString(p) }

// NormalizeRoot turns D: into D:\.
func NormalizeRoot(p string) string {
	return strings.ToUpper(p[:1]) + `:\`
}
