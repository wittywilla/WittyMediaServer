import { useEffect, useState } from 'react'
import { api } from '../../../api'
import DownloadButton from '../../../components/DownloadButton'
import { ChooseFolderButton } from '../../../components/FolderPicker'
import { IconFile, IconFolder, IconRefresh } from '../../../icons'
import { toast } from '../../../state'
import { Section } from '../ui'
import { fmtAge, fmtDate, fmtSize, type LogsView } from './types'

// Where the logs are, how long they're kept, and the files themselves.
export default function Files({ view, onView, reload }: { view: LogsView; onView: (v: LogsView) => void; reload: () => void }) {
  const [dirInput, setDirInput] = useState(view.dir)
  useEffect(() => { setDirInput(view.dir) }, [view.dir])
  const setDir = async (dir: string) => {
    try { onView(await api.put<LogsView>('/logs', { dir })); toast('Log folder saved') } catch (e: any) { toast(e.message) }
  }
  const open = async () => { try { await api.post('/logs/open') } catch (e: any) { toast(e.message) } }
  const h = view.health
  return (
    <>
      <Section title="Where the logs are" desc="New lines go to the folder you choose. Files already written stay where they are.">
        <div className="lg-dir">
          <IconFolder />
          <code>{view.dir}</code>
          {view.canOpen && <button type="button" className="btn sm" onClick={open}>Open folder</button>}
        </div>
        <div className="lg-dir-edit">
          <ChooseFolderButton className="btn" label="Choose folder…" onPick={(p) => void setDir(p)} />
          <input className="input" aria-label="Log folder" spellCheck={false} value={dirInput} onChange={(e) => setDirInput(e.target.value)} />
          <button type="button" className="btn" disabled={!dirInput.trim() || dirInput === view.dir} onClick={() => void setDir(dirInput)}>Use this folder</button>
          <button type="button" className="btn ghost" onClick={() => void setDir('')}>Default</button>
        </div>
        <p className="lg-limits">
          Using <b>{fmtSize(h.totalSize)}</b> of {fmtSize(h.maxSize)}. Files older than {h.keepDays} days are deleted, and the oldest go first when the limit is reached.
        </p>
      </Section>
      <Section title="Log files" actions={<button type="button" className="btn sm ghost" onClick={reload}><IconRefresh /> Refresh</button>}>
        {view.files.length === 0
          ? <div className="set-empty"><IconFile size={28} /><b>No log files yet</b><p>The first one appears when the server has something to write.</p></div>
          : (
            <ul className="lg-files">
              {view.files.map((f) => (
                <li key={f.name}>
                  <div className="lg-file-text">
                    <b>{f.name}</b>
                    <span title={fmtDate(f.modified)}>{fmtSize(f.size)} · {fmtAge(f.modified)}</span>
                  </div>
                  <DownloadButton path={`/logs/files/${encodeURIComponent(f.name)}`} name={f.name} label={`Download ${f.name}`}>Download</DownloadButton>
                </li>
              ))}
            </ul>
          )}
      </Section>
    </>
  )
}
