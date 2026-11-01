package modules

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/wittywilla/wittymodule"
	"github.com/wittywilla/wittymodule/service"
)

// A service is a module that stays running while it's in use, for what one call at a time down a
// pipe can't do: a game has calls that overlap and connections a browser makes straight to it. It
// listens on this machine only, with a secret for each start. It starts on first use, and leaves
// a while after the last use, unless it says it's still busy.

const (
	// serviceStart is how long a service has to say where it is.
	serviceStart = 30 * time.Second
	// serviceLook is how often a running service's process is checked on as it's used.
	serviceLook = 5 * time.Second
	// serviceIdle is how long a service stays with nothing to do.
	serviceIdle = 5 * time.Minute
	// serviceWatch is how often that's looked at.
	serviceWatch = time.Minute
)

// serviceClient has no proxy and no time limit of its own: what's at the other end is on this
// machine, and each call has its own deadline.
var serviceClient = &http.Client{Transport: &http.Transport{
	Proxy:               nil,
	DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	MaxIdleConnsPerHost: 4,
	IdleConnTimeout:     time.Minute,
}}

// runningService is a service in use.
type runningService struct {
	a       *Attached
	conn    *service.Conn
	version string

	mu     sync.Mutex
	last   time.Time // the last time it was asked for
	looked time.Time // the last time its process was checked
	notify map[string]func(json.RawMessage)
}

// ServiceConn finds a module's service, starting it if need be. It is what service.Dial is set to.
func (m *Manager) ServiceConn(id string) (*service.Conn, error) {
	m.svcMu.Lock()
	defer m.svcMu.Unlock()
	now := time.Now()
	s := m.services[id]
	if s != nil && s.version != m.VersionInUse(id) {
		s.a.Close() // a new version, or none: the next call uses what's in use now
		delete(m.services, id)
		s = nil
	}
	if s != nil && now.Sub(s.looked) > serviceLook {
		s.looked = now
		// The helper protocol's pipe says whether the process is still there.
		if _, err := s.a.Call(service.MethodStart, nil, nil); err != nil {
			s.a.Close()
			delete(m.services, id)
			s = nil
		}
	}
	if s == nil {
		a := m.Attach(id, serviceStart)
		if a == nil {
			return nil, m.missing(id)
		}
		var started service.Started
		ns := &runningService{a: a, version: a.Version(), looked: now, notify: map[string]func(json.RawMessage){}}
		a.OnNotify(ns.heard)
		if _, err := a.Call(service.MethodStart, nil, &started); err != nil {
			a.Close()
			return nil, err
		}
		u, err := url.Parse(started.URL)
		if err != nil || u.Scheme != "http" || !loopback(u.Hostname()) {
			a.Close()
			err := errors.New("its service isn't on this machine")
			a.failedNow(err)
			return nil, err
		}
		ns.conn = &service.Conn{Base: u, Token: started.Token, Client: serviceClient}
		s = ns
		m.services[id] = s
		service.RunStarted(id, ns.conn)
		if !m.svcWatching {
			m.svcWatching = true
			go m.watchServices()
		}
	}
	s.last = now
	return s.conn, nil
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// heard passes on what a service says unasked, to whoever asked to hear it.
func (s *runningService) heard(method string, params json.RawMessage) {
	s.mu.Lock()
	fn := s.notify[method]
	s.mu.Unlock()
	if fn != nil {
		fn(params)
	}
}

// OnServiceNotify hears one kind of thing a module's running service says unasked. It lasts
// until the service next starts. fn must not block.
func (m *Manager) OnServiceNotify(id, method string, fn func(json.RawMessage)) error {
	if _, err := m.ServiceConn(id); err != nil {
		return err
	}
	m.svcMu.Lock()
	s := m.services[id]
	m.svcMu.Unlock()
	if s == nil {
		return m.missing(id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notify[method] = fn
	return nil
}

// watchServices stops what has nothing to do, unless it says it's busy.
func (m *Manager) watchServices() {
	t := time.NewTicker(serviceWatch)
	defer t.Stop()
	for range t.C {
		m.svcMu.Lock()
		if len(m.services) == 0 {
			m.svcWatching = false
			m.svcMu.Unlock()
			return
		}
		idle := map[string]*runningService{}
		for id, s := range m.services {
			if time.Since(s.last) > serviceIdle {
				idle[id] = s
			}
		}
		m.svcMu.Unlock()
		for id, s := range idle {
			if busy(s.conn) {
				continue
			}
			m.svcMu.Lock()
			if m.services[id] == s && time.Since(s.last) > serviceIdle {
				delete(m.services, id)
				s.a.Close()
			}
			m.svcMu.Unlock()
		}
	}
}

// busy asks a service. One that can't say counts as busy, since stopping it could end a game.
func busy(c *service.Conn) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var b bool
	if err := c.Call(ctx, service.CallBusy, nil, nil, nil, []any{&b}); err != nil {
		var ce *service.CallError
		if errors.As(err, &ce) && ce.Code == "unknown-method" {
			return false
		}
		return true
	}
	return b
}

// ServiceProxy passes a request through to a module's service, connections that stay open
// included. The server has already decided this person may make it.
func (m *Manager) ServiceProxy(id string, w http.ResponseWriter, r *http.Request, path string) error {
	c, err := m.ServiceConn(id)
	if err != nil {
		return err
	}
	p := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme, pr.Out.URL.Host = c.Base.Scheme, c.Base.Host
			pr.Out.URL.Path, pr.Out.URL.RawPath = path, ""
			pr.Out.Host = c.Base.Host
			pr.Out.Header.Set(service.TokenHeader, c.Token)
		},
		Transport:     serviceClient.Transport,
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			m.log.Warn("modules: passing a request to a service", "module", id, "err", err)
			http.Error(w, "the module isn't answering", http.StatusBadGateway)
		},
	}
	p.ServeHTTP(w, r)
	return nil
}

// closeServices stops every service, as the server stops.
func (m *Manager) closeServices() {
	m.svcMu.Lock()
	defer m.svcMu.Unlock()
	for id, s := range m.services {
		delete(m.services, id)
		s.a.Close()
	}
}

// The permissions each kind of module may be given, if its signed manifest asks. Everything else
// gets none, so a file that takes over a parser has nowhere to send anything and nothing to run.
// Only modules this project publishes are listed: a fork or anyone else's gets none of these.
var mayHave = map[string]map[string]bool{
	tunerID:       {wittymodule.PermissionNetwork: true, wittymodule.PermissionPrograms: true, wittymodule.PermissionDevices: true},
	"wittybluray": {wittymodule.PermissionPrograms: true, wittymodule.PermissionData: true},
	"wittyplay":   {wittymodule.PermissionNetwork: true, wittymodule.PermissionPrograms: true, wittymodule.PermissionDevices: true, wittymodule.PermissionData: true},
	"wittycouch":  {wittymodule.PermissionNetwork: true, wittymodule.PermissionPrograms: true, wittymodule.PermissionData: true},
	"wittyrigs":   {wittymodule.PermissionNetwork: true, wittymodule.PermissionPrograms: true, wittymodule.PermissionData: true},
}

// Call asks a module's helper one thing. Without a version in use, it says which is missing.
func (m *Manager) Call(id, method string, params, out any) ([]byte, error) {
	b, err, handled := m.call(id, method, params, out)
	if !handled {
		return nil, m.missing(id)
	}
	return b, err
}

// UseServices connects the service package to this manager, which stubs reach modules through.
func (m *Manager) UseServices() {
	service.Dial = m.ServiceConn
	service.Proxy = m.ServiceProxy
	service.Subscribe = m.OnServiceNotify
	service.Running = func(id string) bool {
		m.svcMu.Lock()
		defer m.svcMu.Unlock()
		return m.services[id] != nil
	}
}

// Capability is a fact from the manifest of the version in use.
func (m *Manager) Capability(id, name string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.mods[id]; e != nil {
		return e.man.Capabilities[name]
	}
	return nil
}
