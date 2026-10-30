package library

import (
	"context"
	"encoding/xml"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

const testFeed = `<?xml version="1.0"?>
<rss version="2.0" xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd">
<channel>
  <title>Test Cast</title><itunes:author>Host</itunes:author><description>&lt;p&gt;About &amp;amp; more&lt;/p&gt;</description>
  <itunes:image href="https://example.com/cover.jpg"/>
  <item>
    <title>Episode 2</title><guid>ep-2</guid><pubDate>Tue, 14 Oct 2025 10:00:00 +0000</pubDate>
    <itunes:duration>1:02:03</itunes:duration><itunes:episode>2</itunes:episode><itunes:explicit>yes</itunes:explicit>
    <enclosure url="https://example.com/2.mp3" type="audio/mpeg" length="1"/>
  </item>
</channel>
</rss>`

func TestFeedParsing(t *testing.T) {
	var f rssFeed
	if err := xml.Unmarshal([]byte(testFeed), &f); err != nil {
		t.Fatal(err)
	}
	ch := f.Channel
	if ch.Title != "Test Cast" || ch.Author != "Host" || len(ch.Images) != 1 || ch.Images[0].Href == "" || len(ch.Items) != 1 {
		t.Fatalf("channel: %+v", ch)
	}
	e := ch.Items[0]
	if parseDuration(e.Duration) != 3723 || parsePub(e.PubDate) != "2025-10-14" || e.Enclosure.URL == "" || *num(e.Episode) != 2 {
		t.Errorf("item: %+v", e)
	}
	if parseDuration("62:03") != 3723 || parseDuration("3723") != 3723 || parseDuration("abc") != 0 {
		t.Error("durations")
	}
	if codecForMime("audio/mpeg", "") != "mp3" || codecForMime("audio/x-m4a", "") != "aac" {
		t.Error("codecs")
	}
	if got := safeName(`a/b:c*"d?.`); strings.ContainsAny(got, `/:*"?`) || strings.HasSuffix(got, ".") {
		t.Errorf("safeName: %q", got)
	}
}

// Feeds and episodes may only come from the internet, never from this
// machine or the local network.
func TestPublicIP(t *testing.T) {
	cases := map[string]bool{
		"8.8.8.8": true, "1.1.1.1": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "::1": false, "10.0.0.1": false, "172.16.0.1": false, "192.168.1.1": false, "169.254.169.254": false,
		"0.0.0.0": false, "0.1.2.3": false, "100.64.0.1": false, "100.127.255.1": false, "198.18.0.1": false, "224.0.0.1": false, "240.0.0.1": false,
		"fc00::1": false, "fe80::1": false, "ff02::1": false, "::ffff:127.0.0.1": false, "::ffff:192.168.1.1": false, "64:ff9b::7f00:1": false, "::": false,
	}
	for s, want := range cases {
		if got := publicIP(net.ParseIP(s)); got != want {
			t.Errorf("%s: got %v, want %v", s, got, want)
		}
	}
	if publicIP(nil) {
		t.Error("an unreadable address counts as public")
	}
}

func TestSafeName(t *testing.T) {
	for _, s := range []string{"..", "...", " . ", ""} {
		if got := safeName(s); got != "Episode" {
			t.Errorf("safeName(%q) = %q", s, got)
		}
	}
	if got := safeName(strings.Repeat("é", 100)); !utf8.ValidString(got) || len(got) > 120 {
		t.Errorf("cut inside a character: %q", got)
	}
}

func TestSafeClientRefusesLocal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testFeed)) }))
	defer srv.Close()
	if _, err := FetchFeed(context.Background(), srv.URL); err == nil {
		t.Fatal("a feed on this machine was fetched")
	}
	if _, err := FetchFeed(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("a file URL was accepted")
	}
}
