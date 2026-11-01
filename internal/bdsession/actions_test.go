package bdsession

import (
	"context"
	"log/slog"
	"testing"

	"github.com/wittywilla/wittybluray"
)

func quietSession(t *testing.T) *Session {
	s := seamSession(t)
	s.log = slog.New(slog.DiscardHandler)
	return s
}

// panicNav goes wrong the way bad disc data can, part-way through an action.
type panicNav struct{ Navigator }

func (panicNav) PlayPlaylist(*wittybluray.Playlist, int, uint32) *wittybluray.Step {
	panic("bad disc data")
}

// A panic under an action is one bad request. With the session's lock let go and taken
// again around it, unlocking on the way out was a fatal error nothing can catch.
func TestAPanicDuringATitleActionCanBeRecovered(t *testing.T) {
	s := quietSession(t)
	s.nav = panicNav{s.nav}
	defer func() {
		if recover() == nil {
			t.Fatal("it should have panicked, for the server's own recover to catch")
		}
	}()
	s.Do(context.Background(), Action{Type: "title", TTN: 1})
}

// countNav notes anything that would move an encrypted disc on.
type countNav struct {
	Navigator
	calls int
}

func (n *countNav) MenuCall() *wittybluray.Step { n.calls++; return n.Navigator.MenuCall() }
func (n *countNav) Resume() *wittybluray.Step   { n.calls++; return n.Navigator.Resume() }

// Whatever is pressed on the refusal screen, the refusal stays.
func TestEncryptedDiscStaysRefused(t *testing.T) {
	s := quietSession(t)
	nav := &countNav{Navigator: s.nav}
	s.nav = nav
	s.disc.Encrypted = true
	ctx := context.Background()
	first := s.Start(ctx)
	if first.Kind != "stop" || !first.Scrambled {
		t.Fatalf("%+v", first)
	}
	for _, a := range []Action{{Type: "menu"}, {Type: "resume"}, {Type: "tick"}, {Type: "title", TTN: 1}, {Type: "ended"}, {Type: "key", Key: "menu"}} {
		if p := s.Do(ctx, a); p.Kind != "stop" || !p.Scrambled || p.Serial != first.Serial {
			t.Errorf("%s: %+v", a.Type, p)
		}
	}
	if nav.calls != 0 {
		t.Errorf("the navigator was asked %d times", nav.calls)
	}
	// Asked straight away, with no refusal on the screen yet.
	s = quietSession(t)
	s.disc.Encrypted = true
	if p := s.Do(ctx, Action{Type: "menu"}); p.Kind != "stop" || !p.Scrambled {
		t.Errorf("%+v", p)
	}
}

// stillNav is a disc's button that holds or releases the picture.
type stillNav struct {
	Navigator
	still bool
	fx    wittybluray.Effects
}

func (n *stillNav) PressButton([]wittybluray.Command) *wittybluray.Step {
	fx := n.fx
	fx.Still = &n.still
	return &wittybluray.Step{Kind: wittybluray.StepStay, Effects: &fx}
}

// A button's pause reaches the player, and only in that one answer.
func TestAButtonsPauseReachesThePlayer(t *testing.T) {
	ctx := context.Background()
	for name, fx := range map[string]wittybluray.Effects{"moves the selection": {}, "hides the pop-up": {PopupOff: true}} {
		s := quietSession(t)
		s.nav = &stillNav{Navigator: s.nav, still: true, fx: fx}
		page := &wittybluray.Page{BOGs: []wittybluray.BOG{{Buttons: []wittybluray.Button{{ID: 0}}}}}
		s.menu = &menuState{menu: &wittybluray.Menu{}, page: page, enabled: map[int]bool{}, disabled: map[int]bool{}}
		s.current = &Presentation{Kind: "menu", Menu: &Menu{}}
		p := s.press(ctx, 1)
		if p == nil || p.Pause == nil || !*p.Pause {
			t.Errorf("%s: the answer lost the pause: %+v", name, p)
		}
		if s.current.Pause != nil {
			t.Errorf("%s: the session kept it, and would say it again", name)
		}
		if again := s.Do(ctx, Action{Type: "tick"}); again.Pause != nil {
			t.Errorf("%s: said again: %+v", name, again)
		}
	}
}
