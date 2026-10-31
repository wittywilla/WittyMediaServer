// wittybluray-player is WittyBluray as a helper for WittyMediaPlayer: a program of its own, so it can be
// updated without a new player, and a damaged disc can only take this down with it.
package main

import (
	"encoding/json"
	"errors"

	"github.com/wittywilla/wittybluray/wittyblurayplayer"
	"github.com/wittywilla/wittydisc/session"
	"github.com/wittywilla/wittymodule"
)

func main() {
	h := &session.Host{Open: wittyblurayplayer.Open}
	wittymodule.Main(wittyblurayplayer.Manifest, func(method string, params json.RawMessage) (any, []byte, error) {
		res, blob, err := h.Handle(method, params)
		var r *session.RefusalError
		if errors.As(err, &r) {
			return nil, nil, &wittymodule.CallError{Code: r.Code, Message: r.Message}
		}
		return res, blob, err
	})
}
