//go:build !linux

package modules

// forkLimits is what this system can really take away from a fork's helper. Only Linux
// can take a network away from one program without an administrator's help.
const forkLimits = "On this system a fork's helper keeps its network access, so it could send what it reads over the internet."
