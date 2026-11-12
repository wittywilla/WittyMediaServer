// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secF, secN, secR } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// Install a module from a file (canvas page "modfile"): choosing the archive and what comes with it, then what the
// server found. Field ids: archive, sums, sig (files) and key (text). The dialog around it owns the buttons.
export interface FileChoose { archive: string; sums: string; sig: string; key: string; error: string; uploading: string }
export interface FileFound {
  title: string; refusal?: string; facts: { label: string; value: string }[]; signature: { words: string; tone: string }
  fingerprint?: string; statusNote?: string; permissions: string[]; problems: string[]; overrideNote: boolean; keptUntil?: string
}

export function installChoose(c: FileChoose): SpecPageData {
  const named = (n: string) => (n ? `Chosen: ${n}` : undefined)
  const sections: SpecSection[] = [
    secN('', 'The server reads the file and says what it is before anything is installed. A module is someone’s code running on your server, so only install one from someone you trust.', true),
    secF('The file', [{ id: 'archive', kind: 'file', label: 'The module’s release archive', accept: '.zip,.tar.gz,.tgz,application/zip,application/gzip', disabled: !!c.uploading, hint: named(c.archive) || 'As the module’s publisher released it, at most 512 MB.' }]),
    secF('If you have them', [
      { id: 'sums', kind: 'file', label: 'Checksum list', disabled: !!c.uploading, hint: named(c.sums) || 'Usually SHA256SUMS, from the same release.' },
      { id: 'sig', kind: 'file', label: 'Signature', disabled: !!c.uploading, hint: named(c.sig) || 'The .sig file that signs the checksum list.' },
      { id: 'key', kind: 'text', label: 'The publisher’s public key', value: c.key, placeholder: '44 characters, from wittymodule keygen', autoComplete: 'off', disabled: !!c.uploading },
    ]),
  ]
  if (c.uploading) sections.push(secN('', c.uploading))
  if (c.error) sections.push(secN('', c.error, true))
  return mk('modfile', 'Modules · Other stores', 'Install a module from a file', '', sections)
}

export function installFound(f: FileFound): SpecPageData {
  const sections: SpecSection[] = []
  if (f.refusal) sections.push(secN('', f.refusal, true))
  sections.push(secR('What the server found', f.facts.map((x) => ({ title: x.label, value: x.value }))))
  sections.push(secN('Signature', f.signature.words + (f.fingerprint ? ` Key fingerprint ${f.fingerprint}.` : ''), f.signature.tone !== 'ok'))
  if (f.statusNote && !f.problems.some((p) => p.includes(f.statusNote!))) sections.push(secN('', f.statusNote, true))
  sections.push(f.permissions.length ? secR('What it asks for', f.permissions.map((p) => ({ title: p }))) : secN('What it asks for', 'No special permissions.'))
  if (f.problems.length) sections.push(secR('What’s wrong with it', f.problems.map((p) => ({ title: p }))))
  if (f.overrideNote) sections.push(secN('', 'Installing it needs an override: the next step names each rule it bypasses.'))
  return mk('modfile-found', 'Modules · Other stores', f.title, '', sections, [], f.keptUntil ? `Kept until ${f.keptUntil}, then discarded if it isn’t installed.` : '')
}
