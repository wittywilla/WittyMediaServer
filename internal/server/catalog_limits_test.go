package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const noLibraryForYou = `{"maxAge":-1,"allowUnrated":true,"libraries":[999]}`

// Liked before a limit came, or in a library out of reach: still not to be named.
func TestLikedSongsKeepToWhatAnAccountMaySee(t *testing.T) {
	f := newMusicFixture(t)
	u, tok := f.user(t, "limited", "adult", false, noLibraryForYou)
	if _, err := f.DB.ExecContext(context.Background(), `INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES(?,?,1,1)`, u.ID, f.song.ID); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	w := f.do(t, "GET", "/api/music/likes", tok, nil)
	if err := json.Unmarshal(w.Body.Bytes(), &ids); err != nil {
		t.Fatalf("%v %s", err, w.Body)
	}
	if len(ids) != 0 {
		t.Errorf("a limited account was told about %v", ids)
	}
	if _, err := f.DB.ExecContext(context.Background(), `INSERT INTO item_reactions(user_id, item_id, value, created_at) VALUES((SELECT id FROM users WHERE name = 'adult'),?,1,1)`, f.song.ID); err != nil {
		t.Fatal(err)
	}
	if w := f.do(t, "GET", "/api/music/likes", f.adult, nil); !strings.Contains(w.Body.String(), fmt.Sprint(f.song.ID)) {
		t.Errorf("an account with no limits: %s", w.Body)
	}
}
