package auth

import (
	"context"
	"strings"
	"testing"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

func namesService(t *testing.T) *Service {
	t.Helper()
	d, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return &Service{DB: d}
}

func TestDisplayNameIsSeparateAndFree(t *testing.T) {
	s, ctx := namesService(t), context.Background()
	a, err := s.CreateProfile(ctx, "sam", "a-long-password", KindAdult, false, "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateProfile(ctx, "alex", "a-long-password", KindAdult, false, "")
	if err != nil {
		t.Fatal(err)
	}
	// Two people may share a profile name, and neither signs in with it.
	for _, id := range []int64{a.ID, b.ID} {
		if err := s.SetDisplayName(ctx, id, "  Dad   Bear "); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.User(ctx, b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Dad Bear" || got.Name != "alex" {
		t.Fatalf("got %q / %q", got.DisplayName, got.Name)
	}
	if _, _, err := s.Login(ctx, "Dad Bear", "a-long-password", "test", false); err == nil {
		t.Fatal("signed in with a profile name")
	}
	if _, _, err := s.Login(ctx, "alex", "a-long-password", "test", false); err != nil {
		t.Fatalf("signing in with the username: %v", err)
	}
	if err := s.SetShowUsername(ctx, a.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.User(ctx, a.ID); !got.ShowUsername {
		t.Fatal("the choice wasn't kept")
	}
}

func TestDisplayNameIsCleaned(t *testing.T) {
	for in, want := range map[string]string{
		"a‮b":                         "ab",
		"⁦x⁩":                    "x",
		"line\nbreak":                      "linebreak",
		"tab\there":                        "tabhere",
		"👨‍👩‍👧":                  "👨‍👩‍👧", // emoji joiners stay
		"<b>kept as text</b>":              "<b>kept as text</b>",
		strings.Repeat("é", 70):            strings.Repeat("é", MaxDisplayName),
	} {
		if got := cleanDisplayName(in); got != want {
			t.Errorf("%q: got %q", in, got)
		}
	}
}

func TestUsernamesStayUnique(t *testing.T) {
	s, ctx := namesService(t), context.Background()
	a, _ := s.CreateProfile(ctx, "Sam", "a-long-password", KindAdult, false, "")
	b, err := s.CreateProfile(ctx, "alex", "a-long-password", KindAdult, false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateProfile(ctx, "sam", "a-long-password", KindAdult, false, ""); err == nil {
		t.Fatal("made a second sam, differing only in case")
	}
	if err := s.SetUsername(ctx, b.ID, "SAM"); err == nil {
		t.Fatal("renamed onto a name in use")
	}
	if err := s.SetUsername(ctx, a.ID, "sam"); err != nil {
		t.Fatalf("changing the case of your own name: %v", err)
	}
	for _, bad := range []string{"", "   ", strings.Repeat("x", 41), "sa​m", "a\nb", "a‮b"} {
		if err := s.SetUsername(ctx, b.ID, bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if err := s.SetUsername(ctx, b.ID, "jordan"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Login(ctx, "jordan", "a-long-password", "test", false); err != nil {
		t.Fatalf("signing in under the new name: %v", err)
	}
}
