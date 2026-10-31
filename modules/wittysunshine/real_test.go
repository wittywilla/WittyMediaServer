package wittysunshine

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestRealSunshine runs against a real Sunshine, when one is named:
//
//	WITTYSUNSHINE_REAL=127.0.0.1 WITTYSUNSHINE_USER=name WITTYSUNSHINE_PASS=secret go test -run TestRealSunshine -v
//
// It pairs a new identity, which stays on that PC's list of devices afterwards.
func TestRealSunshine(t *testing.T) {
	addr := os.Getenv("WITTYSUNSHINE_REAL")
	if addr == "" {
		t.Skip("no real Sunshine named")
	}
	user, pass := os.Getenv("WITTYSUNSHINE_USER"), os.Getenv("WITTYSUNSHINE_PASS")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	id, err := NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	r := &Rig{Address: addr, Identity: id}
	hello, err := r.Hello(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("hello: %+v", hello)

	// The person typing the PIN, as a separate program would. Sunshine only answers this
	// once the client has finished pairing.
	pin, _ := NewPIN()
	typed := make(chan error, 1)
	go func() { typed <- typePIN(ctx, addr, user, pass, pin, DeviceName) }()
	cert, err := r.Pair(ctx, pin)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := <-typed; err != nil {
		t.Errorf("typing the PIN: %v", err)
	}
	r.Cert = cert
	t.Logf("pinned %s", CertFingerprint(cert))

	info, err := r.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("status: %+v", info)
	if info.UUID != hello.UUID {
		t.Errorf("different PC? %s, %s", info.UUID, hello.UUID)
	}
	apps, err := r.Apps(ctx)
	if err != nil {
		t.Fatalf("apps: %v", err)
	}
	t.Logf("apps: %+v", apps)
	if len(apps) == 0 {
		t.Fatal("no apps")
	}
	for _, a := range apps {
		data, ext, err := r.AppImage(ctx, a.ID)
		t.Logf("art for %q: %d bytes %q %v", a.Title, len(data), ext, err)
	}

	// A stranger's certificate and a wrong PIN are refused for real.
	other, _ := NewIdentity()
	stranger := &Rig{Address: addr, Identity: other, Cert: cert}
	if _, err := stranger.Apps(ctx); !errors.Is(err, ErrNotPaired) {
		t.Errorf("a stranger: %v", err)
	}
	wrong := &Rig{Address: addr, Identity: other}
	wrongCtx, wrongCancel := context.WithTimeout(ctx, 20*time.Second)
	defer wrongCancel()
	go func() { _ = typePIN(wrongCtx, addr, user, pass, "1111", DeviceName) }()
	if _, err := wrong.Pair(wrongCtx, "2222"); !errors.Is(err, ErrWrongPIN) {
		t.Errorf("a wrong PIN: %v", err)
	}

	// Starting and ending a game.
	var game App
	for _, a := range apps {
		if a.Title == "Test Game One" {
			game = a
		}
	}
	if game.ID == 0 {
		game = apps[len(apps)-1]
	}
	err = r.Launch(ctx, game.ID)
	t.Logf("launch %q: %v", game.Title, err)
	if err == nil {
		now, err := r.Status(ctx)
		t.Logf("running: %+v %v", now, err)
		if err == nil && now.CurrentGame != game.ID {
			t.Errorf("running %d, wanted %d", now.CurrentGame, game.ID)
		}
		if err := r.Launch(ctx, game.ID); !errors.Is(err, ErrBusy) {
			t.Errorf("second launch: %v", err)
		}
	}
	if err := r.Quit(ctx); err != nil {
		t.Errorf("quit: %v", err)
	}
	if now, err := r.Status(ctx); err != nil || now.CurrentGame != 0 {
		t.Errorf("after quit: %+v %v", now, err)
	}

	// Typing a device's PIN through the web interface, to the pinned certificate.
	if err := r.SubmitPIN(ctx, user, pass, "1234", ""); !errors.Is(err, ErrNoPending) {
		t.Errorf("nothing waiting: %v", err)
	}
	if err := r.SubmitPIN(ctx, user, "wrong-password", "1234", ""); !errors.Is(err, ErrWebSignIn) {
		t.Errorf("wrong sign-in: %v", err)
	}
	device, _ := NewIdentity()
	dev := &Rig{Address: addr, Identity: device}
	devCtx, devCancel := context.WithTimeout(ctx, 30*time.Second)
	defer devCancel()
	devPIN, _ := NewPIN()
	devDone := make(chan error, 1)
	go func() {
		// Something else pairing: its name differs, which SubmitPIN relies on.
		_, err := dev.pairNamed(devCtx, devPIN, "Living Room TV")
		devDone <- err
	}()
	var submit error
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		if submit = r.SubmitPIN(ctx, user, pass, devPIN, "Living Room TV"); !errors.Is(submit, ErrNoPending) {
			break
		}
	}
	if submit != nil {
		t.Errorf("submit: %v", submit)
	}
	if err := <-devDone; err != nil {
		t.Errorf("the device's pairing: %v", err)
	}
}

// typePIN is someone at the PC: it waits for a device to appear, and types the PIN. The
// certificate isn't checked here, as this is a test talking to its own throwaway Sunshine.
func typePIN(ctx context.Context, addr, user, pass, pin, name string) error {
	c := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	base := "https://" + addr + ":47990/api/pin"
	for ctx.Err() == nil {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
		req.SetBasicAuth(user, pass)
		res, err := c.Do(req)
		if err != nil {
			return err
		}
		var list struct {
			Pairings []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"pairings"`
		}
		err = json.NewDecoder(res.Body).Decode(&list)
		res.Body.Close()
		if err != nil {
			return err
		}
		for _, p := range list.Pairings {
			if p.Name != name {
				continue
			}
			body, _ := json.Marshal(map[string]string{"pairing_id": p.ID, "pin": pin, "name": name})
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base, bytes.NewReader(body))
			req.SetBasicAuth(user, pass)
			req.Header.Set("Content-Type", "application/json")
			res, err := c.Do(req)
			if err != nil {
				return err
			}
			res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("posting the PIN: %s", res.Status)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ctx.Err()
}
