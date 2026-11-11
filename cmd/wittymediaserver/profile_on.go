// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

//go:build wmsprofile

// A profiling port, for measuring the server (docs/PERFORMANCE.md). Only a build
// made with -tags wmsprofile has it, and only when WMS_PPROF names an address on
// this machine: release builds have none of this in them.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strings"
	"sync"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
	"github.com/wittywilla/wittymediaserver/internal/sysinfo"
)

func init() {
	addr := os.Getenv("WMS_PPROF")
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); err != nil || ip == nil || !ip.IsLoopback() {
		fmt.Fprintln(os.Stderr, "WMS_PPROF must be a loopback address such as 127.0.0.1:6060")
		os.Exit(2)
	}
	db.Trace = traceStatement
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WMS_PPROF:", err)
		os.Exit(2)
	}
	m := http.NewServeMux()
	m.HandleFunc("/debug/pprof/", pprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", pprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	m.HandleFunc("/debug/pprof/trace", pprof.Trace)
	m.HandleFunc("/debug/wms", profileVars)
	m.HandleFunc("/debug/wms/sql", profileSQL)
	// How much of what the process holds is only waiting to go back to the system.
	m.HandleFunc("POST /debug/wms/free", func(w http.ResponseWriter, _ *http.Request) { debug.FreeOSMemory() })
	go (&http.Server{Handler: loopbackOnly(m), ReadHeaderTimeout: 10 * time.Second}).Serve(ln)
}

// loopbackOnly answers only requests addressed to this machine by number or as
// localhost: a web page can point a name of its own at 127.0.0.1 (DNS rebinding), and
// the heap profile holds whatever the server holds in memory.
func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var profileMetrics = []string{
	"/gc/heap/live:bytes",
	"/gc/heap/goal:bytes",
	"/gc/cycles/total:gc-cycles",
	"/gc/heap/allocs:bytes",
	"/gc/heap/allocs:objects",
	"/memory/classes/total:bytes",
	"/memory/classes/heap/released:bytes",
	"/memory/classes/heap/free:bytes",
	"/memory/classes/heap/objects:bytes",
	"/memory/classes/heap/unused:bytes",
	"/memory/classes/heap/stacks:bytes",
	"/memory/classes/os-stacks:bytes",
	"/memory/classes/metadata/other:bytes",
	"/memory/classes/other:bytes",
	"/memory/classes/profiling/buckets:bytes",
	"/sched/goroutines:goroutines",
	"/cpu/classes/gc/total:cpu-seconds",
	"/cpu/classes/user:cpu-seconds",
	"/cpu/classes/idle:cpu-seconds",
}

// profileVars is what the benchmark reads: the process as the system sees it, and
// the Go runtime's own counters.
func profileVars(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"goroutines": runtime.NumGoroutine()}
	if u, ok := sysinfo.ReadProcess(); ok {
		out["rss"], out["cpuSec"], out["threads"], out["handles"] = u.RSS, u.CPUSec, u.Threads, u.Handles
	}
	s := make([]metrics.Sample, len(profileMetrics))
	for i, n := range profileMetrics {
		s[i].Name = n
	}
	metrics.Read(s)
	rt := map[string]any{}
	for _, x := range s {
		switch x.Value.Kind() {
		case metrics.KindUint64:
			rt[x.Name] = x.Value.Uint64()
		case metrics.KindFloat64:
			rt[x.Name] = x.Value.Float64()
		}
	}
	out["runtime"] = rt
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Every statement the server ran, with how often and how long, for finding the slow
// ones and the ones run once per row.
var (
	sqlMu    sync.Mutex
	sqlStats = map[string]*sqlStat{}
	sqlCount int64
)

type sqlStat struct {
	Query string
	N     int64
	Sec   float64
}

// idList is a list of numbers written into a statement, as Store's ids() does.
var idList = regexp.MustCompile(`IN \(\d+(\s*,\s*\d+)*\)`)

func traceStatement(q string, took time.Duration) {
	// A list of ids written in makes each page's statement different: they're one
	// statement, run once per page, which is what finding one run per row needs to see.
	q = idList.ReplaceAllString(q, "IN (…)")
	sqlMu.Lock()
	defer sqlMu.Unlock()
	sqlCount++
	st := sqlStats[q]
	if st == nil {
		st = &sqlStat{Query: strings.Join(strings.Fields(q), " ")}
		sqlStats[q] = st
	}
	st.N++
	st.Sec += took.Seconds()
}

func profileSQL(w http.ResponseWriter, _ *http.Request) {
	sqlMu.Lock()
	out := struct {
		Count      int64
		Statements []sqlStat
	}{Count: sqlCount}
	for _, st := range sqlStats {
		out.Statements = append(out.Statements, *st)
	}
	sqlMu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
