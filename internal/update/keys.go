package update

// releaseKey is the public half of the key that signs WittyMediaServer
// releases (made with `wms-sign keygen`; the private half stays with the
// project owner, outside this repository).
const releaseKey = "VsPm/CW2lhxXrMSbWZJYwAGtT420DWVEbTB3ub47CHE="

// moduleKey is the public half of a key that signs plugins' releases and can't sign
// a server (docs/MODULES.md, "Signing"). Empty until the owner makes one with
// scripts/new-module-key.sh: until then plugins are signed with the release key.
const moduleKey = ""
