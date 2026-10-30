package hostsandbox

// Apply does nothing on Windows: the server starts the host in a job object and
// with a low-integrity token.
func Apply(Limits) error { return nil }
