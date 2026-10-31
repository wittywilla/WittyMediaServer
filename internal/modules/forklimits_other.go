//go:build !linux && !windows

package modules

// forkLimits is what this system can really take away from a fork's helper.
const forkLimits = "This system can't limit a fork's helper at all: it could change your files, start other programs, and send what it reads over the internet."
