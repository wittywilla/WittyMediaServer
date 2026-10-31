package relay

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/wittywilla/wittymoonlight/gamestream"
)

// TestRealStream takes a real stream from a real Sunshine through the real host program:
//
//	WITTYMOONLIGHT_REAL=127.0.0.1 WITTYMOONLIGHT_USER=name WITTYMOONLIGHT_PASS=secret WITTYMOONLIGHT_HOST=/path/to/wittymoonlight-host go test ./relay -run TestRealStream -v
func TestRealStream(t *testing.T) {
	addr, program := os.Getenv("WITTYMOONLIGHT_REAL"), os.Getenv("WITTYMOONLIGHT_HOST")
	if addr == "" || program == "" {
		t.Skip("no real Sunshine and host program named")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := CheckProgram(ctx, program); err != nil {
		t.Fatal(err)
	}
	id, err := gamestream.NewIdentity()
	if err != nil {
		t.Fatal(err)
	}
	r := &gamestream.Rig{Address: addr, Identity: id}
	pin, _ := gamestream.NewPIN()
	typed := make(chan error, 1)
	go func() { typed <- typePIN(ctx, addr, pin) }()
	cert, err := r.Pair(ctx, pin)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if err := <-typed; err != nil {
		t.Fatal(err)
	}
	r.Cert = cert
	info, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := r.Apps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	game := apps[len(apps)-1]
	var key, iv [16]byte
	_, _ = rand.Read(key[:])
	_, _ = rand.Read(iv[:])
	opts := gamestream.StreamOptions{AppID: game.ID, Width: 1280, Height: 720, FPS: 30, Key: key, IV: iv}
	launched, err := r.Launch(ctx, opts)
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	t.Logf("launched %q: %s", game.Title, launched.SessionURL)
	defer r.Quit(context.Background())
	s, err := New(ctx, Options{Program: program, Start: Start{
		Address: addr, AppVersion: info.Version, GfeVersion: info.GfeVersion, RTSP: launched.SessionURL, CodecModes: info.CodecModes,
		Width: 1280, Height: 720, FPS: 30, BitrateKbps: 5000, PacketSize: 1024, Key: key, IV: iv,
	}})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	defer s.Close()
	time.Sleep(6 * time.Second)
	v, a := s.Counts()
	t.Logf("after six seconds: %d frames, %d audio packets, %+v", v, a, s.Info())
	if v == 0 {
		t.Error("no video arrived")
	}
}

func typePIN(ctx context.Context, addr, pin string) error {
	user, pass := os.Getenv("WITTYMOONLIGHT_USER"), os.Getenv("WITTYMOONLIGHT_PASS")
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
				ID string `json:"id"`
			} `json:"pairings"`
		}
		err = json.NewDecoder(res.Body).Decode(&list)
		res.Body.Close()
		if err != nil {
			return err
		}
		if len(list.Pairings) > 0 {
			body, _ := json.Marshal(map[string]string{"pairing_id": list.Pairings[0].ID, "pin": pin, "name": "relay test"})
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
