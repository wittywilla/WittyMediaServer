// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Cc, R, mk, secP, secR } from '../model.ts'
import type { SpecChip, SpecPageData, SpecRow, SpecSection } from '../types.ts'

// Panels of media pages: a creator video's chapters and autoplay (canvas "videopage"), a photo's details (canvas
// "photoview") and a book's actions and formats (canvas "bookpage"). The pages keep their picture, player and cover.

export interface ChapterLine { start: number; title?: string }
const clock = (s: number) => { const h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), x = Math.floor(s % 60); return (h ? h + ':' + String(m).padStart(2, '0') : String(m)) + ':' + String(x).padStart(2, '0') }

// Chapters (or a setlist for a long video): a row per part, the time at the end. Ids: ch:<n>.
export function chaptersSpec(chapters: ChapterLine[], long: boolean): SpecPageData {
  const rows: SpecRow[] = chapters.map((c, i) => ({ ...R(c.title || `Part ${i + 1}`, clock(c.start)), id: 'ch:' + i }))
  return mk('videopage', 'Creator video', long ? 'Setlist' : 'Chapters', '', rows.length > 1 ? [secR('', rows)] : [])
}

// Up next's one setting. Id: auto.
export function autoplaySpec(on: boolean): SpecPageData {
  return mk('videopage', 'Creator video', 'Up next', '', [secR('', [{ ...R('Autoplay', on ? 'On' : 'Off', on), id: 'auto' }])])
}

export interface PhotoFacts { takenAt?: number; fileName: string; width?: number; height?: number; size?: string; folder?: string; camera?: string; lens?: string; exposure?: string; place?: string; altitude?: string; taken: string }

// A photo's details: what the file is, when and where it was taken, with what. Id: original (the download).
export function photoDetailsSpec(f: PhotoFacts): SpecPageData {
  const rows: SpecRow[] = [R('Taken', f.taken), R('File', f.fileName, false, [f.width && f.height ? `${f.width} × ${f.height}` : '', f.size || ''].filter(Boolean).join(' · '))]
  if (f.folder) rows.push(R('Folder', f.folder))
  if (f.camera) rows.push(R('Camera', f.camera, false, f.lens))
  if (f.exposure) rows.push(R('Exposure', f.exposure))
  if (f.place) rows.push(R('Place', f.place, false, f.altitude))
  return mk('photoview', 'Photos · Viewer', 'Details', '', [secR('', rows), secP('', [{ ...Cc('Download the original'), id: 'original' }])])
}

export interface BookFacts { readable: boolean; started: boolean; finished: boolean; formatName: string }

// A book's actions: Read (or Continue reading, Read again), Download, and the format it is in. Ids: read, download.
export function bookActionsSpec(b: BookFacts): SpecPageData {
  const chips: SpecChip[] = []
  if (b.readable) chips.push({ ...Cc(b.started ? 'Continue reading' : b.finished ? 'Read again' : 'Read', true), id: 'read' })
  chips.push({ ...Cc('Download'), id: 'download' })
  const sections: SpecSection[] = [secP('', chips)]
  if (b.formatName) sections.push(secR('Format', [R(b.formatName, '', true, b.readable ? undefined : 'This format can’t be read in the browser. Convert it to EPUB or CBZ, or download it.')]))
  return mk('bookpage', 'Book', 'Book', '', sections)
}
