package modules

// SandboxFlag is what the server starts itself with to box a program in before it runs.
const SandboxFlag = "__module-sandbox"

// What the launcher is asked to allow and to insist on.
const (
	SandboxNetwork = 1 << iota // it may have a network, and open devices for writing
	SandboxStrict              // a fork: if a limit can't be applied, it doesn't run
	SandboxDevices             // it may open devices for writing, as a graphics card is
	SandboxSaveArg             // it may write to the folder after its -save argument
)
