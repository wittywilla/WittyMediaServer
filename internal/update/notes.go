// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

package update

import (
	"context"
	"net/url"
	"strings"
)

// What's new: the card in Settings shows what changed in every version after the one running, in
// the words of CHANGELOG.md, rather than the release page's table of downloads.

// Change is what one version changed, in Markdown.
type Change struct {
	Version string `json:"version"`
	Notes   string `json:"notes"`
}

const (
	maxChangelogBytes = 2 << 20
	maxChangeBytes    = 48 << 10 // one version's notes; 0.6.0-alpha's long list is about this
	maxChangesBytes   = 160 << 10
	maxChanges        = 20
)

// devChangelog replaces the changelog's address in development builds and tests.
var devChangelog string

// changelogURL is CHANGELOG.md as it was at a release's tag, from GitHub's API, which is one of
// the hosts downloads may come from.
func changelogURL(tag string) string {
	if devChangelog != "" {
		return devChangelog + "?ref=" + url.QueryEscape(tag)
	}
	return "https://api.github.com/repos/" + Repo + "/contents/CHANGELOG.md?ref=" + url.QueryEscape(tag)
}

// readChanges finds what changed after the running version up to rel: from the changelog at rel's
// tag, or, when that can't be read, from each release's own notes.
func (u *Updater) readChanges(ctx context.Context, rels []ghRelease, rel *Release) (changes []Change, fromLog bool) {
	if devFeedURL == "" || devChangelog != "" {
		md, err := u.fetchChangelog(ctx, "v"+rel.Version)
		if err == nil {
			if c := changesBetween(md, u.Current, rel.Version); len(c) > 0 {
				return c, true
			}
		} else {
			u.Log.Warn("the changelog could not be read; showing the releases' own notes", "err", err)
		}
	}
	return releaseChanges(rels, u.Current, rel.Version), false
}

func (u *Updater) fetchChangelog(ctx context.Context, tag string) (string, error) {
	res, err := u.get(ctx, changelogURL(tag), "application/vnd.github.raw+json")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := readLimited(res.Body, maxChangelogBytes)
	return string(b), err
}

// changesBetween reads the changelog's "## <version>" sections above from and up to to, newest
// first. Anything else at that level (Unreleased) is left out.
func changesBetween(md, from, to string) []Change {
	var out []Change
	var cur *Change
	var body []string
	end := func() {
		if cur != nil {
			cur.Notes = clip(strings.TrimSpace(strings.Join(body, "\n")), maxChangeBytes)
			if cur.Notes != "" {
				out = append(out, *cur)
			}
		}
		cur, body = nil, nil
	}
	inFence := false
	for _, line := range strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			end()
			ver := strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(line, "## ")), "v")
			if Valid(ver) && Compare(ver, from) > 0 && Compare(ver, to) <= 0 {
				cur = &Change{Version: ver}
			}
			continue
		}
		if cur != nil {
			body = append(body, line)
		}
	}
	end()
	return capChanges(newestFirst(out))
}

// releaseChanges is the same from GitHub's list of releases, for when the changelog can't be read.
func releaseChanges(rels []ghRelease, from, to string) []Change {
	var out []Change
	for _, g := range rels {
		ver := strings.TrimPrefix(g.TagName, "v")
		if g.Draft || !Valid(ver) || Compare(ver, from) <= 0 || Compare(ver, to) > 0 {
			continue
		}
		if notes := clip(releaseSummary(g.Body), maxChangeBytes); notes != "" {
			out = append(out, Change{Version: ver, Notes: notes})
		}
	}
	return capChanges(newestFirst(out))
}

// releaseSummary is what a release's notes say changed. Notes written from 0.7.0 on start with a
// "What's new" section; older ones start with the table of which file to download, which belongs
// on GitHub's page and is left out here, with GitHub's own list of pull requests.
func releaseSummary(body string) string {
	sections := splitSections(strings.ReplaceAll(body, "\r\n", "\n"))
	for _, s := range sections {
		if strings.HasPrefix(strings.ToLower(s.title), "what's new") || strings.HasPrefix(strings.ToLower(s.title), "what’s new") {
			return strings.TrimSpace(s.body)
		}
	}
	var keep []string
	for _, s := range sections {
		switch t := strings.ToLower(s.title); {
		case strings.HasPrefix(t, "which file to download"), strings.HasPrefix(t, "what's changed"), strings.HasPrefix(t, "new contributors"):
			continue
		}
		var lines []string
		for _, l := range strings.Split(s.body, "\n") {
			if !strings.HasPrefix(strings.TrimSpace(l), "**Full Changelog**") {
				lines = append(lines, l)
			}
		}
		text := strings.TrimSpace(strings.Join(lines, "\n"))
		if s.title != "" && text != "" {
			text = "### " + s.title + "\n\n" + text
		}
		if text != "" {
			keep = append(keep, text)
		}
	}
	return strings.TrimSpace(strings.Join(keep, "\n\n"))
}

type section struct{ title, body string }

// splitSections cuts Markdown at its "## " headings; what comes before the first has no title.
func splitSections(md string) []section {
	var out []section
	cur := section{}
	var body []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			cur.body = strings.Join(body, "\n")
			out = append(out, cur)
			cur, body = section{title: strings.TrimSpace(strings.TrimPrefix(line, "## "))}, nil
			continue
		}
		body = append(body, line)
	}
	cur.body = strings.Join(body, "\n")
	return append(out, cur)
}

// clip shortens notes to at most n bytes at the end of a line, saying there's more.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndexByte(s[:n], '\n')
	if cut < n/2 {
		cut = n
		for cut > 0 && !utf8Start(s[cut]) {
			cut--
		}
	}
	return strings.TrimSpace(s[:cut]) + "\n\n*More is on the release page.*"
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

func newestFirst(c []Change) []Change {
	for i := 1; i < len(c); i++ {
		for j := i; j > 0 && Compare(c[j].Version, c[j-1].Version) > 0; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
	return c
}

// capChanges keeps the newest versions within the limits on how many and how much.
func capChanges(c []Change) []Change {
	if len(c) > maxChanges {
		c = c[:maxChanges]
	}
	total := 0
	for i, ch := range c {
		total += len(ch.Notes)
		if total > maxChangesBytes && i > 0 {
			return c[:i]
		}
	}
	return c
}
