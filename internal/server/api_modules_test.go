package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	modmgr "github.com/wittywilla/wittymediaserver/internal/modules"
)

func TestModuleRoutes(t *testing.T) {
	call := func(s *Server, h func(*Server) http.HandlerFunc, id string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/modules/"+id, nil)
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()
		h(s)(w, r)
		return w
	}
	list := func(s *Server) http.HandlerFunc { return s.handleModules }
	install := func(s *Server) http.HandlerFunc { return s.handleModuleInstall }
	rollback := func(s *Server) http.HandlerFunc { return s.handleModuleRollback }

	// A build without them says so.
	none := &Server{}
	for _, h := range []func(*Server) http.HandlerFunc{list, install, rollback} {
		if w := call(none, h, "wittyphotos"); w.Code != 404 {
			t.Errorf("without modules: %d", w.Code)
		}
	}

	m := modmgr.New(modmgr.Config{Dir: filepath.Join(t.TempDir(), "modules"), Server: "0.4.0", Builtins: modmgr.GoManifests()})
	defer m.Close()
	s := &Server{Modules: m}
	w := call(s, list, "")
	if w.Code != 200 {
		t.Fatalf("list: %d", w.Code)
	}
	var r modmgr.Report
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, st := range r.Modules {
		if st.ID == "wittyphotos" {
			found = st.Updatable && st.State == modmgr.StateBuiltin && st.MaxServer != ""
		}
	}
	if !found || r.Server != "0.4.0" || r.API < 1 {
		t.Fatalf("%+v", r)
	}
	if w := call(s, install, "nothing"); w.Code != 404 {
		t.Errorf("unknown module: %d", w.Code)
	}
	if w := call(s, install, "wittyphotos"); w.Code != 409 {
		t.Errorf("nothing known to install: %d", w.Code)
	}
	if w := call(s, rollback, "wittyphotos"); w.Code != 409 {
		t.Errorf("nothing to roll back: %d", w.Code)
	}
	if w := call(s, install, "wittydvd"); w.Code != 409 {
		t.Errorf("a module that updates with the server: %d", w.Code)
	}
}
