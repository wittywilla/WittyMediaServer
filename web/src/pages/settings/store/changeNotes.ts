// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors

// A module's notes for a version are its CHANGELOG section, and those are often written for the
// people who work on it: file names, API paths, test names (#549). What's new shows people only the
// plain part, and the rest folds away under "Developer notes".
//
// A changelog says which part is which with headings (docs/MODULES.md, "What's new, and developer notes"):
//
//   ### What's new
//   - Friends can invite each other into a game.
//   ### Developer notes
//   - `POST /games/online/friends/{user}/invite` …
//
// An entry written before that has no headings. Its bullets mostly start with a bold sentence
// that says what changed in plain words ("**Friends: invites, and batteries.** Someone playing…"),
// so that sentence is the plain part and the whole bullet is a developer note. A bullet with no
// bold start is plain unless it names code (anything in backticks). A note only about the
// README or its pictures is never news.
//
// No i18n import: node tests load this file.

export type SplitNotes = { people: string; developer: string }

const DEV_HEAD = /^(?:#{1,6}\s+|\*\*)\s*(?:developer notes|for developers|developers|technical notes|under the hood)\b/i
const NEWS_HEAD = /^(?:#{1,6}\s+|\*\*)\s*what[’']?s new\b/i
const ANY_HEAD = /^#{1,6}\s+/
const BULLET = /^[-*+]\s+(.*)$/
const BOLD_LEAD = /^\*\*(.+?)\*\*\s*(.*)$/
// About the README, its pictures or other documents only: nothing a person using the module sees.
const DOCS_ONLY = /\breadme\b|\bscreenshots?\b.*\b(retaken|updated|redone)\b|\bdocs?(umentation)?\b.*\b(only|pictures|images)\b/i

const join = (lines: string[]) => lines.join('\n').replace(/\n{3,}/g, '\n\n').trim()

export function splitNotes(text: string | undefined | null): SplitNotes {
  const lines = (text || '').replace(/\r\n/g, '\n').split('\n')
  const devAt = lines.findIndex((l) => DEV_HEAD.test(l.trim()))
  const newsAt = lines.findIndex((l) => NEWS_HEAD.test(l.trim()))
  if (devAt >= 0 || newsAt >= 0) {
    // Written with headings: the "What's new" part (or everything before "Developer notes") is
    // for people, as written; "Developer notes" up to the next heading is for developers.
    const head = (l: string) => ANY_HEAD.test(l.trim()) || DEV_HEAD.test(l.trim()) || NEWS_HEAD.test(l.trim())
    const endOf = (from: number) => { const e = lines.findIndex((l, i) => i > from && head(l)); return e < 0 ? lines.length : e }
    const devEnd = devAt < 0 ? -1 : endOf(devAt)
    const dev = devAt < 0 ? [] : lines.slice(devAt + 1, devEnd)
    const people = newsAt >= 0
      ? lines.slice(newsAt + 1, endOf(newsAt))
      : [...lines.slice(0, devAt), ...lines.slice(devEnd)]
    return { people: join(people), developer: join(dev) }
  }
  // Written before there were headings for it.
  const people: string[] = []
  let technical = false
  for (const raw of lines) {
    const line = raw.trim()
    const item = BULLET.exec(line)
    if (!item) {
      if (!line || ANY_HEAD.test(line)) { people.push(raw); continue }
      if (/`/.test(line) || DOCS_ONLY.test(line)) { technical = true; continue }
      people.push(raw)
      continue
    }
    const lead = BOLD_LEAD.exec(item[1])
    if (lead) {
      if (lead[2].trim()) technical = true
      if (DOCS_ONLY.test(lead[1])) { technical = true; continue }
      people.push('- ' + lead[1].replace(/`/g, '').trim())
    } else if (/`/.test(item[1]) || DOCS_ONLY.test(item[1])) {
      technical = true
    } else {
      people.push(raw)
    }
  }
  return { people: join(people), developer: technical ? join(lines) : '' }
}

// hasNews: the plain part says something (more than headings).
export const hasNews = (s: SplitNotes) => s.people.split('\n').some((l) => l.trim() && !ANY_HEAD.test(l.trim()))
