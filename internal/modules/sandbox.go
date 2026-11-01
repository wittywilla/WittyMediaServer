package modules

import "sync/atomic"

// SandboxFlag is what the server starts itself with to box a program in before it runs.
const SandboxFlag = "__module-sandbox"

// What the launcher is asked to allow and to insist on.
const (
	SandboxNetwork   = 1 << iota // it may have a network, and open devices for writing
	SandboxStrict                // a fork: if a limit can't be applied, it doesn't run
	SandboxDevices               // it may open devices for writing, as a graphics card is
	SandboxSaveArg               // it may write to the folder after its -save argument
	SandboxWriteData             // it may write to WITTYMODULE_DATA, and the folders in SandboxWriteEnv
	SandboxPrograms              // it may start other programs
)

// SandboxWriteEnv lists other folders a program may write to, as the environment.
const SandboxWriteEnv = "WITTYMODULE_WRITE"

// How well a helper is boxed in on this system.
const (
	SandboxFull       = "full"       // what docs/SECURITY-REVIEW.md promises
	SandboxPartial    = "partial"    // most of it, and the words say what's missing
	SandboxLimited    = "limited"    // some limits, which aren't a sandbox
	SandboxUnconfined = "unconfined" // nothing that would stop it doing harm
)

// SandboxInfo says, in words a person can act on, what a module's helper is held to here. Anything
// short of full is shown on the pages that add modules.
type SandboxInfo struct {
	Level string `json:"level"`
	Title string `json:"title,omitempty"`
	Words string `json:"words,omitempty"`
}

func unconfined() SandboxInfo {
	return SandboxInfo{Level: SandboxUnconfined, Title: "Modules aren't confined on this system",
		Words: "A module's helper runs with everything this server's account can do: it could change your files, start other programs, and send what it reads over the internet. Only modules from this project run here, and modules from other publishers are refused."}
}

// launcherFailed: helpers wouldn't start in the launcher here, and went on without it.
var launcherFailed atomic.Bool
