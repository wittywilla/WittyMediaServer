package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/logging"
)

// The evidence the server keeps for when something goes wrong: the flight recorder and
// Go's execution trace (see internal/logging/flight.go), the watchdog that writes them
// down when the server stops answering (hang.go), and the button in Settings → Logs for
// when someone has just seen something go wrong.

func init() {
	settingDefaults["logFlight"] = "1"
	settingDefaults["logTrace"] = "1"
}

// ApplyEvidenceSettings turns the recorders on or off as Settings say. At start, and
// whenever they change.
func (s *Server) ApplyEvidenceSettings(ctx context.Context) {
	if s.Logs == nil {
		return
	}
	s.Logs.SetFlightRecorder(s.DB.GetSetting(ctx, "logFlight", "1") == "1")
	if s.DB.GetSetting(ctx, "logTrace", "1") == "1" {
		if err := logging.StartTraceRecorder(); err != nil {
			s.Log.WarnContext(ctx, "the execution trace can't be kept", "cat", "server", "err", err)
		}
	} else {
		logging.StopTraceRecorder()
	}
}

// WatchChecks are the watchdog's questions: does the server answer a web request, and
// does its database answer. base is the server's own address, as this machine reaches it.
func (s *Server) WatchChecks(base string) []logging.Check {
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			Proxy:               nil, // its own address, never through a proxy
			DialContext:         (&net.Dialer{Timeout: 2 * time.Second}).DialContext,
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     30 * time.Second,
		},
	}
	health := strings.TrimRight(base, "/") + "/api/health"
	return []logging.Check{
		{Name: "web", Run: func(ctx context.Context) error {
			req, err := http.NewRequestWithContext(ctx, "GET", health, nil)
			if err != nil {
				return err
			}
			req.Header.Set("User-Agent", "WittyMediaServer watchdog")
			res, err := client.Do(req)
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return errors.New("no answer from its own web address in 2 seconds")
				}
				return fmt.Errorf("its own web address failed: %w", err)
			}
			res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("its own web address answered %d", res.StatusCode)
			}
			return nil
		}},
		{Name: "database", Run: func(ctx context.Context) error {
			var one int
			if err := s.DB.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
				if ctx.Err() != nil {
					return errors.New("no answer from the database in 2 seconds: every connection is busy, or one is stuck")
				}
				return err
			}
			return nil
		}},
	}
}

// POST /api/logs/save (admin) {note}: "something just went wrong". Writes a report with
// the flight recorder's last minutes, every goroutine's stack and the execution trace.
func (s *Server) handleSaveEvidence(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Note string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := readJSON(r, &in); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
	}
	if len(in.Note) > 500 {
		writeErr(w, 400, "that note is too long")
		return
	}
	summary := "Saved from Settings by " + userFrom(r).Name + "."
	if note := strings.TrimSpace(in.Note); note != "" {
		summary += " " + strings.Join(strings.Fields(note), " ")
	}
	c, err := logging.SaveEvidence("saved", summary, "")
	if err != nil {
		writeErr(w, 500, "it couldn't be saved: "+err.Error())
		return
	}
	_ = s.Crashes.MarkSeen(c.ID) // the one who saved it has seen it: no banner for it
	c.Seen = true
	s.Log.InfoContext(r.Context(), "what just happened was saved", "cat", "server", "report", c.ID)
	writeJSON(w, 200, c)
}

var partTypes = map[string]string{
	logging.PartFlight:     "text/plain; charset=utf-8",
	logging.PartGoroutines: "text/plain; charset=utf-8",
	logging.PartTrace:      "application/octet-stream",
}

// GET /api/logs/crashes/{id}/{part} (admin): what was kept beside a report.
func (s *Server) handleCrashPart(w http.ResponseWriter, r *http.Request) {
	id, part := r.PathValue("id"), r.PathValue("part")
	p, err := s.Crashes.PartPath(id, part)
	if err != nil {
		writeErr(w, 404, "no such part of a crash report")
		return
	}
	f, err := os.Open(p)
	if err != nil {
		writeErr(w, 404, "no such part of a crash report")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeErr(w, 404, "no such part of a crash report")
		return
	}
	w.Header().Set("Content-Type", partTypes[part])
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+"."+part+`"`)
	http.ServeContent(w, r, "", st.ModTime(), f)
}
