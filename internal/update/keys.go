package update

// releaseKey is the public half of the key that signs WittyMediaServer
// releases (made with `go run ./scripts/sign keygen`; the private half stays with the
// project owner, outside this repository).
const releaseKey = "VsPm/CW2lhxXrMSbWZJYwAGtT420DWVEbTB3ub47CHE="

// moduleKey is the public half of a key that signs plugins' releases and can't sign
// a server (docs/MODULES.md, "Signing"). The private half stays with the project owner,
// and goes in each repository's secrets with scripts/set-module-secret.sh.
const moduleKey = "oITeMfo/k0yM4l5saoL1ypTfXxdacZTfa0kg5PNmf6s="
