package bdsession

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/bluray"
	"github.com/wittywilla/wittymediaserver/internal/dvdsession"
)

// sentHost keeps what was sent to the applications.
type sentHost struct {
	sent  []string
	kicks int
}

func (h *sentHost) Version() int                                       { return 1 }
func (h *sentHost) Kick()                                              { h.kicks++ }
func (h *sentHost) WaitChange(context.Context, int, time.Duration) int { return 0 }
func (h *sentHost) PNG() ([]byte, int)                                 { return nil, 0 }
func (h *sentHost) Send(f string, a ...any)                            { h.sent = append(h.sent, fmt.Sprintf(f, a...)) }
func (h *sentHost) Sync(*bluray.Registers)                             {}
func (h *sentHost) StartTitle(int, *bluray.BDJObject)                  {}
func (h *sentHost) Done() <-chan struct{}                              { return nil }
func (h *sentHost) Close()                                             {}

func TestTapsReachOnlyApplicationsThatTakeAPointer(t *testing.T) {
	h := &sentHost{}
	st := &bdjState{host: h, active: true}
	s := &Session{bd: st, m: &Manager{}, ID: "s"}
	s.current = s.bdjOver(&dvdsession.Presentation{Kind: "menu"}, st)
	tap := dvdsession.Action{Type: "pointer", X: 960, Y: 540}
	if p, ok := s.bdjAction(context.Background(), tap); !ok || p == nil {
		t.Fatal("a tap during a Java menu is the applications' to handle")
	}
	if len(h.sent) != 0 {
		t.Errorf("sent %v to applications that take no pointer", h.sent)
	}
	s.bdjEvent(st, JavaEvent{Kind: "pointer", Args: []string{"1"}})
	if !s.current.BDJ.Pointer || h.kicks == 0 {
		t.Error("the player hears that the applications take a pointer")
	}
	s.bdjAction(context.Background(), tap)
	s.bdjAction(context.Background(), dvdsession.Action{Type: "pointer", X: 1920, Y: 10})
	s.bdjAction(context.Background(), dvdsession.Action{Type: "pointer", X: -1, Y: 10})
	if len(h.sent) != 1 || h.sent[0] != "pointer 960 540" {
		t.Errorf("sent %v", h.sent)
	}
	s.bdjEvent(st, JavaEvent{Kind: "pointer", Args: []string{"0"}})
	if s.current.BDJ.Pointer {
		t.Error("an application that stops listening takes taps no longer")
	}
}
