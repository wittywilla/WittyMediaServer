import { useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { api, dvdStartMode, getPrefs, isDisc, type Item, type Prefs } from '../api'
import DvdPlayer from './DvdPlayer'
import VideoPlayer from './VideoPlayer'

export default function PlayerPage() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const [item, setItem] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  const [prefs, setPrefs] = useState<Prefs | null>(null)
  useEffect(() => { api.get<Item>(`/items/${id}`).then(setItem).catch((e) => setErr(e.message)) }, [id])
  useEffect(() => { getPrefs().then(setPrefs) }, [])
  if (err) return <div className="center"><div className="error">{err}</div></div>
  if (!item || !prefs) return <div className="center"><div className="spinner" /></div>
  const startParam = params.get('start')
  const startAt = startParam != null ? Number(startParam) : null
  if (isDisc(item)) {
    // No explicit mode: the item's start setting (menu by default).
    const m = params.get('mode')
    const mode = m === 'menu' || m === 'title' ? m : params.get('ttn') ? 'title' : dvdStartMode(item, prefs)
    const ttn = params.get('ttn') ? Number(params.get('ttn')) : undefined
    const chapter = params.get('chapter') ? Number(params.get('chapter')) : undefined
    return <DvdPlayer key={`${item.id}-${mode}-${ttn}-${chapter}`} item={item} mode={mode} ttn={ttn} chapter={chapter} startAt={startAt} insert={params.get('insert') === '1'} />
  }
  return <VideoPlayer key={item.id} item={item} startAt={startAt} />
}
