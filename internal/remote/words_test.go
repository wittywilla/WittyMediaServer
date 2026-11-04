package remote

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDescribeNamesWhatItIsAbout(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	vpn := Way{Kind: "vpn", Label: "Tailscale", URL: "http://100.101.102.103:9797", State: "waiting"}
	https := Way{Kind: "https", Label: "The internet, over HTTPS", URL: "https://media.example.com", State: "waiting"}
	broken := https
	broken.State, broken.Detail = "problem", "The HTTPS port isn't open."
	seen := vpn
	seen.State, seen.LastSeen = "ok", now.Add(-3*time.Minute)
	cases := []struct {
		name           string
		f              Facts
		level, subject string
		has            []string
	}{
		{"home network only", Facts{}, "good", SubjectHomeOnly, []string{"by your choice", "can connect", "mobile data"}},
		{"chosen, no way in", Facts{On: true}, "waiting", SubjectNotYet, []string{"no way in", "mobile data can't connect"}},
		{"set up, not seen", Facts{On: true, Ways: []Way{https}}, "waiting", SubjectNotYet, []string{"router may not be sending", "Run the check"}},
		{"VPN set up, not seen", Facts{On: true, Ways: []Way{vpn}}, "waiting", SubjectNotYet, []string{"isn't known"}},
		{"in the way", Facts{On: true, Ways: []Way{broken}}, "fault", SubjectNo, []string{"can't connect", "HTTPS port isn't open"}},
		{"seen through the VPN", Facts{On: true, Ways: []Way{seen}}, "good", SubjectYes, []string{"through Tailscale at http://100.101.102.103:9797, with Tailscale switched on", "3 minutes ago"}},
		{"proved by the phone", Facts{On: true, Ways: []Way{https}, Proof: now.Add(-2 * time.Minute)}, "good", SubjectYes, []string{"Checked from outside with your phone, 2 minutes ago"}},
		{"one works, one doesn't", Facts{On: true, Ways: []Way{seen, broken}}, "waiting", SubjectYes, []string{"Another way in has a problem"}},
	}
	for _, c := range cases {
		c.f.Now = now
		s := Describe(c.f)
		if s.Level != c.level || s.Subject != c.subject {
			t.Errorf("%s: %s %q, want %s %q", c.name, s.Level, s.Subject, c.level, c.subject)
		}
		all := strings.Join([]string{s.Subject, s.Summary, s.Home, s.Away, s.Checked}, " | ")
		for _, want := range c.has {
			if !strings.Contains(all, want) {
				t.Errorf("%s: %q doesn't say %q", c.name, all, want)
			}
		}
		for _, line := range []string{s.Subject, s.Summary, s.Home, s.Away, s.Checked} {
			if line == "" {
				t.Errorf("%s: an empty line in %+v", c.name, s)
			}
			// Never a bare on or off.
			for _, bare := range []string{"On", "Off", "on", "off"} {
				if line == bare || strings.HasSuffix(line, ": "+bare) || strings.HasSuffix(line, " · "+bare) {
					t.Errorf("%s: %q is a bare %s", c.name, line, bare)
				}
			}
		}
		if !strings.HasPrefix(s.Home, "Devices at home: ") || !strings.HasPrefix(s.Away, "Devices away from home: ") {
			t.Errorf("%s: each line names its subject: %q, %q", c.name, s.Home, s.Away)
		}
	}
}

// A reverse proxy in front of the HTTPS address is one way in, said once.
func TestDescribeSaysAnAddressOnce(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	https := Way{Kind: "https", URL: "https://media.example.com", State: "ok", LastSeen: now}
	proxy := Way{Kind: "proxy", URL: "https://media.example.com", State: "ok", LastSeen: now}
	if s := Describe(Facts{On: true, Ways: []Way{https, proxy}, Now: now}); strings.Count(s.Summary, "media.example.com") != 1 {
		t.Errorf("%q", s.Summary)
	}
}

func TestAgo(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for d, want := range map[time.Duration]string{30 * time.Second: "just now", 2 * time.Minute: "2 minutes ago", 3 * time.Hour: "3 hours ago", 50 * time.Hour: "2 days ago"} {
		if got := Ago(now.Add(-d), now); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
}

// The help says the same as the page: each subject is in the guide.
func TestTheGuideUsesTheSameWords(t *testing.T) {
	b, err := os.ReadFile("../../docs/REMOTE.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Subjects {
		if !strings.Contains(string(b), s) {
			t.Errorf("docs/REMOTE.md doesn't mention %q: say what it means in the table under \"When it doesn't work\"", s)
		}
	}
}
