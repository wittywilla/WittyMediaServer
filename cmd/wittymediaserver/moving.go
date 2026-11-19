// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/datahome"
)

// showMoving answers on the server's port while the data folder is copied, so the page that asked
// for the move can say how far it is instead of waiting on a server that seems to be gone. It
// says nothing but how many bytes are done.
func showMoving(addr string, p *datahome.Progress) (stop func()) {
	ln, err := listen(addr)
	if err != nil {
		return func() {}
	}
	srv := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Retry-After", "2")
		if r.URL.Path == "/api/data-location/progress" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"moving": true, "done": p.Done.Load(), "total": p.Total.Load()})
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("WittyMediaServer is moving its data to a new folder. It's back as soon as that's done.\n"))
	})}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
}
