import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { VideoCard } from '../../gallery'

// Songs tied to their music videos (GET /api/music/links/{id}, docs/CLIENT-API.md).

export type MusicLinkSide = { id: number; libraryId: number; title: string; artist: string; album?: string; runtimeSec: number; updatedAt: number }
export type MusicLink = {
  songId: number; videoId: number; state: 'tied' | 'suggested'; how: 'isrc' | 'recording' | 'name' | 'close' | 'hand'
  syncMs?: number; by?: string; song: MusicLinkSide; video: MusicLinkSide
}
export type MusicLinks = { itemId: number; role: 'song' | 'video' | ''; tied: MusicLink[]; suggested: MusicLink[]; canEdit: boolean }
export type MusicVideoCard = VideoCard & { song?: MusicLinkSide; syncMs?: number }

// Why a suggestion was made, in words.
export const howWords: Record<MusicLink['how'], string> = {
  isrc: 'The same ISRC', recording: 'The same MusicBrainz recording', name: 'The same artist and title', close: 'A nearly matching name', hand: 'Tied by hand',
}

// The links of one song or video, again whenever `reload` changes.
export function useMusicLinks(id: number | undefined, reload = 0) {
  const [links, setLinks] = useState<MusicLinks | null>(null)
  useEffect(() => {
    setLinks(null)
    if (!id) return
    let live = true
    api.get<MusicLinks>(`/music/links/${id}`).then((l) => { if (live) setLinks(l) }).catch(() => { if (live) setLinks(null) })
    return () => { live = false }
  }, [id, reload])
  return links
}

// Where to be in the other one: the song's place in the video is the song's time plus
// the offset. Undefined when they don't line up, or the place is outside the other.
export function placeIn(to: 'video' | 'song', at: number, link: Pick<MusicLink, 'syncMs'>, length: number) {
  if (link.syncMs === undefined || link.syncMs === null) return undefined
  const t = to === 'video' ? at + link.syncMs / 1000 : at - link.syncMs / 1000
  if (t < 1 || (length > 0 && t > length - 2)) return undefined
  return t
}
