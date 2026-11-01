import { useEffect, useState, useSyncExternalStore } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../../api'
import { announce } from '../../a11y'
import { toast, useApp } from '../../state'
import type { Programme, Reminder } from './tvApi'
import './reminders.css'

// Reminders: "tell me when this starts". The list is one account's, kept on the
// server, and asked for once; after that a single timer waits for the next one,
// so a page left open does nothing until a reminder is due.

const LEAD_SEC = 60

let list: Reminder[] = []
let loadedFor: number | null = null
const listeners = new Set<() => void>()
const emit = () => { list = [...list]; listeners.forEach((l) => l()) }
const subscribe = (l: () => void) => { listeners.add(l); return () => { listeners.delete(l) } }

function has(channel: string, start: number) { return list.some((r) => r.channel === channel && r.start === start) }

async function toggle(channel: string, p: Pick<Programme, 'title' | 'sub' | 'start' | 'end'>) {
  const found = list.find((r) => r.channel === channel && r.start === p.start)
  try {
    if (found) {
      await api.del(`/tv/reminders/${found.id}`)
      list = list.filter((r) => r !== found)
      announce(`Reminder for ${p.title} removed`)
    } else {
      const r = await api.post<Reminder>('/tv/reminders', { channel, title: p.title, sub: p.sub, start: p.start, end: p.end })
      list = [...list, r].sort((a, b) => a.start - b.start)
      toast(`We'll tell you when ${p.title} starts`)
    }
    emit()
  } catch (e) { toast((e as Error).message) }
}

export function useReminders() {
  const current = useSyncExternalStore(subscribe, () => list)
  void current
  return { list: current, has, toggle }
}

// Lives once in the app, on every page.
export function ReminderWatcher() {
  const { user } = useApp()
  const nav = useNavigate()
  const current = useSyncExternalStore(subscribe, () => list)
  const [due, setDue] = useState<Reminder | null>(null)
  const [shown, setShown] = useState(new Set<number>())

  useEffect(() => {
    if (!user || loadedFor === user.id) return
    loadedFor = user.id
    api.get<Reminder[]>('/tv/reminders').then((r) => { list = r || []; emit() }).catch(() => { /* an account without TV has none */ })
  }, [user])

  useEffect(() => {
    const next = current.find((r) => !shown.has(r.id) && r.end > Date.now() / 1000)
    if (!next) return
    const wait = Math.max(0, (next.start - LEAD_SEC) * 1000 - Date.now())
    // Timers can't wait longer than this; it just looks again.
    const t = setTimeout(() => {
      if ((next.start - LEAD_SEC) * 1000 <= Date.now()) {
        setDue(next)
        setShown((s) => new Set(s).add(next.id))
      } else setShown((s) => new Set(s))
    }, Math.min(wait, 0x7fffffff))
    return () => clearTimeout(t)
  }, [current, shown])

  if (!due) return null
  const close = () => {
    setDue(null)
    list = list.filter((r) => r.id !== due.id)
    emit()
    api.del(`/tv/reminders/${due.id}`).catch(() => { /* it goes by itself once the programme ends */ })
  }
  return (
    <div className="tv-remind" role="alert">
      <div><b>{due.title}</b>{due.start > Date.now() / 1000 ? ' is about to start' : ' has started'}{due.sub ? <small> {due.sub}</small> : null}</div>
      <div className="tv-remind-actions">
        <button className="btn sm primary" onClick={() => { close(); nav(`/tv/watch/${due.channel}`) }}>Watch</button>
        <button className="btn sm ghost" onClick={close}>Dismiss</button>
      </div>
    </div>
  )
}
