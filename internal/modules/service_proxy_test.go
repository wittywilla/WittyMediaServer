package modules

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wittywilla/wittymodule/service"
)

// What a browser sends under X-Witty- names never reaches a module; the server's own word does.
func TestProxyToServiceSetsIdentityAndDropsForgeries(t *testing.T) {
	var got http.Header
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, path = r.Header.Clone(), r.URL.Path
	}))
	defer srv.Close()
	base, _ := url.Parse(srv.URL)
	c := &service.Conn{Base: base, Token: "real", Client: srv.Client()}

	send := func(r *http.Request) {
		r.Header.Set("X-Witty-User", "1")
		r.Header.Set("X-Witty-Admin", "1")
		r.Header.Set("x-witty-extra", "1")
		r.Header.Set(service.TokenHeader, "forged")
		ProxyToService(c, httptest.NewRecorder(), r, "/docker/status", nil)
	}

	// No identity: nothing claimed survives.
	send(httptest.NewRequest("GET", "/whatever", nil))
	if path != "/docker/status" || got.Get("X-Witty-User") != "" || got.Get("X-Witty-Admin") != "" || got.Get("X-Witty-Extra") != "" || got.Get(service.TokenHeader) != "real" {
		t.Errorf("no identity: %s %v", path, got)
	}
	// A person: their id only.
	r := httptest.NewRequest("GET", "/whatever", nil)
	send(r.WithContext(WithIdentity(r.Context(), 42, false)))
	if got.Get("X-Witty-User") != "42" || got.Get("X-Witty-Admin") != "" || got.Get("X-Witty-Extra") != "" {
		t.Errorf("person: %v", got)
	}
	// An administrator.
	r = httptest.NewRequest("GET", "/whatever", nil)
	send(r.WithContext(WithIdentity(r.Context(), 7, true)))
	if got.Get("X-Witty-User") != "7" || got.Get("X-Witty-Admin") != "1" {
		t.Errorf("admin: %v", got)
	}
}
