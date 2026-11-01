package cores

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wittywilla/wittymodule/service"
)

// Written by hand: what holds functions or clients, or takes a stream.

const module = "wittyplay"

// Manager looks in one folder it owns, plus folders other programs own. Each call says where.
type Manager struct {
	Dir    string // installs go here; an administrator may also place files here
	System string // a core's support files go here, in the folder cores are told is theirs
	Client *http.Client
	Extra  []string // folders to read cores from; DefaultExtra if nil
	// Custom lists the approved custom cores, when they are switched on.
	Custom func() []Core
}

// MarshalJSON sends what the module needs to act on these folders, with the list as it is now.
func (m *Manager) MarshalJSON() ([]byte, error) {
	var custom []Core
	if m.Custom != nil {
		custom = m.Custom()
	}
	return service.Marshal(struct {
		Dir    string
		System string
		Extra  []string
		Custom []Core
	}{m.Dir, m.System, m.Extra, custom})
}

// CustomStore keeps custom cores. Each call says where.
type CustomStore struct {
	Dir    string // <data>/gaming/custom-cores
	System string // the BIOS folder a package's system/ files go to
	Placed []string
	Limits Limits
	// Client fetches addresses. The module has its own, which refuses private addresses.
	Client *http.Client `json:"-"`
}

// toFile keeps a stream in a file beside the store, where a helper can read it: a call carries
// settings, not uploads.
func (st *CustomStore) toFile(r io.Reader) (string, func(), error) {
	if err := os.MkdirAll(st.Dir, 0o755); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(st.Dir, ".upload-*")
	if err != nil {
		return "", nil, err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	abs, err := filepath.Abs(f.Name())
	if err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	return abs, func() { _ = os.Remove(abs) }, nil
}

// StageReader checks an uploaded package and keeps it until someone decides.
func (st *CustomStore) StageReader(r io.Reader, name string) (*Staged, error) {
	path, done, err := st.toFile(r)
	if err != nil {
		return nil, err
	}
	defer done()
	var out *Staged
	err = service.Call(context.Background(), module, "cores.CustomStore.StageFile", st, nil, []any{path, name}, []any{&out})
	return out, err
}

// AddExtras adds a package's support files to a core that's already approved.
func (st *CustomStore) AddExtras(r io.Reader, name, id, sum string) (added, kept []string, err error) {
	path, done, err := st.toFile(r)
	if err != nil {
		return nil, nil, err
	}
	defer done()
	err = service.Call(context.Background(), module, "cores.CustomStore.AddExtrasFile", st, nil, []any{path, name, id, sum}, []any{&added, &kept})
	return added, kept, err
}
