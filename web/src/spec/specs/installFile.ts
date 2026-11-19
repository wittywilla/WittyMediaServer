// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'
import { t } from '../../sdui/words.ts'

// Install a module from a file (canvas page "modfile"): choosing the archive and what comes with it, then what the
// server found. Field ids: archive, sums, sig (files) and key (text). The dialog around it owns the buttons.
export interface FileChoose { archive: string; sums: string; sig: string; key: string; error: string; uploading: string }
export interface FileFound {
  title: string; refusal?: string; facts: { label: string; value: string }[]; signature: { words: string; tone: string }
  fingerprint?: string; statusNote?: string; permissions: string[]; problems: string[]; overrideNote: boolean; keptUntil?: string
}

export function installChoose(c: FileChoose): SpecPageData {
  const named = (n: string) => (n ? t('Chosen: {name}', { name: n }) : undefined)
  const sections: SpecSection[] = [
    secN('', t('The server reads the file and says what it is before anything is installed. A module is someone’s code running on your server, so only install one from someone you trust.'), true),
    secF(t('The file'), [{ id: 'archive', kind: 'file', label: t('The module’s release archive'), accept: '.zip,.tar.gz,.tgz,application/zip,application/gzip', disabled: !!c.uploading, hint: named(c.archive) || t('As the module’s publisher released it, at most 512 MB.') }]),
    secF(t('If you have them'), [
      { id: 'sums', kind: 'file', label: t('Checksum list'), disabled: !!c.uploading, hint: named(c.sums) || t('Usually SHA256SUMS, from the same release.') },
      { id: 'sig', kind: 'file', label: t('Signature'), disabled: !!c.uploading, hint: named(c.sig) || t('The .sig file that signs the checksum list.') },
      { id: 'key', kind: 'text', label: t('The publisher’s public key'), value: c.key, placeholder: t('44 characters, from wittymodule keygen'), autoComplete: 'off', disabled: !!c.uploading },
    ]),
  ]
  if (c.uploading) sections.push(secN('', c.uploading))
  if (c.error) sections.push(secN('', c.error, true))
  return mk('modfile', t('Modules · Other stores'), t('Install a module from a file'), '', sections)
}

export function installFound(f: FileFound): SpecPageData {
  const sections: SpecSection[] = []
  if (f.refusal) sections.push(secN('', f.refusal, true))
  sections.push(secR(t('What the server found'), f.facts.map((x) => ({ title: x.label, value: x.value }))))
  sections.push(secN(t('Signature'), f.signature.words + (f.fingerprint ? ' ' + t('Key fingerprint {fingerprint}.', { fingerprint: f.fingerprint }) : ''), f.signature.tone !== 'ok'))
  if (f.statusNote && !f.problems.some((p) => p.includes(f.statusNote!))) sections.push(secN('', f.statusNote, true))
  sections.push(f.permissions.length ? secR(t('What it asks for'), f.permissions.map((p) => ({ title: p }))) : secN(t('What it asks for'), t('No special permissions.')))
  if (f.problems.length) sections.push(secR(t('What’s wrong with it'), f.problems.map((p) => ({ title: p }))))
  if (f.overrideNote) sections.push(secN('', t('Installing it needs an override: the next step names each rule it bypasses.')))
  return mk('modfile-found', t('Modules · Other stores'), f.title, '', sections, [], f.keptUntil ? t('Kept until {time}, then discarded if it isn’t installed.', { time: f.keptUntil }) : '')
}
