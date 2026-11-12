// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package library

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// How each source's people reach the credits tables (docs/METADATA.md, "Cast and crew").
//
// Whatever reads a title (a scan, a refresh, a match) leaves what each source said on the item
// or show it is filling in, and the credits are written when the title is saved. A source that
// wasn't read this time isn't in the list at all, so what it said before stays; one that was
// read and names nobody is there with an empty list, which clears it.

// pendingCredits are what each source said of a title's people in this pass, by source.
type pendingCredits map[string][]CreditIn

// set records what one source said; nil is "nobody".
func (p *pendingCredits) set(source string, list []CreditIn) {
	if source == "" {
		return
	}
	if *p == nil {
		*p = pendingCredits{}
	}
	if list == nil {
		list = []CreditIn{}
	}
	(*p)[source] = list
}

// onlineCreditSources are the services whose credits go with a title's match.
var onlineCreditSources = []string{CreditsTMDB, CreditsTVDB, CreditsTVMaze}

// matched says a title is now matched to provider: what the other services said was said of
// another match (or of none), so it goes.
func (p *pendingCredits) matched(provider string) {
	keep := creditSourceOf(provider)
	for _, src := range onlineCreditSources {
		if src != keep {
			p.set(src, nil)
		}
	}
}

// creditSourceOf is the credits source of a match's provider; "" for one that gives no people.
func creditSourceOf(provider string) string {
	switch provider {
	case "tmdb", "tmdb-tv":
		return CreditsTMDB
	case "tvdb":
		return CreditsTVDB
	case "tvmaze":
		return CreditsTVMaze
	}
	return ""
}

// savePending writes what was read for a saved title and forgets it, so saving the title again
// doesn't write it twice.
func (s *Store) savePending(ctx context.Context, target string, id int64, p *pendingCredits) error {
	if id == 0 || len(*p) == 0 {
		return nil
	}
	srcs := make([]string, 0, len(*p))
	for src := range *p {
		srcs = append(srcs, src)
	}
	sort.Strings(srcs)
	for _, src := range srcs {
		if err := s.SetCredits(ctx, target, id, src, (*p)[src]); err != nil {
			return err
		}
	}
	*p = nil
	return nil
}

// splitNames splits a field that holds several names, as Kodi writes them ("A / B") and tags
// and other servers do ("A, B", "A; B").
func splitNames(s string) []string {
	f := strings.FieldsFunc(s, func(r rune) bool { return r == '/' || r == ',' || r == ';' || r == '|' })
	out := make([]string, 0, len(f))
	for _, n := range f {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// creditsFromText turns names a source writes in one field into credits of one job.
func creditsFromText(text string, role, dept, job string) []CreditIn {
	var out []CreditIn
	for _, n := range splitNames(text) {
		out = append(out, CreditIn{Name: n, Role: role, Department: dept, Job: job})
	}
	return out
}

// numberID is an id a source sends as a number, as text; 0 is none.
func numberID(n int) string {
	if n <= 0 {
		return ""
	}
	return strconv.Itoa(n)
}

// castOrder gives each of a source's cast its billing order: the source's own when it gives
// one, else the order it lists them in.
func castOrder(list []CreditIn, given []int) {
	any := false
	for _, o := range given {
		if o > 0 {
			any = true
		}
	}
	for i := range list {
		if any && i < len(given) {
			list[i].Order = given[i]
		} else {
			list[i].Order = i
		}
	}
}

// pendingCredits is where what is read of a title's people waits for the title to be saved.
func (t *Title) pendingCredits() *pendingCredits {
	switch {
	case t.Item != nil:
		return &t.Item.credits
	case t.Show != nil:
		return &t.Show.credits
	}
	return nil
}
