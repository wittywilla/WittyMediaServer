import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { EmptyState, Spinner } from '../components/common'
import { toast } from '../state'
import '../gallery.css'

// Year in review: this profile's year, added up on this server from its own
// record. Nobody else sees it, and nothing is sent anywhere (docs/REVIEW.md).

type Entry = { id?: number; showId?: number; name: string; sub?: string; seconds: number }
type Review = {
  year: number; seconds: number; items: number; days: number; longestRun: number; busiestDay?: number; busiestSeconds?: number
  byType: Entry[]; byMonth: number[]; topTitles: Entry[]; topShows: Entry[]; topArtists: Entry[]; topCreators: Entry[]; topGenres: Entry[]; years: number[]
}

const TYPE_NAMES: Record<string, string> = { movies: 'Movies', shows: 'TV shows', anime: 'Anime', discs: 'Discs', videos: 'Home videos', music: 'Music', musicvideos: 'Music videos', audiobooks: 'Audiobooks', podcasts: 'Podcasts', channels: 'Creator videos', shorts: 'Vertical videos', radio: 'Radio', photos: 'Photo clips' }
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']

function hours(sec: number) {
  if (sec < 3600) return `${Math.round(sec / 60)} min`
  const h = sec / 3600
  return `${h < 10 ? h.toFixed(1) : Math.round(h).toLocaleString()} h`
}

function dayName(d?: number) {
  if (!d) return ''
  return new Date(Date.UTC(Math.floor(d / 10000), Math.floor(d / 100) % 100 - 1, d % 100)).toLocaleDateString([], { timeZone: 'UTC', day: 'numeric', month: 'long' })
}

function TopList({ title, list, link }: { title: string; list: Entry[]; link?: (e: Entry) => string | null }) {
  if (!list.length) return null
  const max = list[0].seconds || 1
  return (
    <section className="rv-card">
      <h2>{title}</h2>
      <ol className="rv-top">
        {list.map((e) => {
          const to = link?.(e)
          return (
            <li key={e.name + (e.id || e.showId || '')}>
              <span className="rv-top-name">{to ? <Link to={to}>{e.name}</Link> : e.name}</span>
              <span className="rv-top-time">{hours(e.seconds)}</span>
              <span className="rv-bar" aria-hidden="true"><i style={{ width: `${(e.seconds / max) * 100}%` }} /></span>
            </li>
          )
        })}
      </ol>
    </section>
  )
}

export default function ReviewPage() {
  const [params, setParams] = useSearchParams()
  const year = Number(params.get('year')) || new Date().getFullYear()
  const [r, setR] = useState<Review | null>(null)
  const load = () => { api.get<Review>(`/me/review?year=${year}`).then(setR).catch(() => setR(null)) }
  useEffect(load, [year]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!r) return <Spinner />
  const peak = Math.max(1, ...r.byMonth)
  const clear = async () => {
    if (!confirm('Forget what this profile watched and listened to? Your year in review, the numbers behind For You and your history start again from nothing. Your places in videos and books stay.')) return
    try { await api.del('/me/history'); load(); toast('History cleared') } catch (e: any) { toast(e.message) }
  }
  return (
    <div className="rv">
      <div className="toolbar">
        <h1>Your {r.year}</h1>
        {r.years.length > 1 && (
          <select className="select" aria-label="Year" value={year} onChange={(e) => setParams({ year: e.target.value })}>
            {r.years.map((y) => <option key={y} value={y}>{y}</option>)}
          </select>
        )}
        <div style={{ flex: 1 }} />
        <button className="btn sm ghost" onClick={clear}>Clear my history</button>
      </div>
      {r.seconds === 0 ? <EmptyState title={`Nothing yet for ${r.year}`} text="As you watch and listen, this fills up. It's worked out on this server from your profile's own record, and only you can see it." /> : (
        <>
          <section className="rv-hero" aria-label="The year in numbers">
            <div><b>{hours(r.seconds)}</b><span>watching and listening</span></div>
            <div><b>{r.items.toLocaleString()}</b><span>{r.items === 1 ? 'title' : 'titles'}</span></div>
            <div><b>{r.days}</b><span>{r.days === 1 ? 'day' : 'days'}</span></div>
            <div><b>{r.longestRun}</b><span>{r.longestRun === 1 ? 'day' : 'days'} in a row at best</span></div>
          </section>
          {r.busiestDay && <p className="rv-note">Your biggest day was {dayName(r.busiestDay)}, with {hours(r.busiestSeconds || 0)}.</p>}
          <section className="rv-card">
            <h2>Month by month</h2>
            <div className="rv-months" role="img" aria-label={MONTHS.map((m, i) => `${m}: ${hours(r.byMonth[i])}`).join(', ')}>
              {r.byMonth.map((v, i) => <div key={i} className="rv-month"><i style={{ height: `${Math.max(v > 0 ? 3 : 0, (v / peak) * 100)}%` }} /><span>{MONTHS[i]}</span></div>)}
            </div>
          </section>
          <div className="rv-grid">
            <TopList title="What you spent it on" list={r.byType.map((e) => ({ ...e, name: TYPE_NAMES[e.name] || e.name }))} />
            <TopList title="Shows" list={r.topShows} link={(e) => (e.showId ? `/show/${e.showId}` : null)} />
            <TopList title="Movies and videos" list={r.topTitles} link={(e) => (e.id ? `/item/${e.id}` : null)} />
            <TopList title="Artists" list={r.topArtists} />
            <TopList title="Creators" list={r.topCreators} />
            <TopList title="Genres" list={r.topGenres} />
          </div>
        </>
      )}
    </div>
  )
}
