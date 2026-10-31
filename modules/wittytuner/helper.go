package wittytuner

import (
	"errors"
	"net/url"
	"strings"
)

// What a helper process (cmd/wittytuner-module) and a server say to each other. Each
// question is a process of its own, and its answer comes as a stream: a stream of
// bytes for a channel, and for everything else JSON documents, because a list of
// channels can be longer than one line of the protocol and a scan reports as it goes.

// HelperCall is what a helper is asked: which tuner, since it keeps nothing, and what for.
type HelperCall struct {
	Driver  string            `json:"driver,omitempty"`
	Name    string            `json:"name,omitempty"`
	Options map[string]string `json:"options,omitempty"`
	Channel *Channel          `json:"channel,omitempty"` // "open"
	Source  string            `json:"source,omitempty"`  // "scan": ScanOptions.Source
}

// HelperDoc is one document of an answer. Progress may come any number of times; any
// other document ends the answer.
type HelperDoc struct {
	Progress *Progress `json:"progress,omitempty"`
	Channels []Channel `json:"channels,omitempty"`
	Info     *Info     `json:"info,omitempty"`
	Found    []Found   `json:"found,omitempty"`
	Error    string    `json:"error,omitempty"`
	Code     string    `json:"code,omitempty"`
	Done     bool      `json:"done,omitempty"`
}

// HelperOpen is the answer to "open" for a stream FFmpeg should open by itself. It is
// a suggestion, and nothing else: the server opens and checks such a stream itself.
type HelperOpen struct {
	Input []string `json:"input,omitempty"`
}

// CheckLocator says whether this source would use an address for a channel. A helper's
// list of channels is only data, so the server asks its own copy.
func (c checked) CheckLocator(locator string) error {
	if l, ok := c.Source.(interface{ checkLocator(string) error }); ok {
		return l.checkLocator(locator)
	}
	return errors.New("this kind of tuner takes no addresses from anywhere else")
}

func (t *netTuner) checkLocator(locator string) error {
	_, err := t.streamURL(Channel{Locator: locator})
	return err
}

func (s *iptv) checkLocator(locator string) error {
	if s.single != "" {
		if locator != s.single {
			return errors.New("this channel has only one address")
		}
		return nil
	}
	return checkStreamURL(locator, s.localPlaylist())
}

func (b *backend) checkLocator(locator string) error {
	u, err := url.Parse(locator)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("that is not a stream address")
	}
	host := b.host
	if host == "" {
		base, err := url.Parse(b.base)
		if err != nil {
			return err
		}
		host = base.Hostname()
	}
	if !strings.EqualFold(u.Hostname(), host) {
		return errors.New("streams stay on the backend's own host")
	}
	return nil
}
