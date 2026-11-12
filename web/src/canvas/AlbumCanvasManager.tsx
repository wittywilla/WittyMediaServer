// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { Field, Select } from '../ui'
import CanvasManager from './CanvasManager'

// An album's page, for an administrator: the canvas of each of its songs. A canvas that
// covers the whole album is a file in the album's folder; what is uploaded here belongs to
// one song, so the song is chosen first.
export default function AlbumCanvasManager({ songs, onClose }: { songs: { id: number; title: string; track?: number | null; runtimeSec: number }[]; onClose: () => void }) {
  const [id, setId] = useState(songs[0].id)
  const song = songs.find((s) => s.id === id) || songs[0]
  return (
    <CanvasManager itemId={song.id} title={song.title} kind="song" runtimeSec={song.runtimeSec} onClose={onClose}
      picker={songs.length > 1 ? (
        <Field label="Song" hint="A canvas.mp4 in the album's folder covers every song without its own.">
          <Select value={String(id)} onChange={(e) => setId(Number(e.target.value))}
            options={songs.map((s) => ({ value: String(s.id), label: s.track ? `${s.track}. ${s.title}` : s.title }))} />
        </Field>
      ) : undefined} />
  )
}
