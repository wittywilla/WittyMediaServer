// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useId, useState } from 'react'
import { api } from '../../../api'
import DownloadButton from '../../../components/DownloadButton'
import { Spinner } from '../../../components/common'
import { Notice } from '../../../ui'
import { Row, Section, Switch } from '../ui'
import { fmtSize, scrollKeys, type BundlePreview } from './types'
import { Segmented } from '../../../ui/Choice'
import { t, tn, tx } from '../../../i18n'

const DAYS = [1, 3, 7, 14]
const SAMPLE_LINES = 8

// What each kind of placeholder stands for, in the order it's most likely to matter.
const KINDS: [string, (n: number) => string][] = [
  ['path', (n) => tn('{count} file path', '{count} file paths', n)],
  ['folder', (n) => tn('{count} folder', '{count} folders', n)],
  ['title', (n) => tn('{count} title', '{count} titles', n)],
  ['library', (n) => tn('{count} library name', '{count} library names', n)],
  ['user', (n) => tn('{count} account name', '{count} account names', n)],
  ['ip', (n) => tn('{count} IP address', '{count} IP addresses', n)],
  ['host', (n) => tn('{count} computer or web name', '{count} computer and web names', n)],
  ['mac', (n) => tn('{count} network card address', '{count} network card addresses', n)],
  ['email', (n) => tn('{count} email address', '{count} email addresses', n)],
  ['login', (n) => tn('{count} login in a web address', '{count} logins in web addresses', n)],
  ['device', (n) => tn('{count} device name', '{count} device names', n)],
  ['name', (n) => tn('{count} piece of encoded text', '{count} pieces of encoded text', n)],
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
    <Section title={t('Download logs')} desc={t('One ZIP file for someone helping you: recent logs, crash reports and a summary of this server. It’s made on this server when you ask, and nothing is ever uploaded. You decide who sees it.')}>
      <div className="lg-go">
        <label className="lg-go-toggle">
          <Switch checked={sanitize} onChange={setSanitize} labelledBy={toggleId} />
          <span id={toggleId}><b>{t('Sanitize for sharing')}</b></span>
        </label>
        <DownloadButton className="btn primary" path={`/logs/bundle?${query}`} name={sanitize ? 'wittymediaserver-logs-sanitized.zip' : 'wittymediaserver-logs.zip'}>{t('Download logs as ZIP')}</DownloadButton>
      </div>
      {sanitize
        ? <p className="lg-go-note" role="status">{t('Safe to pass on. Names, addresses, paths and titles are replaced with placeholders such as {example}. The same thing keeps the same number throughout the file, so what happened to it can still be followed.', { example: '[path-3]' })}</p>
        : <Notice tone="warn" title={t('This copy isn’t sanitized')}>{t('It holds your folder and file names, what’s in your libraries, account names and network addresses. Keep it for yourself, or read through it before you pass it on. Passwords, tokens and API keys are still taken out.')}</Notice>}
      <div className="lg-what">
        <div>
          <b>{t('Taken out when sanitized')}</b>
          <ul>
            <li>{t('Passwords, tokens, API keys and logins (these go from every copy)')}</li>
            <li>{t('IP addresses, computer names, web addresses and network card addresses')}</li>
            <li>{t('Account names and email addresses')}</li>
            <li>{t('File paths, folders, library names and the titles of what you own')}</li>
          </ul>
        </div>
        <div>
          <b>{t('What stays')}</b>
          <ul>
            <li>{t('What went wrong, and when')}</li>
            <li>{t('Versions, codecs, status codes and timings')}</li>
            <li>{t('The server’s own routes and source lines')}</li>
            <li>{t('File types, such as {example}', { example: '.mkv' })}</li>
          </ul>
        </div>
      </div>
      <Row label={t('How far back')}>
        <Segmented label={t('How far back')} value={String(days)} onChange={(d) => setDays(Number(d) as typeof days)} options={DAYS.map((d) => ({ value: String(d), label: tn('{count} day', '{count} days', d) }))} />
      </Row>
      <div className="set-row stack">
        <div className="set-row-label">{t('What will be in it')}</div>
        {err ? <div className="error" role="alert">{err}</div>
          : !preview ? <Spinner />
          : (
            <>
              <ul className="lg-parts" tabIndex={0} aria-label={t('Files in the ZIP')} onKeyDown={scrollKeys}>
                {preview.parts.map((p) => (
                  <li key={p.name}>
                    <div className="lg-file-text"><b>{p.name}</b><span>{tx(p.description)}</span></div>
                    <span className="lg-part-size">{fmtSize(p.size)}</span>
                  </li>
                ))}
              </ul>
              <p className="lg-total">{t('About {size} before compression.', { size: fmtSize(preview.total) })}</p>
              {preview.sanitize && preview.scanned > 0 && (
                <div className="lg-counts">
                  <div className="lg-long-name">{tn('What’s replaced, in the newest line and the summary', 'What’s replaced, in the newest {count} lines and the summary', preview.scanned)}</div>
                  {counted.length > 0
                    ? <ul className="lg-count-list">{counted.map(([k, words]) => <li key={k}>{words(preview.replaced![k])}</li>)}</ul>
                    : <p className="lg-total">{t('Nothing that needed replacing.')}</p>}
                </div>
              )}
              {preview.changed && preview.changed.length > 0 && (
                <details className="lg-changed">
                  <summary>{t('Compare some lines, before and after')}</summary>
                  <p className="lg-total">{t('“Before” is only on this page, and stays on this server. It isn’t in the ZIP.')}</p>
                  {preview.changed.map((c, i) => (
                    <div key={i} className="lg-changed-one">
                      <div className="lg-long-name">{t('Before')}</div>
                      <pre className="lg-pre" tabIndex={0} aria-label={t('Line {n} before', { n: i + 1 })} onKeyDown={scrollKeys}>{c.before}</pre>
                      <div className="lg-long-name">{t('After')}</div>
                      <pre className="lg-pre" tabIndex={0} aria-label={t('Line {n} after', { n: i + 1 })} onKeyDown={scrollKeys}>{c.after}</pre>
                    </div>
                  ))}
                </details>
              )}
              {preview.sample.length > 0 && (
                <>
                  <div className="lg-long-name">{t('A few lines as they will appear')}</div>
                  <pre className="lg-pre" tabIndex={0} aria-label={t('Sample lines')} onKeyDown={scrollKeys}>{preview.sample.slice(0, SAMPLE_LINES).join('\n')}</pre>
                </>
              )}
            </>
          )}
      </div>
    </Section>
  )
}
