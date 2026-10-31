package session

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewMediaLaysOutSegments(t *testing.T) {
	m := NewMedia("mpeg", "tok", [][]Extent{
		{{File: "a", Offset: 0, Length: 100}, {File: "b", Offset: 5, Length: 50}},
		{{File: "a", Offset: 200, Length: 10}},
	}, []float64{3, 4})
	if len(m.Files) != 2 || len(m.Extents) != 3 {
		t.Fatalf("files %v extents %v", m.Files, m.Extents)
	}
	if m.Extents[2] != [3]int64{0, 200, 10} {
		t.Errorf("the second use of a file makes a new entry: %v", m.Extents[2])
	}
	if m.Segments[0].Bytes != 150 || m.Segments[1].StartSec != 3 || m.Segments[1].BasePTS != -1 {
		t.Errorf("segments %+v", m.Segments)
	}
}

func TestHostNeedsADisc(t *testing.T) {
	h := &Host{Open: func(string, bool, Prefs) (Session, error) {
		return nil, &RefusalError{Code: CodeProtected, Message: MsgDVDProtected}
	}}
	if _, _, err := h.Handle("action", json.RawMessage(`{}`)); err == nil {
		t.Error("an action with no disc should fail")
	}
	_, _, err := h.Handle("open", json.RawMessage(`{"path":"x"}`))
	if !IsProtected(err) {
		t.Errorf("a refusal keeps its reason: %v", err)
	}
	var r *RefusalError
	if !errors.As(err, &r) || r.Message != MsgDVDProtected {
		t.Errorf("%v", err)
	}
	if PlainReason(err) != MsgDVDProtected {
		t.Errorf("a refusal is already worded: %s", PlainReason(err))
	}
	if got := PlainReason(errors.New("x")); got != "This part of the disc could not be read: x" {
		t.Errorf("%s", got)
	}
}
