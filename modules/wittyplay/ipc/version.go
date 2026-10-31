package ipc

// Version names this protocol, as the capability "ipc" in WittyPlay's manifest.
// The server's streaming half and a downloaded runner are compiled apart, so the
// server refuses a runner whose manifest names any other. The messages themselves
// have no version to read: raise this, and the manifest with it, whenever one changes
// or a new one is added.
const Version = "1"
