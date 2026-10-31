package session

import (
	"encoding/json"
	"errors"
	"fmt"
)

// OpenReply answers an "open" call.
type OpenReply struct {
	Info         *Info         `json:"info,omitempty"`
	Presentation *Presentation `json:"presentation,omitempty"`
}

// Host answers a player's calls about one disc at a time. The same calls come from a
// helper's protocol or straight from a player using its built-in copy, so a downloaded
// version and the built-in one behave alike. A *RefusalError is a plain no, not a
// failure of the module.
type Host struct {
	Open    Opener
	session Session
}

// Close ends the session in progress.
func (h *Host) Close() {
	if h.session != nil {
		_ = h.session.Close()
		h.session = nil
	}
}

// Handle answers one call with a result to send as JSON, and an optional blob.
func (h *Host) Handle(method string, raw json.RawMessage) (any, []byte, error) {
	if method == "open" {
		var in struct {
			Path  string `json:"path"`
			Drive bool   `json:"drive"`
			Prefs Prefs  `json:"prefs"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		h.Close()
		s, err := h.Open(in.Path, in.Drive, in.Prefs)
		if err != nil {
			return nil, nil, err
		}
		h.session = s
		info := s.Info()
		return OpenReply{Info: &info, Presentation: s.Start()}, nil, nil
	}
	if method == "close" {
		h.Close()
		return struct{}{}, nil, nil
	}
	if h.session == nil {
		return nil, nil, errors.New("no disc is open")
	}
	missing := func(what string) error {
		return &RefusalError{Code: CodeMissing, Message: "no such " + what}
	}
	switch method {
	case "action":
		var a Action
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, nil, err
		}
		return h.session.Do(a), nil, nil
	case "title":
		var in struct {
			TTN     int `json:"ttn"`
			Chapter int `json:"chapter"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		return h.session.PlayTitle(in.TTN, in.Chapter), nil, nil
	case "locate":
		var in struct {
			Sec float64 `json:"sec"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		loc, err := h.session.Locate(in.Sec)
		return loc, nil, err
	case "art":
		var in struct {
			Art   int    `json:"art"`
			Kind  string `json:"kind"`
			N     int    `json:"n"`
			Frame int    `json:"frame"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.Art(in.Art, in.Kind, in.N, in.Frame)
		if !ok {
			return nil, nil, missing("picture")
		}
		return struct{}{}, b, nil
	case "sound":
		var in struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.Sound(in.ID)
		if !ok {
			return nil, nil, missing("sound")
		}
		return struct{}{}, b, nil
	case "subs":
		var in struct {
			TTN    int     `json:"ttn"`
			Stream int     `json:"stream"`
			From   float64 `json:"from"`
			To     float64 `json:"to"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		ev, err := h.session.Subtitles(in.TTN, in.Stream, in.From, in.To)
		return ev, nil, err
	case "subpic":
		var in struct {
			Key string `json:"key"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, nil, err
		}
		b, ok := h.session.SubPic(in.Key)
		if !ok {
			return nil, nil, missing("picture")
		}
		return struct{}{}, b, nil
	}
	return nil, nil, fmt.Errorf("unknown call %q", method)
}
