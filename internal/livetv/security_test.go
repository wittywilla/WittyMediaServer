package livetv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wittywilla/wittytuner"
)

func securityServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"FriendlyName":"Box","ModelNumber":"M","DeviceID":"1","TunerCount":4}`)
	})
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{"GuideNumber":"5.1","GuideName":"WTST","URL":"%[1]s/ok"},
			{"GuideNumber":"105.1","GuideName":"WTST ATSC3","VideoCodec":"HEVC","AudioCodec":"AC4","Tags":"drm","URL":"%[1]s/ok"},
			{"GuideNumber":"7.1","GuideName":"ALONE","Tags":"drm","URL":"%[1]s/ok"},
			{"GuideNumber":"9.1","GuideName":"SNEAKY","URL":"%[1]s/scrambled"},
			{"GuideNumber":"11.1","GuideName":"WCLR3","VideoCodec":"HEVC","AudioCodec":"AC4","URL":"%[1]s/ok"}]`, srv.URL)
	})
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(3000, false)) })
	mux.HandleFunc("/scrambled", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(tsData(3000, true)) })
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func channelNamed(t *testing.T, list []*Channel, name string) *Channel {
	t.Helper()
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no channel %s", name)
	return nil
}

func TestSecurityStatusAndAlternatives(t *testing.T) {
	m := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "network", Name: "Box", Options: map[string]string{"address": srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if tn.Encrypted != 2 || tn.Clear != 3 {
		t.Fatalf("counts: %+v", tn)
	}
	list, _ := m.GuideChannels(ctx)
	a3, plain, alone := channelNamed(t, list, "WTST ATSC3"), channelNamed(t, list, "WTST"), channelNamed(t, list, "ALONE")
	if a3.Security != SecurityEncrypted || !a3.ATSC3 || plain.Security != SecurityClear || plain.ATSC3 {
		t.Fatalf("%+v\n%+v", a3, plain)
	}
	alts := Alternatives(list, nil)
	if got := alts[a3.ID]; got == nil || got.Number != "5.1" || got.Key != fmt.Sprintf("tun-%d", plain.ID) {
		t.Fatalf("the same station on ATSC 1.0 should be offered: %+v", got)
	}
	if alts[alone.ID] != nil {
		t.Fatalf("a station with no clear version gets no offer: %+v", alts[alone.ID])
	}
	if alts[plain.ID] != nil {
		t.Fatal("a clear channel needs no alternative")
	}
	// Silent channels get an offer too, and are not offered themselves.
	silent := channelNamed(t, list, "WCLR3")
	withSilent := Alternatives(list, func(c *Channel) bool { return c.ID == silent.ID })
	if withSilent[silent.ID] != nil {
		t.Fatal("WCLR3 has no other version")
	}
	// The encrypted one is never opened, and never reaches the device.
	if _, err := m.Open(ctx, a3.ID); !wittytuner.IsEncrypted(err) {
		t.Fatalf("%v", err)
	}
	if !strings.Contains((&wittytuner.EncryptedError{Channel: "X", Why: "y"}).Error(), wittytuner.EncryptedLabel) {
		t.Fatal("the refusal uses the label")
	}
}

func TestUnencryptedATSC3Plays(t *testing.T) {
	m := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	if _, err := m.Create(ctx, &Tuner{Driver: "network", Name: "Box", Options: map[string]string{"address": srv.URL}}); err != nil {
		t.Fatal(err)
	}
	list, _ := m.GuideChannels(ctx)
	c := channelNamed(t, list, "WCLR3")
	if !c.ATSC3 || c.Security != SecurityClear || c.VideoCodec != "HEVC" || c.AudioCodec != "AC4" {
		t.Fatalf("%+v", c)
	}
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		t.Fatalf("unencrypted ATSC 3.0 is an ordinary channel: %v", err)
	}
	o.Release()
}

func TestStreamShowsWhatItIs(t *testing.T) {
	m := testManager(t)
	srv := securityServer(t)
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "network", Name: "Box", Options: map[string]string{"address": srv.URL}})
	if err != nil {
		t.Fatal(err)
	}
	list, _ := m.GuideChannels(ctx)
	c := channelNamed(t, list, "SNEAKY")
	if c.Security != SecurityClear {
		t.Fatalf("the device said nothing against it: %+v", c)
	}
	o, err := m.Open(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 188*64)
	for {
		if _, err := o.Stream.Reader.Read(buf); err != nil {
			if !wittytuner.IsEncrypted(err) {
				t.Fatalf("the guard should stop it: %v", err)
			}
			break
		}
	}
	o.Release()
	got, _ := m.Channel(ctx, c.ID)
	if got.Security != SecurityEncrypted || got.Encrypted != wittytuner.ScrambledReason {
		t.Fatalf("it should now be labelled: %+v", got)
	}
	// A rescan reads the same clean lineup: what the stream showed stays.
	if err := m.Refresh(ctx, tn.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = m.Channel(ctx, c.ID)
	if got.Security != SecurityEncrypted {
		t.Fatal("a rescan must not forget what the stream showed")
	}
	if _, err := m.Open(ctx, c.ID); !wittytuner.IsEncrypted(err) {
		t.Fatalf("now refused before tuning: %v", err)
	}
	// Check again only forgets: the guard still stands in the way.
	if _, err := m.EditChannel(ctx, c.ID, ChannelEdit{CheckAgain: true}); err != nil {
		t.Fatal(err)
	}
	o, err = m.Open(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var last error
	for last == nil {
		_, last = o.Stream.Reader.Read(buf)
	}
	if !wittytuner.IsEncrypted(last) {
		t.Fatalf("Check again must not get past the guard: %v", last)
	}
	o.Release()
}

func TestPlaylistChannelIsCheckedByPlaying(t *testing.T) {
	m := testManager(t)
	streams := securityServer(t)
	list := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "#EXTM3U\n#EXTINF:-1,Long\n%s/ok\n", streams.URL)
	}))
	defer list.Close()
	ctx := context.Background()
	tn, err := m.Create(ctx, &Tuner{Driver: "iptv", Name: "L", Options: map[string]string{"playlist": list.URL}})
	if err != nil {
		t.Fatal(err)
	}
	ch, _ := m.Channels(ctx, tn.ID)
	if ch[0].Security != SecurityUnknown {
		t.Fatalf("a playlist doesn't say: %+v", ch[0])
	}
	o, err := m.Open(ctx, ch[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 188*64)
	for {
		if _, err := o.Stream.Reader.Read(buf); err != nil {
			break
		}
	}
	o.Release()
	got, _ := m.Channel(ctx, ch[0].ID)
	if got.Security != SecurityClear {
		t.Fatalf("%+v", got)
	}
}

func TestRecordingRestrictionsAreHonoured(t *testing.T) {
	m := testManager(t)
	srv := iptvServer(t)
	_, ch := addIPTV(t, m, srv)
	ctx := context.Background()
	yes := true
	if _, err := m.EditChannel(ctx, ch[0].ID, ChannelEdit{NoRecord: &yes}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	if _, err := m.Schedule(ctx, ch[0].ID, 1, "x", "", now, now+60); err == nil || !strings.Contains(err.Error(), "can't be recorded") {
		t.Fatalf("%v", err)
	}
	if _, err := m.AddRule(ctx, ch[0].ID, 1, "x"); err == nil {
		t.Fatal("nor as a series")
	}
	// Scheduled first, restricted after: it must not start.
	no := false
	if _, err := m.EditChannel(ctx, ch[0].ID, ChannelEdit{NoRecord: &no}); err != nil {
		t.Fatal(err)
	}
	r, err := m.Schedule(ctx, ch[0].ID, 1, "x", "", now, now+60)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.EditChannel(ctx, ch[0].ID, ChannelEdit{NoRecord: &yes}); err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	m.Folder = func() string { return folder }
	full, _ := m.recording(ctx, r.ID)
	m.record(full)
	got, _ := m.recording(ctx, r.ID)
	if got.Status != "failed" || got.Path != "" {
		t.Fatalf("%+v", got)
	}
}

func TestHelpOnlySaysWhatWasChecked(t *testing.T) {
	h := EncryptedHelp()
	if h.Checked == "" || len(h.Devices) == 0 {
		t.Fatalf("%+v", h)
	}
	for _, d := range h.Devices {
		if !strings.HasPrefix(d.URL, "https://") || d.Facts == "" || d.Source == "" {
			t.Errorf("every device note names its source: %+v", d)
		}
	}
	text := fmt.Sprintf("%+v", h)
	for _, needle := range []string{"licensed", "ATSC 1.0", "isn't a device licensed", "support can change"} {
		if !strings.Contains(text, needle) {
			t.Errorf("help lacks %q", needle)
		}
	}
}
