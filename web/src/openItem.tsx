import { useNavigate } from 'react-router-dom'
import { type Item } from './api'
import { useAudio, type AudioMode } from './audio/player'
import { IconAudio, IconBook, IconPlay } from './icons'
import { useApp } from './state'

// useOpenItem: the main action for any item, worded for what it is:
// videos and discs "Play"/"Resume", books "Read", audio "Listen".
export function useOpenItem() {
  const nav = useNavigate()
  const audio = useAudio()
  const { libraries } = useApp()
  const started = (it: Item) => !!it.progress && !it.progress.watched && it.progress.positionSec > (it.kind === 'book' ? 0 : 60)
  return {
    label(it: Item): string {
      if (it.kind === 'book') return started(it) ? 'Continue reading' : 'Read'
      if (it.kind === 'audio') return started(it) ? 'Continue listening' : 'Listen'
      if (it.kind === 'game') return 'Play'
      return started(it) ? 'Resume' : 'Play'
    },
    icon(it: Item) {
      return it.kind === 'book' ? <IconBook /> : it.kind === 'audio' ? <IconAudio /> : <IconPlay />
    },
    open(it: Item) {
      if (it.kind === 'book') { nav(['cbz', 'epub', 'pdf'].includes(it.container) ? `/read/${it.id}` : `/item/${it.id}`); return }
      if (it.kind === 'game') { nav(`/game/${it.id}`); return }
      if (it.kind === 'audio') {
        const type = libraries.find((l) => l.id === it.libraryId)?.type
        audio.play([it], 0, (type === 'audiobooks' || type === 'podcasts' ? type : 'music') as AudioMode)
        return
      }
      nav(`/play/${it.id}`)
    },
  }
}
