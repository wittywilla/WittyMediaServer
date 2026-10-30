import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { useLook } from './look'
import type { EncryptedHelpPage } from './tvApi'
import './tv.css'

// The page behind "Encrypted: can't be played here". The words come from the
// server, so every client says the same thing.
export default function EncryptedHelp() {
  const look = useLook()
  const [help, setHelp] = useState<EncryptedHelpPage | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<EncryptedHelpPage>('/tv/encrypted-help').then(setHelp).catch((e) => setErr(e.message)) }, [])
  return (
    <div className="ctv ctv-page ctv-list-page ctv-help-page" data-skin={look.colours} data-layout={look.layout}>
      <header className="ctv-list-head">
        <h1>{help?.title || 'Encrypted channels'}</h1>
        <Link className="ctv-btn sm" to="/tv">Back to the guide</Link>
      </header>
      {err && <div className="ctv-err" role="alert">{err}</div>}
      {help && (
        <>
          {help.sections.map((s) => (
            <section key={s.heading}>
              <h2 className="ctv-list-h2">{s.heading}</h2>
              {s.paragraphs?.map((p) => <p key={p}>{p}</p>)}
              {s.bullets && <ul>{s.bullets.map((b) => <li key={b}>{b}</li>)}</ul>}
            </section>
          ))}
          <section>
            <h2 className="ctv-list-h2">Devices checked on {help.checked}</h2>
            <p>{help.devicesIntro}</p>
            <ul className="ctv-list">
              {help.devices.map((d) => (
                <li key={d.name} className="ctv-rec">
                  <div className="ctv-rec-main">
                    <b>{d.name}</b>
                    <span>{d.facts}</span>
                    <span>Source: <a href={d.url} target="_blank" rel="noopener noreferrer nofollow">{d.source}</a></span>
                  </div>
                </li>
              ))}
            </ul>
            <p className="ctv-list-note">{help.note}</p>
          </section>
        </>
      )}
    </div>
  )
}
