// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtDateTime } from '../../api'
import { Spinner } from '../../components/common'
import { IconFolder, IconRefresh } from '../../icons'
import { toast } from '../../state'
import { Button, LinkButton, Notice, Progress } from '../../ui'
import { Section } from './ui'
import { num, t, tk, tn, tx } from '../../i18n'
import './metadatafolder.css'

// Metadata files: one plain text file per title, in a folder of the server's own, to fix
// by hand with any text editor. Nothing changes until it's applied, and what it would
// change is shown first. Everything from a file is shown here as text, never as markup.

type Problem = { file: string; line?: number; field?: string; msg: string; severity: 'error' | 'warning' }
type FieldChange = { field: string; label: string; old: string; new: string; line?: number }
type FileChange = { file: string; target: string; id: number; name: string; fields: FieldChange[] }
type Preview = { dir: string; files: number; unchanged: number; total: number; listed: boolean; changes: FileChange[]; problems: Problem[] }
type Applied = { batch?: string; applied: number; fields: number; skipped: number; changes: FileChange[]; problems: Problem[] }
type ExportResult = { total: number; written: number; unchanged: number; kept: number; problems: Problem[] }
type Job = { running: boolean; what?: string; done: number; total: number; finished?: number; error?: string; result?: ExportResult }
type Info = { path: string; files: number; pictures: number; walkProblems: number; layout: string[]; job: Job }

const POLL_MS = 1200

function ProblemList({ problems }: { problems: Problem[] }) {
  const errors = problems.filter((p) => p.severity === 'error')
  const notes = problems.filter((p) => p.severity !== 'error')
  const group = (title: string, list: Problem[], kind: string) => list.length > 0 && (
    <div className="mf-problems">
      <h3>{title}</h3>
      <ul>
        {list.map((p, i) => (
          <li key={i} className={'mf-problem ' + kind}>
            <span className="mf-word">{kind === 'bad' ? t('Problem') : t('Note')}</span>
            <span className="mf-where">{p.file}{p.line ? <> <b>{t('line {line}', { line: p.line })}</b></> : null}</span>
            <span className="mf-msg">{p.msg}</span>
          </li>
        ))}
      </ul>
    </div>
  )
  return <>{group(t('Fix these, and check again. These files are left out until then.'), errors, 'bad')}{group(t('Worth knowing'), notes, 'note')}</>
}

function ChangeList({ changes, done }: { changes: FileChange[]; done?: boolean }) {
  return (
    <ul className="mf-changes">
      {changes.map((c) => (
        <li key={c.file} className="mf-change">
          <h4>{c.name || c.file}</h4>
          <div className="mf-file">{c.file}</div>
          <dl>
            {c.fields.map((f) => (
              <div key={f.field} className="mf-field">
                <dt>{tx(f.label)}{f.line ? <small> {t('line {line}', { line: f.line })}</small> : null}</dt>
                <dd>
                  <span className="mf-old"><span className="mf-tag">{done ? t('Was') : t('Now')}</span> {f.old === '' ? <i>{t('Nothing')}</i> : f.old}</span>
                  <span className="mf-new"><span className="mf-tag">{done ? t('Now') : t('New')}</span> {f.new === '' ? <i>{t('Nothing')}</i> : f.new}</span>
                </dd>
              </div>
            ))}
          </dl>
        </li>
      ))}
    </ul>
  )
}

export default function MetadataFolder() {
  const [info, setInfo] = useState<Info | null>(null)
  const [err, setErr] = useState('')
  const [preview, setPreview] = useState<Preview | null>(null)
  const [checking, setChecking] = useState(false)
  const [applying, setApplying] = useState(false)
  const [applied, setApplied] = useState<Applied | null>(null)
  const [undone, setUndone] = useState(false)
  const results = useRef<HTMLHeadingElement>(null)
  const load = useCallback(() => api.get<Info>('/metadata-folder').then((v) => { setInfo(v); setErr('') }).catch((e) => setErr(e.message)), [])
  useEffect(() => { void load() }, [load])

  // Asks only while an export runs and someone can see the answer.
  const running = !!info?.job.running
  useEffect(() => {
    if (!running) return
    const tick = () => { if (document.visibilityState === 'visible') void load() }
    const timer = setInterval(tick, POLL_MS)
    document.addEventListener('visibilitychange', tick)
    return () => { clearInterval(timer); document.removeEventListener('visibilitychange', tick) }
  }, [running, load])

  // The answer arrives away from the button that asked: take the keyboard and a screen
  // reader to it.
  useEffect(() => { if (preview || applied) results.current?.focus() }, [preview, applied])

  const startExport = async () => {
    setErr(''); setApplied(null); setPreview(null)
    try { await api.post('/metadata-folder/export', {}); await load() } catch (e: any) { setErr(e.message) }
  }
  const check = async () => {
    setChecking(true); setErr(''); setApplied(null); setUndone(false)
    try {
      setPreview(await api.get<Preview>('/metadata-folder/preview'))
    } catch (e: any) { setErr(e.message) } finally { setChecking(false) }
  }
  const apply = async () => {
    setApplying(true); setErr('')
    try {
      const r = await api.post<Applied>('/metadata-folder/apply', {})
      setApplied(r); setPreview(null); setUndone(false); void load()
    } catch (e: any) { setErr(e.message) } finally { setApplying(false) }
  }
  const undo = async () => {
    if (!applied?.batch) return
    try {
      await api.post('/meta/undo', { batch: applied.batch })
      setUndone(true); toast(t('Those changes are undone'))
    } catch (e: any) { toast(e.message) }
  }

  if (!info) return err ? <div className="error" role="alert">{err}</div> : <Spinner />
  const job = info.job
  const exporting = job.running && job.what === 'export'
  const res = job.result
  const blocking = preview?.problems.some((p) => p.severity === 'error')
  return (
    <>
      <p className="set-row-desc">{t("Fix what the server knows about a film or show with any text editor: one plain file for each title, in a folder of its own. Nothing changes until you've seen what would change and said yes.")}</p>
      {err && <div className="error" role="alert">{err}</div>}

      <Section title={tk('The folder')} desc={t("Export writes a file for each film, show, episode, disc, home video, book and game. It never deletes anything, and never writes over a file you've edited and not applied.")}
        actions={<Button className="mf-touch" variant={info.files ? 'default' : 'primary'} size="sm" icon={<IconFolder />} onClick={startExport} busy={exporting}>{exporting ? t('Exporting…') : info.files ? t('Export again') : t('Export the files')}</Button>}>
        <div className="mf-where-box">
          <span className="mf-label">{t('Where it is on the server')}</span>
          <code className="mf-path">{info.path}</code>
          <p className="mf-counts" role="status">
            {info.files || info.pictures
              ? <>{tn('{count} file for titles', '{count} files for titles', info.files)}{info.pictures ? <>, {tn('{count} picture', '{count} pictures', info.pictures)}</> : null}.</>
              : <>{t('Empty so far. Export to fill it.')}</>}
          </p>
        </div>
        {exporting && (
          <div className="mf-progress">
            <p role="status">{job.total ? t('Writing files: {done} of {total}', { done: num(job.done), total: num(job.total) }) : t('Writing files…')}</p>
            <Progress value={job.total ? job.done : undefined} max={job.total} label={t('How far along the export is')} />
          </div>
        )}
        {!exporting && job.error && <Notice tone="danger" title={t('The export stopped')}>{job.error}</Notice>}
        {!exporting && res && !job.error && (
          <Notice tone="ok" title={t('Exported {when}', { when: job.finished ? fmtDateTime(job.finished) : '' })}>
            {tn('{count} file written', '{count} files written', res.written)}, {t('{count} already up to date.', { count: num(res.unchanged) })}
            {res.kept > 0 && <> {tn('{count} file kept as it is, with edits you haven’t applied.', '{count} files kept as they are, with edits you haven’t applied.', res.kept)}</>}
          </Notice>
        )}
        {!exporting && res && res.problems.length > 0 && <ProblemList problems={res.problems} />}
        <details className="mf-layout">
          <summary>{t('What’s in the folder')}</summary>
          <pre tabIndex={0} aria-label={t("An example of the folder's layout")}>{info.layout.join('\n')}</pre>
          <p>{t('The name of a library is the name of its folder. The number in square brackets is how the server knows which title a file is for. Everything else in the name is only there to help you find it.')}</p>
        </details>
      </Section>

      <Section title={tk('Check and apply')} desc={t('After editing some files, check them. You’ll see each thing that would change, or the file and line of anything that’s wrong. Files you haven’t touched change nothing.')}
        actions={<Button className="mf-touch" variant="primary" size="sm" icon={<IconRefresh />} onClick={check} busy={checking} disabled={exporting || applying}>{checking ? t('Checking…') : t('Check what would change')}</Button>}>
        <h3 className="mf-results-head" ref={results} tabIndex={-1}>
          {applied ? (undone ? t('Undone') : t('Applied')) : preview ? t('What would change') : t('Nothing checked yet')}
        </h3>
        {!applied && !preview && <p className="mf-quiet">{t('Press')} <b>{t('Check what would change')}</b> {t('when you’ve saved your edits. Checking doesn’t change anything.')}</p>}

        {preview && (
          <div>
            <p role="status" className="mf-summary">
              {preview.total === 0 && !preview.problems.length && t('Nothing would change.') + ' '}
              {preview.total > 0 && tn('{count} title would change.', '{count} titles would change.', preview.total) + ' '}
              {preview.unchanged > 0 && tn('{count} file with nothing to do.', '{count} files with nothing to do.', preview.unchanged)}
            </p>
            <ProblemList problems={preview.problems} />
            <ChangeList changes={preview.changes} />
            {!preview.listed && <p className="mf-quiet">{t('The first {count} are shown here. Applying does them all.', { count: num(preview.changes.length) })}</p>}
            {preview.total > 0 && (
              <div className="mf-apply">
                {blocking && <p className="mf-quiet">{t('The files with problems are left out, and these go ahead.')}</p>}
                <Button variant="primary" onClick={apply} busy={applying}>{applying ? t('Applying…') : tn('Apply to {count} title', 'Apply to {count} titles', preview.total)}</Button>
              </div>
            )}
          </div>
        )}

        {applied && (
          <div>
            {undone
              ? <Notice tone="ok" title={t('Those changes are undone')}>{t('The titles are back as they were. Export again to bring their files back into line.')}</Notice>
              : applied.applied > 0
                ? <Notice tone="ok" title={tn('{count} title changed', '{count} titles changed', applied.applied)} action={applied.batch ? <Button className="mf-touch" size="sm" onClick={undo}>{t('Undo all of it')}</Button> : undefined}>
                    {t('What you changed is locked, so no scan or lookup changes it back. You can also undo it later under')} <Link to="/fix-matches">{t('Fix matches')}</Link>{t(', in History.')}
                  </Notice>
                : <Notice title={t('Nothing changed')}>{t('None of the files had anything to apply.')}</Notice>}
            {applied.skipped > 0 && <p className="mf-quiet">{tn('{count} file left out.', '{count} files left out.', applied.skipped)}</p>}
            <ProblemList problems={applied.problems} />
            {!undone && <ChangeList changes={applied.changes} done />}
          </div>
        )}
      </Section>

      <Section title={tk('How to fix something')} desc={t('You don’t need to know anything about JSON. Change the words between the quotes, keep the quotes and commas, and save.')}>
        <div className="mf-guide">
          <details>
            <summary>{t('A wrong title or year')}</summary>
            <p>{t('Open the title’s file in Notepad, TextEdit or any text editor. Near the top you’ll see:')}</p>
            <pre tabIndex={0} aria-label={t('A file before the fix')}>{'  "title": "Harbour Ligts",\n  "sortTitle": "Harbour Ligts",\n  "year": 1998,'}</pre>
            <p>{t('Change what’s wrong, and save:')}</p>
            <pre tabIndex={0} aria-label={t('The same file after the fix')}>{'  "title": "Harbour Lights",\n  "sortTitle": "Harbour Lights",\n  "year": 2004,'}</pre>
            <p>{t('Text keeps its quotes. A number, like the year, has none. Then press')} <b>{t('Check what would change')}</b>{t(', read it, and press')} <b>{t('Apply')}</b>.</p>
          </details>
          <details>
            <summary>{t('A wrong poster')}</summary>
            <p>{t('Two ways, and either works for')} <code>poster</code>, <code>backdrop</code>, <code>banner</code> {t('and')} <code>logo</code>:</p>
            <p><b>{t('Put a picture beside the file.')}</b> {t('If the file is')} <code>Harbour Lights (2004) [item-12].json</code>{t(', name the picture')} <code>Harbour Lights (2004) [item-12]-poster.jpg</code>. {t('For a show it’s')} <code>poster.jpg</code> {t('beside')} <code>show.json</code>.</p>
            <p><b>{t('Or give a web address')}</b> {t('in the file:')}</p>
            <pre tabIndex={0} aria-label={t('A poster given as a web address')}>{'  "poster": "https://example.com/my-poster.jpg",'}</pre>
            <p>{t('Pictures have to be JPEG, PNG, WebP or GIF, and up to 20 MB. A picture is checked by what’s in it, not what it’s called, and SVG isn’t accepted because it can hold scripts.')}</p>
          </details>
          <details>
            <summary>{t('A wrong genre, or several')}</summary>
            <p>{t('A list goes in square brackets, with a comma between things and none after the last:')}</p>
            <pre tabIndex={0} aria-label={t('A list of genres')}>{'  "genres": ["Drama", "Mystery"],'}</pre>
            <p>{t('To clear something, empty its quotes or its brackets. To leave something as it is, you can delete its line. A title can’t be emptied.')}</p>
          </details>
          <details>
            <summary>{t('It says a file is broken')}</summary>
            <p>{t('Messages give the file and the line. Look at that line and the one before it.')}</p>
            <table className="mf-table">
              <thead><tr><th scope="col">{t('What it says')}</th><th scope="col">{t('What to do')}</th></tr></thead>
              <tbody>
                <tr><td>{t('A comma is missing at the end of the line before this one')}</td><td>{t('Every line but the last needs a comma at its end. Add it.')}</td></tr>
                <tr><td>{t('Something is wrong just before here. Often it’s a comma after the last line')}</td><td>{t('The last line before the closing')} <code>{'}'}</code> {t('mustn’t have a comma. Take it off.')}</td></tr>
                <tr><td>{t('This line has something that isn’t a value')}</td><td>{t('Text needs quotes around it. A number doesn’t.')}</td></tr>
                <tr><td>{t('There’s something a text can’t hold as it is')}</td><td>{t('To put a')} <code>"</code> {t('in text, write')} <code>\"</code>. {t('For a new line, write')} <code>\n</code>.</td></tr>
                <tr><td>{t('"year" has to be a year such as 2001')}</td><td>{t('Write the number with no quotes.')}</td></tr>
                <tr><td>{t('This file was written for…, but this number is now…')}</td><td>{t('The number in the name now means another title. Delete the file and export again.')}</td></tr>
                <tr><td>{t('There’s no title with the number…')}</td><td>{t('That title has gone from the server. Delete the file.')}</td></tr>
              </tbody>
            </table>
            <p>{t('Other files aren’t held up by a broken one. If you’d rather start that one again, delete it and press')} <b>{t('Export again')}</b>.</p>
          </details>
          <details>
            <summary>{t('Taking it back')}</summary>
            <p>{t('Right after applying, press')} <b>{t('Undo all of it')}</b>. {t('Later, open')} <Link to="/fix-matches">{t('Fix matches')}</Link>{t(', choose History, and undo it there. The last 5,000 changes are kept.')}</p>
            <p>{t('What you change here locks itself, so a scan can’t change it back. To let a field follow its source again, take it out of')} <code>"locked"</code> {t('in the file, or use')} <b>{t('Revert')}</b> {t('in a title’s Edit dialog.')}</p>
          </details>
          <details>
            <summary>{t('What’s safe to do here')}</summary>
            <p>{t('Anything. A file can’t change anything until you apply it, and you see what it would change first. Whatever a file holds, it’s read as plain text: HTML, scripts and hidden characters are taken out, and you’re told when that happens. A link in this folder is never followed, and a picture has to really be a picture. If you get in a muddle, delete the whole folder and export again: your titles are in the server’s database, not here.')}</p>
          </details>
          <details>
            <summary>{t('Doing this without the web page')}</summary>
            <p>{t('If the server won’t start, or you’d rather use a terminal, the same things work with the server stopped:')}</p>
            <pre tabIndex={0} aria-label={t('Commands for the terminal')}>{'wittymediaserver metadata export\nwittymediaserver metadata check\nwittymediaserver metadata apply          (says what would change)\nwittymediaserver metadata apply --apply  (changes it)'}</pre>
          </details>
          <p className="mf-more">{t('Worried about a match rather than a typo?')} <LinkButton className="mf-touch" to="/fix-matches" size="sm">{t('Open Fix matches')}</LinkButton></p>
        </div>
      </Section>
    </>
  )
}
