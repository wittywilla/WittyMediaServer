import { useEffect, useId, useState } from 'react'
import { api } from '../../../api'
import DownloadButton from '../../../components/DownloadButton'
import { Spinner } from '../../../components/common'
import { Notice } from '../../../ui'
import { Row, Section, Switch } from '../ui'
import { fmtSize, scrollKeys, type BundlePreview } from './types'
import { Segmented } from '../../../ui/Choice'

const DAYS = [1, 3, 7, 14]
const SAMPLE_LINES = 8

// What each kind of placeholder stands for, in the order it's most likely to matter.
const KINDS: [string, string, string][] = [
  ['path', 'file path', 'file paths'],
  ['folder', 'folder', 'folders'],
  ['title', 'title', 'titles'],
  ['library', 'library name', 'library names'],
  ['user', 'account name', 'account names'],
  ['ip', 'IP address', 'IP addresses'],
  ['host', 'computer or web name', 'computer and web names'],
  ['mac', 'network card address', 'network card addresses'],
  ['email', 'email address', 'email addresses'],
  ['login', 'login in a web address', 'logins in web addresses'],
  ['device', 'device name', 'device names'],
  ['name', 'piece of encoded text', 'pieces of encoded text'],
]

// The logs in one file for someone helping you, made on this server when it's downloaded.
export default function Bundle() {
  const [days, setDays] = useState(3)
  const [sanitize, setSanitize] = useState(true)
  const [preview, setPreview] = useState<BundlePreview | null>(null)
  const [err, setErr] = useState('')
  const toggleId = useId()
  const query = `days=${days}&sanitize=${sanitize ? 1 : 0}`
  useEffect(() => {
    let gone = false
    setPreview(null); setErr('')
    api.get<BundlePreview>(`/logs/bundle/preview?${query}`).then((p) => { if (!gone) setPreview(p) }).catch((e) => { if (!gone) setErr(e.message) })
    return () => { gone = true }
  }, [query])
  const counted = preview?.replaced ? KINDS.filter(([k]) => preview.replaced![k]) : []
  return (
    <Section title="Download logs" desc="One ZIP file for someone helping you: recent logs, crash reports and a summary of this server. It’s made on this server when you ask, and nothing is ever uploaded. You decide who sees it.">
      <div className="lg-go">
        <label className="lg-go-toggle">
          <Switch checked={sanitize} onChange={setSanitize} labelledBy={toggleId} />
          <span id={toggleId}><b>Sanitize for sharing</b></span>
        </label>
        <DownloadButton className="btn primary" path={`/logs/bundle?${query}`} name={sanitize ? 'wittymediaserver-logs-sanitized.zip' : 'wittymediaserver-logs.zip'}>Download logs as ZIP</DownloadButton>
      </div>
      {sanitize
        ? <p className="lg-go-note" role="status">Safe to pass on. Names, addresses, paths and titles are replaced with placeholders such as <code>[path-3]</code>. The same thing keeps the same number throughout the file, so what happened to it can still be followed.</p>
        : <Notice tone="warn" title="This copy isn’t sanitized">It holds your folder and file names, what’s in your libraries, account names and network addresses. Keep it for yourself, or read through it before you pass it on. Passwords, tokens and API keys are still taken out.</Notice>}
      <div className="lg-what">
        <div>
          <b>Taken out when sanitized</b>
          <ul>
            <li>Passwords, tokens, API keys and logins (these go from every copy)</li>
            <li>IP addresses, computer names, web addresses and network card addresses</li>
            <li>Account names and email addresses</li>
            <li>File paths, folders, library names and the titles of what you own</li>
          </ul>
        </div>
        <div>
          <b>What stays</b>
          <ul>
            <li>What went wrong, and when</li>
            <li>Versions, codecs, status codes and timings</li>
            <li>The server’s own routes and source lines</li>
            <li>File types, such as <code>.mkv</code></li>
          </ul>
        </div>
      </div>
      <Row label="How far back">
        <Segmented label="How far back" value={String(days)} onChange={(d) => setDays(Number(d) as typeof days)} options={DAYS.map((d) => ({ value: String(d), label: `${d} ${d === 1 ? 'day' : 'days'}` }))} />
      </Row>
      <div className="set-row stack">
        <div className="set-row-label">What will be in it</div>
        {err ? <div className="error" role="alert">{err}</div>
          : !preview ? <Spinner />
          : (
            <>
              <ul className="lg-parts" tabIndex={0} aria-label="Files in the ZIP" onKeyDown={scrollKeys}>
                {preview.parts.map((p) => (
                  <li key={p.name}>
                    <div className="lg-file-text"><b>{p.name}</b><span>{p.description}</span></div>
                    <span className="lg-part-size">{fmtSize(p.size)}</span>
                  </li>
                ))}
              </ul>
              <p className="lg-total">About <b>{fmtSize(preview.total)}</b> before compression.</p>
              {preview.sanitize && preview.scanned > 0 && (
                <div className="lg-counts">
                  <div className="lg-long-name">What’s replaced, in the newest {preview.scanned.toLocaleString()} {preview.scanned === 1 ? 'line' : 'lines'} and the summary</div>
                  {counted.length > 0
                    ? <ul className="lg-count-list">{counted.map(([k, one, many]) => <li key={k}><b>{preview.replaced![k].toLocaleString()}</b> {preview.replaced![k] === 1 ? one : many}</li>)}</ul>
                    : <p className="lg-total">Nothing that needed replacing.</p>}
                </div>
              )}
              {preview.changed && preview.changed.length > 0 && (
                <details className="lg-changed">
                  <summary>Compare some lines, before and after</summary>
                  <p className="lg-total">“Before” is only on this page, and stays on this server. It isn’t in the ZIP.</p>
                  {preview.changed.map((c, i) => (
                    <div key={i} className="lg-changed-one">
                      <div className="lg-long-name">Before</div>
                      <pre className="lg-pre" tabIndex={0} aria-label={`Line ${i + 1} before`} onKeyDown={scrollKeys}>{c.before}</pre>
                      <div className="lg-long-name">After</div>
                      <pre className="lg-pre" tabIndex={0} aria-label={`Line ${i + 1} after`} onKeyDown={scrollKeys}>{c.after}</pre>
                    </div>
                  ))}
                </details>
              )}
              {preview.sample.length > 0 && (
                <>
                  <div className="lg-long-name">A few lines as they will appear</div>
                  <pre className="lg-pre" tabIndex={0} aria-label="Sample lines" onKeyDown={scrollKeys}>{preview.sample.slice(0, SAMPLE_LINES).join('\n')}</pre>
                </>
              )}
            </>
          )}
      </div>
    </Section>
  )
}
