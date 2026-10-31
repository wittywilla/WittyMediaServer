package gamestream

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Sunshine's web interface, used for one thing: typing a PIN for another device, so an
// admin at the couch needn't walk to the PC. It takes Sunshine's own sign-in, which can
// run any command on that PC, so it is passed through once and never kept, and only ever
// sent to the certificate pinned at pairing. The first pairing can't use this, as there
// is no pinned certificate to send it to yet.

var (
	ErrWebSignIn = errors.New("Sunshine didn't accept that user name and password")
	ErrWebDenied = errors.New("Sunshine's web interface refused this server. Its setting \"Origin Web UI Allowed\" has to include your network")
	ErrNoPending = errors.New("No device is waiting to pair. Start pairing on the device first, then type its PIN here")
	ErrManyWait  = errors.New("Several devices are waiting to pair. Type the device's name as Sunshine shows it")
)

// SubmitPIN gives Sunshine a PIN for a device that is waiting to pair. Newer versions of
// Sunshine name each waiting device; older ones take the PIN for whichever is waiting.
func (r *Rig) SubmitPIN(ctx context.Context, user, password, pin, deviceName string) error {
	if !ValidPIN(pin) {
		return errors.New("A PIN is four digits")
	}
	if len(deviceName) > 128 {
		return errors.New("That device name is too long")
	}
	if len(r.Cert) == 0 {
		return ErrNotPaired
	}
	c, err := r.pinnedClient(false, r.Cert)
	if err != nil {
		return err
	}
	base := r.url("https", r.base()+offsetWeb, "/api/pin", nil)
	ctx, cancel := context.WithTimeout(ctx, 3*questionTimeout)
	defer cancel()
	call := func(method string, payload any) (int, []byte, error) {
		var body io.Reader
		if payload != nil {
			b, err := json.Marshal(payload)
			if err != nil {
				return 0, nil, err
			}
			body = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, base, body)
		if err != nil {
			return 0, nil, err
		}
		req.SetBasicAuth(user, password)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.Do(req)
		if err != nil {
			return 0, nil, explain(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(io.LimitReader(res.Body, maxAnswer))
		return res.StatusCode, data, err
	}
	code, data, err := call(http.MethodGet, nil)
	if err != nil {
		return err
	}
	send := map[string]string{"pin": pin, "name": deviceName}
	if send["name"] == "" {
		send["name"] = "Moonlight"
	}
	switch code {
	case http.StatusOK:
		var list struct {
			Pairings []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"pairings"`
		}
		if err := json.Unmarshal(data, &list); err != nil {
			return ErrBadAnswer
		}
		id, match := "", 0
		for _, p := range list.Pairings {
			if p.Name == DeviceName {
				continue // this server's own pairing is never finished this way
			}
			if deviceName == "" || p.Name == deviceName {
				id, match = p.ID, match+1
			}
		}
		switch {
		case match == 0:
			return ErrNoPending
		case match > 1:
			return ErrManyWait
		}
		send["pairing_id"] = id
	case http.StatusUnauthorized, http.StatusFound, http.StatusSeeOther:
		return ErrWebSignIn
	case http.StatusForbidden:
		return ErrWebDenied
	case http.StatusNotFound, http.StatusMethodNotAllowed: // an older Sunshine
	default:
		return ErrBadAnswer
	}
	code, data, err = call(http.MethodPost, send)
	if err != nil {
		return err
	}
	switch code {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return ErrWebSignIn
	case http.StatusForbidden:
		return ErrWebDenied
	default:
		return ErrBadAnswer
	}
	var res struct {
		Status bool `json:"status"`
	}
	if err := json.Unmarshal(data, &res); err != nil {
		return ErrBadAnswer
	}
	if !res.Status {
		return ErrWrongPIN
	}
	return nil
}
