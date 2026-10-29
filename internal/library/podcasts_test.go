package library

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
