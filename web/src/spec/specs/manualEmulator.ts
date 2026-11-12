// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { mk, secN } from '../model.ts'
import type { SpecPageData, SpecSection } from '../types.ts'

// "Add <emulator> you installed yourself" (the manual add dialog of canvas page "emustand"): what the server found
// and the field for the program's path. Ids: program.
export interface ManualFacts {
  name: string; detected?: { program: string; version?: string }; dropFolder?: string; notes?: string; programHint?: string
  system: string; programs: string[]; file?: string; id: string
}
const systemName = (s: string) => (s === 'linux' ? 'Linux' : s === 'darwin' ? 'macOS' : s === 'windows' ? 'Windows' : s)

export function manualSpec(f: ManualFacts, program: string, error: string): SpecPageData {
  const s: SpecSection[] = []
  if (f.notes) s.push(secN('', f.notes))
  s.push(f.detected
    ? secN('', `Found: ${f.detected.program}${f.detected.version ? `, version ${f.detected.version}` : ''}. Leave the path empty to add this one.`, true)
    : secN('', `Nothing was found where it’s usually installed${f.dropFolder ? ` or in ${f.dropFolder}` : ''}.`))
  const help = `It runs on ${systemName(f.system)}, as ${f.programs.join(' or ')}.${f.dropFolder ? ` A server in a container sees only its own volumes: put the Linux build (its AppImage, or the unpacked folder) in ${f.dropFolder}, which is on the server’s /config volume, and it’s found by itself.` : ''}${f.file ? ` Without this page, name it in ${f.file} as {"${f.id}": "/path/to/it"}, and start the server again.` : ''}`
  s.push({ kind: 'fields', h: '', items: [{ id: 'program', kind: 'text', label: 'Or its program, by its full path on the server', value: program, placeholder: f.programHint || '', hint: help, error: error || undefined }] })
  return mk('emustand', 'Game · Standalone', `Add ${f.name} you installed yourself`, '', s)
}
