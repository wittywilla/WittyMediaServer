package server

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// On a channel that counts watch progress, tuning in says when what is on the
// screen ends, so a player counts the programme up to there and not through
// the break after it. Other channels don't say, and their players don't ask
// again at every commercial.
func TestTuneSaysWhenTheStretchEnds(t *testing.T) {
	h, admin, _, s := tvServer(t)
	ctx := context.Background()
	lib, err := s.Store.CreateLibrary(ctx, "Films", "movies", []string{t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	// A ten-second clip, too short to be a programme, for the breaks.
	res, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, added_at, updated_at)
		VALUES(?, 'video', '/m/sting.mkv', 'Sting', 'Sting', 10, 1, 1)`, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	sting, _ := res.LastInsertId()
	for i := 1; i <= 3; i++ {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO items(library_id, kind, path, title, sort_title, runtime_sec, added_at, updated_at)
			VALUES(?, 'video', ?, ?, ?, 5400, 1, 1)`, lib.ID, fmt.Sprintf("/m/%d.mkv", i), fmt.Sprintf("Film %d", i), fmt.Sprintf("Film %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	for _, counts := range []bool{true, false} {
		body := fmt.Sprintf(`{"name":"Films %v","enabled":true,"countProgress":%v,"definition":{"mode":"simple","rules":{"libraries":[%d]},"bumpers":{"rules":[{"id":"b","when":"every","everyMin":10,"source":{"items":[%d]}}]}}}`, counts, counts, lib.ID, sting)
		w := tvCall(h, "POST", "/api/channels", admin, body)
		if w.Code != 200 {
			t.Fatalf("%d %s", w.Code, w.Body)
		}
		var c struct{ ID int64 }
		_ = json.Unmarshal(w.Body.Bytes(), &c)
		w = tvCall(h, "GET", fmt.Sprintf("/api/tv/lib-%d/tune", c.ID), admin, "")
		if w.Code != 200 {
			t.Fatalf("tune: %d %s", w.Code, w.Body)
		}
		var info tuneInfo
		if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
			t.Fatal(err)
		}
		if !counts {
			if info.PartEnd != 0 {
				t.Errorf("a channel that doesn't count progress sent partEnd %d", info.PartEnd)
			}
			continue
		}
		ch, err := s.Channels.Get(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		cur, _, err := s.Channels.Now(ctx, ch, time.Unix(info.ServerTime, 0))
		if err != nil {
			t.Fatal(err)
		}
		// The film has a bumper every 10 minutes: what's on ends with this stretch of
		// it, or this bumper, not with the film.
		if info.PartEnd != cur.End || info.Now == nil || info.PartEnd > info.Now.End || (cur.Offset == 0 && cur.Kind == "show" && info.PartEnd == info.Now.End) {
			t.Errorf("partEnd %d, the stretch ends %d, the programme %+v", info.PartEnd, cur.End, info.Now)
		}
	}
}
