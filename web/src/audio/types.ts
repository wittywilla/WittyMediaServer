import type { Item } from '../api'
import type { SoundSettings } from './sound'

export type AudioMode = 'music' | 'audiobooks' | 'podcasts' | 'radio'
export type Repeat = 'off' | 'all' | 'one'

// Everything the bar and the full player show and do. It's handed down rather than kept in
// a context, because it changes with the time: only these two should redraw that often.
export type PlayerView = {
  queue: Item[]
  index: number
  current: Item | null
  mode: AudioMode
  playing: boolean
  time: number
  duration: number
  shuffle: boolean
  repeat: Repeat
  rate: number
  maxSpeed: number
  sleepAt: number | 'chapter' | null
  retrying: boolean
  stationNow: string
  books: boolean
  live: boolean
  cdActive: boolean
  vizHere: boolean
  full: boolean
  fullOpts: { visualizer?: boolean; panel?: 'lyrics' | 'queue' }
  collapsed: boolean
  albumUrl: string
  artistUrl: string
  sound: SoundSettings
  soundNote: string
  liked: boolean
  media: () => HTMLAudioElement | null
  position: () => number
  seek: (t: number) => void
  toggle: () => void
  next: () => void
  prev: () => void
  stop: () => void
  jump: (i: number) => void
  moveInQueue: (from: number, to: number) => void
  removeFromQueue: (i: number) => void
  clearUpNext: () => void
  setShuffle: (on: boolean) => void
  cycleRepeat: () => void
  changeRate: (r: number) => void
  setSleepAt: (v: number | 'chapter' | null) => void
  setSound: (patch: Partial<SoundSettings>) => void
  toggleLike: (id: number) => void
  openFull: (opts?: { visualizer?: boolean; panel?: 'lyrics' | 'queue' }) => void
  closeFull: () => void
  setCollapsed: (v: boolean) => void
  goTowards: (url: string) => void
  startRadio: (it: Item) => void
}
