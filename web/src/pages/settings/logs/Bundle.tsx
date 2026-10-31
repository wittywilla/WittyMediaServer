import { useEffect, useState } from 'react'
import { api } from '../../../api'
import DownloadButton from '../../../components/DownloadButton'
import { Spinner } from '../../../components/common'
import { Row, Section, Switch } from '../ui'
import { fmtSize, scrollKeys, type BundlePreview } from './types'

const DAYS = [1, 3, 7, 14]
const SAMPLE_LINES = 8

// One zip to attach to a bug report, made on this server when it's downloaded.
export default function Bundle() {
  const [days, setDays] = useState(3)
  const [redact, setRedact] = useState(true)
  const [preview, setPreview] = useState<BundlePreview | null>(null)
  const [err, setErr] = useState('')
  const query = `days=${days}&redact=${redact ? 1 : 0}`
  useEffect(() => {
    let gone = false
    setPreview(null); setErr('')
    api.get<BundlePreview>(`/logs/bundle/preview?${query}`).then((p) => { if (!gone) setPreview(p) }).catch((e) => { if (!gone) setErr(e.message) })
    return () => { gone = true }
  }, [query])
  return (
    <Section title="Support bundle" desc="One file for someone helping you: recent logs, crash reports and a summary of this server. Passwords, tokens and API keys are always taken out.">
      <p className="lg-nothing"><b>Nothing is uploaded.</b> The bundle is made on this server and comes to you as a download. You decide who sees it.</p>
      <Row label="How far back">
        <div className="seg" role="radiogroup" aria-label="How far back">
          {DAYS.map((d) => (
            <button key={d} type="button" role="radio" aria-checked={days === d} className={days === d ? 'active' : ''} onClick={() => setDays(d)}>{d} {d === 1 ? 'day' : 'days'}</button>
          ))}
        </div>
      </Row>
      <Row label="Hide titles and file paths" desc="Best if you’ll share the file. What you watch and how your folders are named are replaced with placeholders.">
        <Switch checked={redact} onChange={setRedact} label="Hide titles and file paths" />
      </Row>
      <div className="set-row stack">
        <div className="set-row-label">What will be in it</div>
        {err ? <div className="error" role="alert">{err}</div>
          : !preview ? <Spinner />
          : (
            <>
              <ul className="lg-parts">
                {preview.parts.map((p) => (
                  <li key={p.name}>
                    <div className="lg-file-text"><b>{p.name}</b><span>{p.description}</span></div>
                    <span className="lg-part-size">{fmtSize(p.size)}</span>
                  </li>
                ))}
              </ul>
              <p className="lg-total">About <b>{fmtSize(preview.total)}</b> before compression.</p>
              {preview.sample.length > 0 && (
                <>
                  <div className="lg-long-name">A few lines as they will appear</div>
                  <pre className="lg-pre" tabIndex={0} aria-label="Sample lines" onKeyDown={scrollKeys}>{preview.sample.slice(0, SAMPLE_LINES).join('\n')}</pre>
                </>
              )}
            </>
          )}
      </div>
      <div className="lg-bundle-go">
        <DownloadButton className="btn primary" path={`/logs/bundle?${query}`} name="wittymediaserver-support.zip">Download bundle</DownloadButton>
      </div>
    </Section>
  )
}
