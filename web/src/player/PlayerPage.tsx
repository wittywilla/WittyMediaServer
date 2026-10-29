import { useEffect, useState } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { api, isDisc, type Item } from '../api'
import DvdPlayer from './DvdPlayer'
import VideoPlayer from './VideoPlayer'

export default function PlayerPage() {
  const { id } = useParams()
  const [params] = useSearchParams()
  const [item, setItem] = useState<Item | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<Item>(`/items/${id}`).then(setItem).catch((e) => setErr(e.message)) }, [id])
  if (err) return <div className="center"><div className="error">{err}</div></div>
  if (!item) return <div className="center"><div className="spinner" /></div>
  const startParam = params.get('start')
  const startAt = startParam != null ? Number(startParam) : null
  if (isDisc(item)) {
    const mode = params.get('mode') === 'menu' ? 'menu' : 'title'
    const ttn = params.get('ttn') ? Number(params.get('ttn')) : undefined
    const chapter = params.get('chapter') ? Number(params.get('chapter')) : undefined
    return <DvdPlayer key={`${item.id}-${mode}-${ttn}-${chapter}`} item={item} mode={mode} ttn={ttn} chapter={chapter} startAt={startAt} />
  }
  return <VideoPlayer item={item} startAt={startAt} />
}
