package library

import (
	"context"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/wittywilla/wittymediaserver/internal/db"
)

// For You: the order of a vertical-video feed, worked out here on the server
// from what this account did on this server, and nothing else. There is no
// profile anywhere but in this database, and nothing leaves it.
//
// Each like, comment, full watch and skip adds to or takes from the taste for a
// video's tags, creator and platform, fading with age. A candidate scores its
// taste, plus how new it is, plus a little chance so the feed keeps offering
// something different, minus having been seen. The ranking is a pure function
// (rankFeed) of those numbers and a seed, so one session's pages stay in the
// same order and tests can check it.

type tasteEvent struct {
	Tags     []string
	Creator  int64
	Platform string
	Weight   float64
	At       int64
}

// taste is what an account seems to like.
type taste struct {
	tags      map[string]float64
	creators  map[int64]float64
	platforms map[string]float64
}

// The half-life of what someone did: tastes change.
const tasteHalfLife = 45 * 24 * 3600.0

func buildTaste(events []tasteEvent, now int64) taste {
	t := taste{tags: map[string]float64{}, creators: map[int64]float64{}, platforms: map[string]float64{}}
	for _, e := range events {
		age := float64(now - e.At)
		if age < 0 {
			age = 0
		}
		w := e.Weight * math.Pow(0.5, age/tasteHalfLife)
		for _, tag := range e.Tags {
			t.tags[strings.ToLower(tag)] += w
		}
		if e.Creator != 0 {
			t.creators[e.Creator] += w
		}
		if e.Platform != "" {
			t.platforms[e.Platform] += w * 0.2
		}
	}
	return t
}

// feedCandidate is one video the feed could offer.
type feedCandidate struct {
	ID        int64
	Creator   int64
	CreatorN  string
	Tags      []string
	Platform  string
	Added     int64
	Plays     int
	Completes int
	Skips     int
	LastAt    int64
	Reaction  int
}

type rankedVideo struct {
	feedCandidate
	Score  float64
	Reason string
}

// jitter is a number from 0 to 1 that depends only on the seed and the video.
func jitter(seed, id int64) float64 {
	h := fnv.New64a()
	var b [16]byte
	for i := 0; i < 8; i++ {
		b[i] = byte(seed >> (8 * i))
		b[8+i] = byte(id >> (8 * i))
	}
	_, _ = h.Write(b[:])
	return float64(h.Sum64()>>11) / float64(1<<53)
}

// rankFeed puts candidates in the order to show them. Videos the account
// disliked are left out; those already seen sink, so a feed that has been
// watched to the end starts over rather than stopping.
func rankFeed(c []feedCandidate, t taste, seed, now int64) []rankedVideo {
	out := make([]rankedVideo, 0, len(c))
	for _, v := range c {
		if v.Reaction < 0 {
			continue
		}
		r := rankedVideo{feedCandidate: v}
		bestTag, bestTagScore, tagScore := "", 0.0, 0.0
		for _, tag := range v.Tags {
			w := t.tags[strings.ToLower(tag)]
			tagScore += w
			if w > bestTagScore {
				bestTag, bestTagScore = tag, w
			}
		}
		if n := len(v.Tags); n > 0 {
			tagScore /= math.Sqrt(float64(n)) // many tags must not outweigh one good one
		}
		creatorScore := t.creators[v.Creator] * 0.8
		fresh := 0.0
		if age := float64(now - v.Added); age >= 0 {
			fresh = 1.2 * math.Pow(0.5, age/(14*24*3600))
		}
		r.Score = tagScore + creatorScore + t.platforms[v.Platform] + fresh + 1.5*jitter(seed, v.ID)
		// Seen already: sink by how recently, and by how it went.
		if v.Plays > 0 {
			recent := math.Pow(0.5, float64(now-v.LastAt)/(7*24*3600))
			r.Score -= 3 + 4*recent
			if v.Completes == 0 && v.Skips > 0 {
				r.Score -= 1
			}
		}
		switch {
		case v.Plays == 0 && creatorScore > 1 && creatorScore >= bestTagScore:
			r.Reason = "More from " + v.CreatorN
		case v.Plays == 0 && bestTagScore > 0.5:
			r.Reason = "Because you like #" + bestTag
		case v.Plays == 0 && fresh > 0.6:
			r.Reason = "New on this server"
		case v.Plays > 0:
			r.Reason = "Watch again"
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Score != out[b].Score {
			return out[a].Score > out[b].Score
		}
		return out[a].ID > out[b].ID
	})
	return diversify(out)
}

// diversify keeps one creator from filling the screen: a video of the creator
// just shown waits behind the next one by someone else.
func diversify(in []rankedVideo) []rankedVideo {
	out := make([]rankedVideo, 0, len(in))
	waiting := []rankedVideo{}
	recent := func(creator int64) bool {
		for i := len(out) - 1; i >= 0 && i >= len(out)-2; i-- {
			if out[i].Creator == creator {
				return true
			}
		}
		return false
	}
	for _, v := range in {
		for len(waiting) > 0 && !recent(waiting[0].Creator) {
			out = append(out, waiting[0])
			waiting = waiting[1:]
		}
		if recent(v.Creator) {
			waiting = append(waiting, v)
			continue
		}
		out = append(out, v)
	}
	return append(out, waiting...)
}

// How much each thing someone did says about their taste.
const (
	tasteLike     = 3.0
	tasteDislike  = -4.0
	tasteComment  = 2.0
	tasteComplete = 1.0
	tasteSkip     = -0.75
)

func (s *Store) tasteEvents(ctx context.Context, userID int64) ([]tasteEvent, error) {
	var out []tasteEvent
	add := func(query string, weigh func(a, b float64) float64) error {
		rows, err := s.DB.QueryContext(ctx, query, userID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tags, platform string
			var creator, at int64
			var a, b float64
			if err := rows.Scan(&tags, &creator, &platform, &at, &a, &b); err != nil {
				return err
			}
			if w := weigh(a, b); w != 0 {
				out = append(out, tasteEvent{Tags: splitList(tags), Creator: creator, Platform: platform, Weight: w, At: at})
			}
		}
		return rows.Err()
	}
	if err := add(`SELECT v.tags, IFNULL(v.creator_id, 0), v.platform, r.created_at, r.value, 0 FROM item_reactions r JOIN video_meta v ON v.item_id = r.item_id
		WHERE r.user_id = ? ORDER BY r.created_at DESC LIMIT 1000`, func(a, _ float64) float64 {
		if a > 0 {
			return tasteLike
		}
		return tasteDislike
	}); err != nil {
		return nil, err
	}
	if err := add(`SELECT v.tags, IFNULL(v.creator_id, 0), v.platform, st.last_at, st.completes, st.skips FROM item_stats st JOIN video_meta v ON v.item_id = st.item_id
		WHERE st.user_id = ? ORDER BY st.last_at DESC LIMIT 2000`, func(c, sk float64) float64 { return c*tasteComplete + sk*tasteSkip }); err != nil {
		return nil, err
	}
	if err := add(`SELECT v.tags, IFNULL(v.creator_id, 0), v.platform, c.created_at, 1, 0 FROM item_comments c JOIN video_meta v ON v.item_id = c.item_id
		WHERE c.user_id = ? ORDER BY c.created_at DESC LIMIT 500`, func(_, _ float64) float64 { return tasteComment }); err != nil {
		return nil, err
	}
	return out, nil
}

// maxFeedCandidates bounds the work of one feed page on a large library.
const maxFeedCandidates = 5000

// ForYou returns one page of an account's feed.
func (s *Store) ForYou(ctx context.Context, q VideoQuery, seed int64) ([]VideoCard, bool, error) {
	w := []string{"i.missing = 0", "v.library_id = ?"}
	args := []any{q.UserID, q.UserID, q.LibraryID}
	if q.Access != nil {
		c, a := q.Access.ItemClause()
		w = append(w, c)
		args = append(args, a...)
	}
	if q.Vertical != nil {
		w = append(w, "v.vertical = ?")
		args = append(args, *q.Vertical)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT i.id, IFNULL(v.creator_id, 0), IFNULL(c.name, ''), v.tags, v.platform, i.added_at,
		IFNULL(st.plays, 0), IFNULL(st.completes, 0), IFNULL(st.skips, 0), IFNULL(st.last_at, 0), IFNULL(r.value, 0)
		FROM video_meta v JOIN items i ON i.id = v.item_id LEFT JOIN creators c ON c.id = v.creator_id
		LEFT JOIN item_stats st ON st.item_id = i.id AND st.user_id = ?
		LEFT JOIN item_reactions r ON r.item_id = i.id AND r.user_id = ?
		WHERE `+strings.Join(w, " AND ")+` ORDER BY i.id DESC LIMIT ?`, append(args, maxFeedCandidates)...)
	if err != nil {
		return nil, false, err
	}
	var cands []feedCandidate
	for rows.Next() {
		var c feedCandidate
		var tags string
		if err := rows.Scan(&c.ID, &c.Creator, &c.CreatorN, &tags, &c.Platform, &c.Added, &c.Plays, &c.Completes, &c.Skips, &c.LastAt, &c.Reaction); err != nil {
			rows.Close()
			return nil, false, err
		}
		c.Tags = splitList(tags)
		cands = append(cands, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, false, err
	}
	events, err := s.tasteEvents(ctx, q.UserID)
	if err != nil {
		return nil, false, err
	}
	now := db.Now()
	ranked := rankFeed(cands, buildTaste(events, now), seed, now)
	limit, offset := q.Limit, q.Offset
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	if offset < 0 || offset > len(ranked) {
		offset = len(ranked)
	}
	end := offset + limit
	more := end < len(ranked)
	if end > len(ranked) {
		end = len(ranked)
	}
	page := ranked[offset:end]
	if len(page) == 0 {
		return []VideoCard{}, false, nil
	}
	want := make([]int64, len(page))
	for i, r := range page {
		want[i] = r.ID
	}
	cards, _, err := s.Videos(ctx, VideoQuery{UserID: q.UserID, IDs: want, Limit: len(want)})
	if err != nil {
		return nil, false, err
	}
	byID := make(map[int64]VideoCard, len(cards))
	for _, c := range cards {
		byID[c.ID] = c
	}
	out := make([]VideoCard, 0, len(page))
	for _, r := range page {
		if c, ok := byID[r.ID]; ok {
			c.Reason = r.Reason
			out = append(out, c)
		}
	}
	return out, more, nil
}

// RelatedVideos are what to watch after a video: the rest of its playlist, then
// its creator's, then videos that share its tags, picked by the same taste.
func (s *Store) RelatedVideos(ctx context.Context, q VideoQuery, it *Item, limit int) ([]VideoCard, error) {
	self, err := s.videoMetaOf(ctx, it.ID)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{it.ID: true}
	out := []VideoCard{}
	take := func(q VideoQuery) error {
		rows, _, err := s.Videos(ctx, q)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if !seen[r.ID] && len(out) < limit {
				seen[r.ID] = true
				out = append(out, r)
			}
		}
		return nil
	}
	base := VideoQuery{LibraryID: it.LibraryID, UserID: q.UserID, Access: q.Access, Limit: limit + 1}
	if self.Playlist != "" {
		pq := base
		pq.Playlist, pq.Sort = &self.Playlist, "oldest"
		if err := take(pq); err != nil {
			return nil, err
		}
	}
	for _, tag := range self.Tags {
		if len(out) >= limit/2 {
			break
		}
		tq := base
		tq.Tag = tag
		if err := take(tq); err != nil {
			return nil, err
		}
	}
	if self.CreatorID != 0 {
		cq := base
		cq.CreatorID = self.CreatorID
		if err := take(cq); err != nil {
			return nil, err
		}
	}
	pop := base
	pop.Sort = "popular"
	if err := take(pop); err != nil {
		return nil, err
	}
	return out, nil
}

// feedSeedFor gives a client that has none a seed that changes each day.
func feedSeedFor(userID int64) int64 { return userID*1000003 + time.Now().Unix()/86400 }

// FeedSeed is the seed used when a client doesn't send one.
func FeedSeed(userID int64) int64 { return feedSeedFor(userID) }
