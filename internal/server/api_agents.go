package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/library"
)

// Custom metadata agents (admin): the list, saved as one setting, and a test
// that asks an agent about a title without changing anything.

const agentSecret = "••••••"

// maskAgents hides header values (API keys) in what the page sees.
func maskAgents(list []library.Agent) []library.Agent {
	out := make([]library.Agent, len(list))
	for i, a := range list {
		if k, _, ok := strings.Cut(a.Header, ":"); ok {
			a.Header = k + ": " + agentSecret
		}
		out[i] = a
	}
	return out
}

func (s *Server) agents(ctx context.Context) []library.Agent {
	return library.ParseAgents(s.DB.GetSetting(ctx, "agents", ""))
}

// GET /api/agents
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"agents": maskAgents(s.agents(r.Context())), "kinds": library.AgentKinds})
}

// PUT /api/agents {agents: [...]}: replaces the list (its order is the order
// they run in). A masked header keeps the saved value.
func (s *Server) handlePutAgents(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Agents []library.Agent `json:"agents"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if len(b.Agents) > 20 {
		writeErr(w, 400, "up to 20 agents")
		return
	}
	old := map[string]string{}
	for _, a := range s.agents(r.Context()) {
		old[a.ID] = a.Header
	}
	seen := map[string]bool{}
	for i := range b.Agents {
		a := &b.Agents[i]
		if strings.Contains(a.Header, agentSecret) {
			a.Header = old[a.ID]
		}
		if err := a.Validate(); err != nil {
			writeErr(w, 400, a.Name+": "+err.Error())
			return
		}
		if seen[a.ID] {
			writeErr(w, 400, "two agents share an id")
			return
		}
		seen[a.ID] = true
	}
	raw, _ := json.Marshal(b.Agents)
	if err := s.DB.SetSetting(r.Context(), "agents", string(raw)); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	s.handleAgents(w, r)
}

// POST /api/agents/test {agent, title, year, kind}
func (s *Server) handleTestAgent(w http.ResponseWriter, r *http.Request) {
	var b struct {
		Agent library.Agent `json:"agent"`
		Title string        `json:"title"`
		Year  *int          `json:"year"`
		Kind  string        `json:"kind"`
	}
	if err := readJSON(r, &b); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.Contains(b.Agent.Header, agentSecret) {
		for _, a := range s.agents(r.Context()) {
			if a.ID == b.Agent.ID {
				b.Agent.Header = a.Header
			}
		}
	}
	b.Agent.Enabled = true
	if len(b.Agent.Kinds) == 0 {
		b.Agent.Kinds = []string{"movies"}
	}
	if err := b.Agent.Validate(); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if strings.TrimSpace(b.Title) == "" {
		writeErr(w, 400, "type a title to look up")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	res, err := library.TestAgent(ctx, &b.Agent, strings.TrimSpace(b.Title), b.Year, b.Kind)
	if err == library.ErrNotFound {
		writeErr(w, 404, "the agent found nothing for that title")
		return
	}
	if err != nil {
		writeErr(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, res)
}
