package modules

import (
	"errors"
	"strings"

	"github.com/wittywilla/wittymodule"
)

// Every way an install or a module can fail comes to a person as the same few things: what
// happened in plain words, what to try, and the exact text behind it for anyone helping. The
// server says these rather than the page, so every client says the same thing.

// Kinds of failure.
const (
	FailSignature = "signature"   // not signed with the expected key, or changed since it was
	FailKey       = "key-changed" // a store's signing key isn't the one that was agreed to
	FailNetwork   = "network"
	FailDisk      = "disk"
	FailTooOld    = "too-old"   // built for an older server than this one
	FailTooNew    = "too-new"   // needs a newer server than this one
	FailCrash     = "crash"     // its program wouldn't start, or kept stopping
	FailCorrupt   = "corrupt"   // its package is damaged
	FailMismatch  = "mismatch"  // the catalogue and the release disagree
	FailWithdrawn = "withdrawn" // its publisher took it back
	FailMissing   = "missing"   // nothing was found to install
	FailOther     = "other"
)

// Failure is one thing that went wrong, for a person.
type Failure struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Try     []string `json:"try"`
	Detail  string   `json:"detail,omitempty"` // the original words, for "Show details"
	Repair  bool     `json:"repair,omitempty"` // the repair tools may help
}

// PolicyWords is the module compatibility policy, the same everywhere it is shown.
const PolicyWords = "Modules start with " + MinModuleServer + ": one built for an older server can't run, and those older alpha versions won't be fixed or updated. A later server may keep running a module built for " + MinModuleServer + " or after, if the module says it fits. That's a limited courtesy and it's unofficial: nothing is promised until the beta, which is when proper versioning for modules begins."

// MinModuleServer is the oldest server a module may have been built for.
const MinModuleServer = wittymodule.FirstServer

// containsAny looks for any of these words, ignoring case.
func containsAny(low string, words ...string) bool {
	for _, w := range words {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// Explain turns what went wrong with a module into words for a person. name is the module's.
// An error's words are what's left of it once it has been through the steps of an install, so
// this reads those; each check here is covered by a test against the words that step uses.
func Explain(name, detail string) *Failure {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return nil
	}
	low := strings.ToLower(detail)
	f := Failure{Detail: detail}
	switch {
	case containsAny(low, "signing key has changed", "key changed", "key has changed", "isn't the one you agreed"):
		f = Failure{Kind: FailKey, Title: "The store's signing key has changed",
			Message: "Everything from this store is now signed with a different key from the one you agreed to. Someone may have taken the store over, or its owner may have changed their key. Nothing from it can be installed while this is so.",
			Try:     []string{"Ask the store's owner, somewhere other than the store itself, whether they changed their key.", "If they did, remove the store and add it again, and compare the new key's fingerprint with what they tell you.", "If you can't reach them, leave it, or remove the store. What's installed from it keeps its own signature check."}}
	case containsAny(low, "withdrawn"):
		f = Failure{Kind: FailWithdrawn, Title: "This version was taken back",
			Message: "The publisher of " + name + " withdrew this version, so it isn't offered any more.",
			Try:     []string{"Check for updates and install the newest version."}}
	case containsAny(low, "no space left", "not enough space", "disk full", "disk quota", "there is not enough space"):
		f = Failure{Kind: FailDisk, Title: "The disk is full", Repair: true,
			Message: "There wasn't enough room on the server's disk to unpack " + name + ". Nothing changed, and what was running before still is.",
			Try:     []string{"Free some space on the disk that holds the server's data folder.", "Then install it again."}}
	case containsAny(low, "disagree"):
		f = Failure{Kind: FailMismatch, Title: "The store's description doesn't match the module",
			Message: "The list of modules described this version of " + name + " differently from what its signed files say, so it wasn't installed. Either the list or the release is wrong.",
			Try:     []string{"Check for updates in case the list has been corrected, then try again.", "If it keeps happening, tell whoever publishes the module."}}
	case containsAny(low, "signature", "signed checksum", "isn't signed", "files have changed", "signed list", "doesn't belong there", "is different", "a file is missing"):
		f = Failure{Kind: FailSignature, Title: "The signature doesn't check out", Repair: true,
			Message: "The files of " + name + " aren't exactly what its publisher signed, so this server refused to use them. A damaged download can do this, and so can someone tampering with them.",
			Try:     []string{"Try it again: a damaged download usually goes away.", "If it keeps happening, tell whoever publishes the module. Don't look for another copy of it elsewhere."}}
	case containsAny(low, "unpacking", "the package has no", "differs from the release", "gzip", "unexpected eof", "archive", "not a valid zip", "invalid header", "larger than the release says"):
		f = Failure{Kind: FailCorrupt, Title: "The package is damaged", Repair: true,
			Message: "The package of " + name + " couldn't be unpacked or wasn't complete. Nothing changed, and what was running before still is.",
			Try:     []string{"Try it again: this is usually a download that went wrong.", "If it keeps happening, tell whoever publishes the module."}}
	case containsAny(low, "too old for this server", "supports wittymediaserver up to", "built for an older server"):
		f = Failure{Kind: FailTooOld, Title: "Built for an older server",
			Message: name + " was built for an older server than this one, so it can't run here. " + PolicyWords,
			Try:     []string{"Install the current version from the store, if there is one.", "If there isn't one yet, its publisher hasn't made one for this server."}}
	case containsAny(low, "needs a newer server", "or newer, and this server is", "newer server can read"):
		f = Failure{Kind: FailTooNew, Title: "Needs a newer server",
			Message: "This version of " + name + " needs a newer server than this one.",
			Try:     []string{"Update this server in Settings, About, then try again.", "Or choose an older version of the module, if one fits."}}
	case containsAny(low, "no compatible newer version", "isn't known", "no build for", "has nothing for this system", "no version that fits"):
		f = Failure{Kind: FailMissing, Title: "Nothing to install",
			Message: "No version of " + name + " that fits this server and this computer is known.",
			Try:     []string{"Check for updates, then try again.", "If the module doesn't list this kind of computer, its publisher would need to build for it."}}
	case containsAny(low, "didn't start", "stopped without answering", "kept failing", "hello", "permission denied", "exec format", "cannot execute", "access is denied"):
		f = Failure{Kind: FailCrash, Title: "The module's program wouldn't run", Repair: true,
			Message: "This server tried to run " + name + ", and it stopped or never answered. Nothing else is affected.",
			Try:     []string{"Try it again: a virus scanner holding a new program can cause this the first time.", "If the data folder is on a network share or a disk that doesn't allow programs to run, move it, or change that.", "Look at this module's log, below, for what it said."}}
	case containsAny(low, "dial ", "timeout", "timed out", "deadline exceeded", "connection", "no such host", "answered", "tls", "refusing to download", "network", "limiting requests", "eof", "not on the internet", "refused"):
		f = Failure{Kind: FailNetwork, Title: "The download didn't work",
			Message: "This server couldn't fetch " + name + ". Nothing changed, and what was running before still is.",
			Try:     []string{"Check that this server can reach the internet.", "Try it again in a little while: the place it comes from may be busy or limiting requests."}}
	default:
		f = Failure{Kind: FailOther, Title: "Something went wrong", Repair: true,
			Message: name + " couldn't be changed, for a reason this server doesn't have plainer words for. It's in the details below.",
			Try:     []string{"Try it again.", "If it keeps happening, copy the details and ask for help."}}
	}
	f.Detail = detail
	if f.Message != "" { // a name may have started the sentence
		f.Message = strings.ToUpper(f.Message[:1]) + f.Message[1:]
	}
	return &f
}

// ExplainError is Explain for an error, recognising the kinds that have a type of their own.
func ExplainError(name string, err error) *Failure { return explainErr(name, err) }

func explainErr(name string, err error) *Failure {
	if err == nil {
		return nil
	}
	var big *errStoreTooBig
	var addr *ErrStoreAddress
	switch {
	case errors.As(err, &addr):
		return &Failure{Kind: FailMissing, Title: "That address can't be used", Detail: err.Error(),
			Message: sentence(strings.ToUpper(addr.Why[:1]) + addr.Why[1:]),
			Try:     []string{"Check the address with whoever runs the store.", "If this is a store of yours on your own network, turn on \"Allow an address on my own network\"."}}
	case errors.Is(err, ErrStorePrivate):
		return &Failure{Kind: FailNetwork, Title: "That address is on your own network", Detail: err.Error(),
			Message: "A store's address normally has to be on the public internet, so that a list of modules can't be used to reach other things on your network.",
			Try:     []string{"If this really is a store of yours on your own network, turn on \"Allow an address on my own network\"."}}
	case errors.As(err, &big):
		return &Failure{Kind: FailCorrupt, Title: "That file is too big", Detail: err.Error(),
			Message: "What's at that address is much larger than a list of modules can be, so this server stopped reading it.",
			Try:     []string{"Check the address with whoever runs the store."}}
	case errors.Is(err, errStoreNotFound):
		return &Failure{Kind: FailMissing, Title: "Nothing at that address", Detail: err.Error(),
			Message: "There's no list of modules at that address.",
			Try:     []string{"Check the address. It should end in catalog.json."}}
	}
	return Explain(name, err.Error())
}
