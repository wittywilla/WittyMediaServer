package library

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func localServer(t *testing.T, addr string, h http.HandlerFunc) string {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Skipf("can't listen on %s here: %v", addr, err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener.Close()
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL
}

// A custom agent is on the home network if the owner says so. That's no reason for one to be able
// to send the server to anything else there. 127.0.0.2 stands in for another device.
func TestAgentRedirectStaysOnItsHost(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("uses 127.0.0.2")
	}
	var reached atomic.Int64
	device := localServer(t, "127.0.0.2:0", func(w http.ResponseWriter, r *http.Request) {
		reached.Add(1)
		fmt.Fprint(w, `{"title":"from the device","overview":"its private JSON"}`)
	})
	ctx := context.Background()
	agent := localServer(t, "127.0.0.1:0", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, device+"/status.json", 302)
		case "/round":
			http.Redirect(w, r, "/here", 302)
		default:
			fmt.Fprint(w, `{"title":"from the agent"}`)
		}
	})
	if r, err := (&Agent{ID: "a", Name: "A", URL: agent + "/away?q={title}", Enabled: true}).Lookup(ctx, "x", nil, "movies"); err == nil {
		t.Errorf("it followed the redirect: %+v", r)
	}
	if reached.Load() != 0 {
		t.Errorf("the device was reached %d times", reached.Load())
	}
	// Its own redirects and its own address go on working, at home.
	r, err := (&Agent{ID: "a", Name: "A", URL: agent + "/round?q={title}", Enabled: true}).Lookup(ctx, "x", nil, "movies")
	if err != nil || !strings.Contains(r.Title, "from the agent") {
		t.Errorf("%+v %v", r, err)
	}
}
