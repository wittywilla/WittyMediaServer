// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fill, lookup, pickForm, placeholders, isRtl, templateMatcher, FSI, PDI } from './core.ts'
import { messagesIn } from './extract.ts'
import { UI_LANGUAGES, uiLanguageFor } from '../languages.ts'

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

test('a reason the server put inside its own sentence is translated too', () => {
  const m = templateMatcher({ "This backup can't be used: {why}": 'Diese Sicherung ist nicht nutzbar: {why}', 'The file is damaged.': 'Die Datei ist beschädigt.', 'Disk {name} is full': 'Datenträger {name} ist voll' }, 'de')
  assert.equal(m("This backup can't be used: The file is damaged."), 'Diese Sicherung ist nicht nutzbar: Die Datei ist beschädigt.')
  assert.equal(m("This backup can't be used: Disk D: is full"), 'Diese Sicherung ist nicht nutzbar: Datenträger D: ist voll')
  assert.equal(m("This backup can't be used: something new"), 'Diese Sicherung ist nicht nutzbar: something new')
})

test('a count the server put in its words takes the language’s form for it', () => {
  const ru = { '{count} albums': { one: '{count} альбом', few: '{count} альбома', many: '{count} альбомов', other: '{count} альбома' } }
  const m = templateMatcher({}, 'ru', { '{count} album': '{count} albums' }, ru)
  assert.equal(m('1 album'), '1 альбом')
  assert.equal(m('3 albums'), '3 альбома')
  assert.equal(m('5 albums'), '5 альбомов')
  assert.equal(m('1,234 albums'), '1 234 альбома'.replace(' ', ' '))
  assert.equal(templateMatcher({}, 'ru', { '{count} album': '{count} albums' }, {})('1 album'), undefined, 'nothing to say it with: left as it is')
})

test('in right-to-left languages, filled-in Latin text and numbers with units keep their order', () => {
  assert.equal(fill('فتح {name}', { name: '00-aurora-2019.flac' }, 'ar'), `فتح ${FSI}00-aurora-2019.flac${PDI}`)
  assert.equal(fill('{time} מתוך', { time: '2 hr 10 min' }, 'he'), `${FSI}2 hr 10 min${PDI} מתוך`)
  assert.equal(fill('{name} שלום', { name: 'דנה' }, 'he'), 'דנה שלום', 'right-to-left text needs nothing')
  assert.equal(fill('Open {name}', { name: 'a-1.mkv' }, 'en'), 'Open a-1.mkv', 'left-to-right languages are left alone')
})

test('Portuguese comes as written in Brazil and in Portugal', () => {
  assert.ok(UI_LANGUAGES.includes('pt-BR') && UI_LANGUAGES.includes('pt-PT') && !UI_LANGUAGES.includes('pt'))
  assert.equal(uiLanguageFor('pt'), 'pt-BR')
  assert.equal(uiLanguageFor('pt-BR'), 'pt-BR')
  assert.equal(uiLanguageFor('pt-PT'), 'pt-PT')
  assert.equal(uiLanguageFor('pt_pt'), 'pt-PT')
  assert.equal(uiLanguageFor('pt-AO'), 'pt-PT')
  assert.equal(uiLanguageFor('de-AT'), 'de')
  assert.equal(uiLanguageFor('xx'), '')
})

test('messages are found in the source', () => {
  assert.deepEqual(messagesIn(`t('Sign in') + t("It's {n}", { n }) + tk(\`Movies\`) + tn('{count} film', '{count} films', n) + format('x')`),
    ['Sign in', "It's {n}", 'Movies', '{count} film', '{count} films'])
  assert.deepEqual(messagesIn('t(`Hi ${name}`)'), [])
  assert.deepEqual(messagesIn(`tc('broadcaster', 'Network')`), ['broadcaster|Network'])
})

test('every catalogue is for a known language, and keeps the English’s {names}', () => {
  const dir = new URL('./locales/', import.meta.url)
  const codes = new Set(UI_LANGUAGES)
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

test('a message has one translation per language, whichever part of the app it is in', () => {
  // All of a language's files are merged into one, so two different translations of the same English would show
  // one or the other depending on the order they load in.
  const dir = new URL('./locales/', import.meta.url)
  for (const lang of readdirSync(dir)) {
    const seen = new Map()
    for (const f of readdirSync(new URL(`${lang}/`, dir))) {
      const cat = JSON.parse(readFileSync(new URL(`${lang}/${f}`, dir), 'utf8'))
      for (const [en, tr] of Object.entries(cat)) {
        const v = JSON.stringify(tr)
        const before = seen.get(en)
        assert.ok(!before || before.v === v, `${lang}: “${en}” is “${before?.v}” in ${before?.f} but “${v}” in ${f}`)
        if (!before) seen.set(en, { v, f })
      }
    }
  }
})
