// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fill, lookup, pickForm, placeholders, isRtl, templateMatcher } from './core.ts'
import { messagesIn } from './extract.ts'
import { LANGUAGES } from '../languages.ts'

test('messages fill in their names, numbers the language’s way', () => {
  assert.equal(fill('Hello, {name}', { name: 'Ana' }, 'en'), 'Hello, Ana')
  assert.equal(fill('{count} films', { count: 1234 }, 'de'), '1.234 films')
  assert.equal(fill('{missing} stays', {}, 'en'), '{missing} stays')
})

test('untranslated text stays English, plural forms follow the language', () => {
  assert.equal(lookup({}, 'Sign in'), 'Sign in')
  assert.equal(lookup({ 'Sign in': 'Anmelden' }, 'Sign in'), 'Anmelden')
  const ru = { one: '{count} фильм', few: '{count} фильма', many: '{count} фильмов', other: '{count} фильма' }
  assert.equal(pickForm(ru, 1, 'ru', '', ''), '{count} фильм')
  assert.equal(pickForm(ru, 3, 'ru', '', ''), '{count} фильма')
  assert.equal(pickForm(ru, 5, 'ru', '', ''), '{count} фильмов')
  assert.equal(pickForm(undefined, 1, 'ru', 'one film', '{count} films'), 'one film')
  assert.ok(isRtl('ar') && isRtl('he') && !isRtl('en'))
})

test('text the server filled in is matched by its pattern', () => {
  const m = templateMatcher({ 'Welcome, {user}': 'Bienvenue, {user}', 'New in {each}': 'Nouveau dans {each}', '{kind} {number}': '{kind} n° {number}', 'Sign in': 'Connexion' }, 'fr')
  assert.equal(m('Welcome, Ana'), 'Bienvenue, Ana')
  assert.equal(m('New in Movies'), 'Nouveau dans Movies')
  assert.equal(m('Movies 2'), undefined, 'a key with almost no words of its own matches nothing')
  assert.equal(m('Sign in'), undefined, 'keys without names are for exact lookups')
  assert.equal(m('Something else'), undefined)
})

test('messages are found in the source', () => {
  assert.deepEqual(messagesIn(`t('Sign in') + t("It's {n}", { n }) + tk(\`Movies\`) + tn('{count} film', '{count} films', n) + format('x')`),
    ['Sign in', "It's {n}", 'Movies', '{count} film', '{count} films'])
  assert.deepEqual(messagesIn('t(`Hi ${name}`)'), [])
})

test('every catalogue is for a known language, and keeps the English’s {names}', () => {
  const dir = new URL('./locales/', import.meta.url)
  const codes = new Set(LANGUAGES.map(([c]) => c))
  for (const lang of readdirSync(dir)) {
    assert.ok(codes.has(lang) && lang !== 'en', `locales/${lang} isn't a language the app offers`)
    for (const f of readdirSync(new URL(`${lang}/`, dir))) {
      const cat = JSON.parse(readFileSync(new URL(`${lang}/${f}`, dir), 'utf8'))
      for (const [en, tr] of Object.entries(cat)) {
        const want = placeholders(en).filter((p) => p !== 'count')
        for (const form of typeof tr === 'string' ? [tr] : Object.values(tr)) {
          const got = placeholders(form).filter((p) => p !== 'count')
          assert.deepEqual(got.filter((p) => !want.includes(p)), [], `${lang}/${f}: “${form}” uses a name “${en}” doesn't have`)
        }
      }
    }
  }
})
