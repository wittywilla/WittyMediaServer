import { useCallback, useEffect, useId, useMemo, useState, type ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import { useApp } from '../../state'
import { fmtBytes, fmtDuration, taskAction, useActivity, type Task } from '../../activity'
import { IconCheck, IconInfo, IconPause, IconPlay, IconRefresh, IconStop, IconTrash } from '../../icons'
import { Button, Chip, Chips, EmptyState, Field, Input, LoadError, Notice, Progress, Segmented, Select, Spinner, ask, toast } from '../../ui'
import { PageHead, Row, SaveBar, Section, Switch } from './ui'
import './librarytranscoding.css'
import TdarrSection from './TdarrSection'
import type { Library, Options, Policy, Run, Settings, Step, FileRow, Preview, Sample } from './libraryTranscodingTypes'

// Library transcoding: converting a library's files ahead of time. Everything that could
// lose something is said before it happens: a preset says what it does, a preview says what
// would become of one file, and a dry run says what would become of them all.

const LANGUAGES: [string, string][] = [
  ['en', 'English'], ['es', 'Spanish'], ['fr', 'French'], ['de', 'German'], ['it', 'Italian'], ['pt', 'Portuguese'], ['nl', 'Dutch'],
  ['sv', 'Swedish'], ['da', 'Danish'], ['no', 'Norwegian'], ['fi', 'Finnish'], ['pl', 'Polish'], ['cs', 'Czech'], ['hu', 'Hungarian'], ['el', 'Greek'], ['tr', 'Turkish'],
  ['ru', 'Russian'], ['uk', 'Ukrainian'], ['ja', 'Japanese'], ['ko', 'Korean'], ['zh', 'Chinese'], ['hi', 'Hindi'], ['ar', 'Arabic'], ['he', 'Hebrew'], ['th', 'Thai'], ['vi', 'Vietnamese'],
]
const languageName = (code: string) => LANGUAGES.find(([c]) => c === code)?.[1] ?? code.toUpperCase()

const OUTPUTS: { value: string; title: string; text: string }[] = [
  { value: 'keep', title: 'Keep the original', text: 'The new file takes over and the old one moves to a hidden folder beside it, where you can put it back or delete it later. The safest choice, and it needs the most room.' },
  { value: 'replace', title: 'Replace the original', text: 'Once the new file has been checked, the old one is deleted for good. This frees the most room, and it can’t be undone.' },
  { value: 'beside', title: 'Write it beside the original', text: 'The old file stays in use and a copy is written next to it. Nothing changes for viewers until you choose to use the copy.' },
]

export default function LibraryTranscoding() {
  const [options, setOptions] = useState<Options | null>(null)
  const [libs, setLibs] = useState<Library[] | null>(null)
  const [settings, setSettings] = useState<Settings | null>(null)
  const [failed, setFailed] = useState(false)
  const load = useCallback(() => {
    setFailed(false)
    Promise.all([api.get<Options>('/libtranscode/options'), api.get<Library[]>('/libtranscode/libraries'), api.get<Settings>('/libtranscode/settings')])
      .then(([o, l, s]) => { setOptions(o); setLibs(l); setSettings(s) }).catch(() => setFailed(true))
  }, [])
  useEffect(load, [load])
  const head = <PageHead title="Library transcoding" desc={<>Converts the files in a library ahead of time, so they play without being converted later. It changes your files, so it starts off. Converting while someone watches is a separate thing, under <Link to="/settings/transcoding">Live transcoding</Link>. <a href="https://github.com/wittywilla/WittyMediaServer/blob/master/docs/LIBRARY-TRANSCODING.md" target="_blank" rel="noreferrer">The guide</a> explains it all.</>} />
  if (failed) return <>{head}<LoadError what="This page" onRetry={load} /></>
  if (!options || !libs || !settings) return <>{head}<Spinner /></>
  return (
    <>
      {head}
      {!settings.enabled && <TurnOn settings={settings} onChange={(s) => { setSettings(s); load() }} />}
      {settings.enabled && <RunSettings settings={settings} options={options} onChange={setSettings} />}
      {libs.length === 0
        ? <Section title="Your libraries"><EmptyState title="No video libraries yet" text="Add a library of films or shows, and it will be here." /></Section>
        : <Libraries libs={libs} setLibs={setLibs} options={options} settings={settings} />}
      <TdarrSection libs={libs} />
    </>
  )
}

// The warning, read before it can be turned on.
function TurnOn({ settings, onChange }: { settings: Settings; onChange: (s: Settings) => void }) {
  const [ok, setOk] = useState(false)
  const [busy, setBusy] = useState(false)
  const check = useId()
  const on = async () => {
    setBusy(true)
    try { onChange(await api.put<Settings>('/libtranscode/settings', { ...settings, enabled: true, acknowledged: true })) } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <Section title="Before you turn this on" desc="Trying a file and checking a whole library, below, work without it, and change nothing.">
      <Notice tone="warn" title="This can get in the way of people watching.">
        <ul className="ltc-list">
          <li><b>Processor.</b> Converting a film can keep every core busy for hours. Anyone watching at the same time may see stutters, or a film that takes longer to start, especially if their device needs the server to convert while playing.</li>
          <li><b>Disk.</b> Each file needs room for a second copy while it converts, and reading it slows down anything else using that drive.</li>
          <li><b>Your files.</b> This rewrites them. Each new file is checked before anything is replaced, and you choose whether the old one is kept, but a converted file is never better than its original.</li>
        </ul>
        <p>Out of the box it does one file at a time, on half the processor, at low priority, and it stops whenever someone is playing something. You can change all of that once it’s on.</p>
      </Notice>
      <div className="ltc-accept">
        <input id={check} type="checkbox" checked={ok} onChange={(e) => setOk(e.target.checked)} />
        <label htmlFor={check}>I understand how this affects streaming and my files.</label>
      </div>
      <p className="ltc-actions"><Button variant="primary" disabled={!ok} busy={busy} onClick={on}>Turn on library transcoding</Button></p>
    </Section>
  )
}

function RunSettings({ settings, options, onChange }: { settings: Settings; options: Options; onChange: (s: Settings) => void }) {
  const [draft, setDraft] = useState(settings)
  const [busy, setBusy] = useState(false)
  useEffect(() => setDraft(settings), [settings])
  const dirty = JSON.stringify(draft) !== JSON.stringify(settings)
  const set = (patch: Partial<Settings>) => setDraft((d) => ({ ...d, ...patch }))
  const save = async () => {
    setBusy(true)
    try { onChange(await api.put<Settings>('/libtranscode/settings', draft)); toast('Saved. This applies straight away.') } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const off = async () => {
    if (!await ask({ title: 'Turn off library transcoding?', body: 'Anything converting now carries on to the end or until you cancel it. Nothing new can start. Kept originals stay where they are.', confirm: 'Turn off' })) return
    try { onChange(await api.put<Settings>('/libtranscode/settings', { ...settings, enabled: false })) } catch (e) { toast((e as Error).message) }
  }
  const cores = options.capabilities.cores
  const threadChoices = [
    { value: '0', label: `Half of it: ${Math.max(1, Math.floor(cores / 2))} of ${cores} threads (recommended)` },
    { value: String(Math.max(1, Math.floor(cores / 4))), label: `A quarter: ${Math.max(1, Math.floor(cores / 4))} of ${cores} threads` },
    { value: String(Math.max(1, Math.floor((cores * 3) / 4))), label: `Three quarters: ${Math.max(1, Math.floor((cores * 3) / 4))} of ${cores} threads` },
    { value: String(cores), label: `All of it: ${cores} threads (people watching will notice)` },
  ].filter((o, i, list) => list.findIndex((x) => x.value === o.value) === i)
  if (!threadChoices.some((o) => o.value === String(draft.threads))) threadChoices.push({ value: String(draft.threads), label: `${draft.threads} threads` })
  return (
    <Section title="How it runs on this server" desc="Converting competes with people watching. These keep it out of their way." actions={<Button variant="ghost" size="sm" onClick={off}>Turn off</Button>}>
      <Row label="Pause while people are streaming" desc="Conversions hold still, using no processor, whenever anyone is playing something or a game, and carry on where they were afterwards. A library may take much longer to finish.">
        <Switch label="Pause while people are streaming" checked={draft.pauseWhileStreaming} onChange={(v) => set({ pauseWhileStreaming: v })} />
      </Row>
      <Row label="Low priority" desc="Everything else on this machine gets the processor first.">
        <Switch label="Low priority" checked={draft.lowPriority} onChange={(v) => set({ lowPriority: v })} />
      </Row>
      <Row label="Only at certain hours" desc="Outside these hours it holds still. Hours are on this server’s clock, and can run past midnight.">
        <Switch label="Only at certain hours" checked={draft.hoursOn} onChange={(v) => set({ hoursOn: v })} />
      </Row>
      {draft.hoursOn && (
        <Row label="From and until" dim={!draft.hoursOn}>
          <HourSelect label="From" value={draft.hoursFrom} onChange={(v) => set({ hoursFrom: v })} />
          <HourSelect label="Until" value={draft.hoursTo} onChange={(v) => set({ hoursTo: v })} />
        </Row>
      )}
      <Row label="Wait for new files to settle" desc="For libraries that convert new files by themselves: how long a file has to stay the same size before it’s touched, so one still being copied or downloaded is left to finish.">
        <Select value={String(draft.settleSeconds)} options={[...new Set([60, 120, 300, 900, 3600, draft.settleSeconds])].sort((a, b) => a - b).map((n) => ({ value: String(n), label: n < 3600 ? `${Math.round(n / 60)} min` : `${Math.round(n / 3600)} hour` }))} onChange={(e) => set({ settleSeconds: Number(e.target.value) })} />
      </Row>
      <Row label="How much of the processor" desc="For each file being converted.">
        <Select value={String(draft.threads)} options={threadChoices} onChange={(e) => set({ threads: Number(e.target.value) })} />
      </Row>
      <Row label="Files at once" desc="More finishes sooner and leaves less for everything else. One is right for most machines.">
        <Select value={String(draft.maxJobs)} options={[1, 2, 3, 4].map((n) => ({ value: String(n), label: String(n) }))} onChange={(e) => set({ maxJobs: Number(e.target.value) })} />
      </Row>
      <Row label="Use hardware encoders" desc={<>Much faster and easier on the processor, for slightly larger files. It uses the ones ticked under <Link to="/settings/transcoding">Live transcoding</Link>, for ordinary 8-bit pictures. HDR and 10-bit pictures always use the slower software encoder, which can be trusted to keep them right. {options.capabilities.hardware.length ? `Working here: ${options.capabilities.hardware.join(', ')}.` : 'None is available or ticked here.'}</>}>
        <Switch label="Use hardware encoders" checked={draft.useHardware} onChange={(v) => set({ useHardware: v })} />
      </Row>
      <Row label="Keep this much free on each drive" desc="Conversions stop short of this, and a file that wouldn’t fit waits.">
        <Select value={String(draft.minFreeGb)} options={[...new Set([1, 5, 10, 25, 50, 100, draft.minFreeGb])].sort((a, b) => a - b).map((n) => ({ value: String(n), label: `${n} GB` }))} onChange={(e) => set({ minFreeGb: Number(e.target.value) })} />
      </Row>
      <SaveBar dirty={dirty} busy={busy} save={save} discard={() => setDraft(settings)} />
    </Section>
  )
}

function HourSelect({ label, value, onChange }: { label: string; value: number; onChange: (v: number) => void }) {
  return (
    <label className="ltc-inline">{label}
      <Select value={String(value)} onChange={(e) => onChange(Number(e.target.value))} options={Array.from({ length: 24 }, (_, h) => ({ value: String(h), label: `${String(h).padStart(2, '0')}:00` }))} />
    </label>
  )
}

// A library: its recipe, what that would do, and running it.
function Libraries({ libs, setLibs, options, settings }: { libs: Library[]; setLibs: (l: Library[]) => void; options: Options; settings: Settings }) {
  const [id, setId] = useState(libs[0].id)
  const lib = libs.find((l) => l.id === id) ?? libs[0]
  const update = (l: Library) => setLibs(libs.map((x) => (x.id === l.id ? l : x)))
  return (
    <>
      <Section title="Choose a library" desc="Each library has its own recipe.">
        <Row label="Library" desc={`${lib.items.toLocaleString()} items`}>
          <Select value={String(lib.id)} options={libs.map((l) => ({ value: String(l.id), label: l.name }))} onChange={(e) => setId(Number(e.target.value))} />
        </Row>
      </Section>
      <LibraryPanel key={lib.id} lib={lib} onSaved={update} options={options} settings={settings} />
    </>
  )
}

function LibraryPanel({ lib, onSaved, options, settings }: { lib: Library; onSaved: (l: Library) => void; options: Options; settings: Settings }) {
  const [policy, setPolicy] = useState<Policy>(lib.rule.policy)
  const [output, setOutput] = useState(lib.rule.output)
  const [readOnly, setReadOnly] = useState(lib.readOnly)
  const [allow, setAllow] = useState(lib.rule.allowWrites)
  const [auto, setAuto] = useState(lib.rule.auto)
  const [busy, setBusy] = useState(false)
  const [current, setCurrent] = useState('')
  const [code, setCode] = useState('')
  const { user } = useApp()
  const replacing = output === 'replace' && lib.rule.output !== 'replace' // deleting originals asks for your password
  const [ended, setEnded] = useState(0) // runs that have ended, so what they left gets listed
  const dirty = JSON.stringify(policy) !== JSON.stringify(lib.rule.policy) || output !== lib.rule.output || readOnly !== lib.readOnly || allow !== lib.rule.allowWrites || auto !== lib.rule.auto
  const save = async () => {
    setBusy(true)
    try {
      const n = await api.put<Library>(`/libtranscode/libraries/${lib.id}`, { policy, output, readOnly, allowWrites: allow, auto, current, code })
      setCurrent(''); setCode('')
      // What came back is what's saved, tidied by the server, and is what's compared from now on.
      setPolicy(n.rule.policy); setOutput(n.rule.output); setReadOnly(n.readOnly); setAllow(n.rule.allowWrites); setAuto(n.rule.auto)
      onSaved(n); toast('Saved')
    } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  const change = (next: Policy) => setPolicy({ ...next, preset: 'custom' })
  const presetOf = options.presets.find((p) => p.id === policy.preset)
  return (
    <>
      <Section title="What should the files become?" desc="Pick the one that sounds like what you want. The details under each one say exactly what it does.">
        <div className="ltc-cards" role="radiogroup" aria-label="What the files should become">
          {options.presets.map((p) => {
            const chosen = policy.preset === p.id
            return (
              <div key={p.id} className={'ltc-card' + (chosen ? ' on' : '')}>
                <label className="ltc-card-head">
                  <input type="radio" name={`preset-${lib.id}`} checked={chosen} onChange={() => setPolicy(p.policy)} />
                  <span><b>{p.name}</b><span className="ltc-card-text">{p.summary}</span></span>
                </label>
                <details className="ltc-more">
                  <summary>What this does</summary>
                  <ul className="ltc-list">{p.details.map((d) => <li key={d}>{d}</li>)}</ul>
                  <p className="ltc-cost"><b>Cost.</b> {p.cost}</p>
                </details>
              </div>
            )
          })}
        </div>
        {policy.preset === 'custom' && <Notice title="Your own recipe.">This started from a preset, and you’ve changed its details. Choosing a preset above goes back to it.</Notice>}
        {policy.preset === 'save-space' && !options.capabilities.hevc && <Notice tone="warn" title="This server’s FFmpeg can’t encode H.265.">Save space falls back on H.264 at a stronger setting, which saves less. Choose a different FFmpeg with the WMS_FFMPEG setting if you want H.265.</Notice>}
        {presetOf === undefined && policy.preset !== 'custom' && <Notice tone="warn">This recipe’s preset no longer exists.</Notice>}
        <RecipeDetails policy={policy} change={change} options={options} />
      </Section>

      <Section title="What happens to the old file?">
        <div className="ltc-cards" role="radiogroup" aria-label="What happens to the old file">
          {OUTPUTS.map((o) => (
            <label key={o.value} className={'ltc-card ltc-card-head' + (output === o.value ? ' on' : '')}>
              <input type="radio" name={`output-${lib.id}`} checked={output === o.value} onChange={() => setOutput(o.value)} />
              <span><b>{o.title}</b><span className="ltc-card-text">{o.text}</span></span>
            </label>
          ))}
        </div>
        {replacing && (
          <div className="form full">
            <Field label="Your password" hint="Replacing originals deletes them, so this asks for your password."><Input type="password" autoComplete="current-password" value={current} onChange={(e) => setCurrent(e.target.value)} /></Field>
            {user?.twoFactor && <Field label="Code from your authenticator app"><Input inputMode="numeric" autoComplete="one-time-code" value={code} onChange={(e) => setCode(e.target.value)} /></Field>}
          </div>
        )}
        <Row label="This library is read-only" desc="Nothing the server does may change this library’s files: no conversions, and no saved artwork or details beside them. Previews and dry runs still work.">
          <Switch label="This library is read-only" checked={readOnly} onChange={setReadOnly} />
        </Row>
        <Row label="Convert new files by themselves" desc={<>When a scan finds a new or changed file, it waits until the file has stopped changing, then converts it with this recipe, within the limits above, and shows in the activity panel. Files already here when you turn this on are left alone: use Start converting for those. A file that fails or that you put back isn’t tried again unless it changes.{lib.blocked && auto ? <> It can’t run at the moment: {lib.blocked}</> : null}</>}>
          <Switch label="Convert new files by themselves" checked={auto} onChange={setAuto} />
        </Row>
        {!options.saveMeta && (
          <Row label="Allow conversions in this library" desc={<>This server keeps metadata out of media folders (<Link to="/settings/metadata">Save next to media</Link> is off), so nothing is written here unless you say so. With this on, new video files are written, but still no subtitle files or artwork.</>}>
            <Switch label="Allow conversions in this library" checked={allow} onChange={setAllow} />
          </Row>
        )}
      </Section>
      <SaveBar dirty={dirty} busy={busy} save={save} discard={() => { setPolicy(lib.rule.policy); setOutput(lib.rule.output); setReadOnly(lib.readOnly); setAllow(lib.rule.allowWrites); setAuto(lib.rule.auto) }} />

      <TryOneFile lib={lib} policy={policy} />
      <Whole lib={lib} policy={policy} output={output} dirty={dirty} settings={settings} save={save} saving={busy} onEnded={() => setEnded((n) => n + 1)} />
      <Leftovers key={ended} lib={lib} />
    </>
  )
}

// The details, for anyone who wants them.
function RecipeDetails({ policy: p, change, options }: { policy: Policy; change: (p: Policy) => void; options: Options }) {
  const video = (patch: Partial<Policy['video']>) => change({ ...p, video: { ...p.video, ...patch } })
  const audio = (patch: Partial<Policy['audio']>) => change({ ...p, audio: { ...p.audio, ...patch } })
  const subs = (patch: Partial<Policy['subtitles']>) => change({ ...p, subtitles: { ...p.subtitles, ...patch } })
  const yesNo = [{ value: 'keep', label: 'Keep' }, { value: 'drop', label: 'Remove' }]
  const converting = p.video.codec !== 'keep'
  return (
    <div className="ltc-details">
      <details className="ltc-more big">
        <summary>Picture details</summary>
        <Row label="Picture format" desc="H.264 plays on everything. H.265 and AV1 are much smaller, and older devices may not play them. Leaving it alone is quickest and loses nothing.">
          <Select value={p.video.codec} onChange={(e) => video({ codec: e.target.value })} options={[
            { value: 'keep', label: 'Leave as it is' }, { value: 'h264', label: 'H.264' }, { value: 'hevc', label: options.capabilities.hevc ? 'H.265 (HEVC)' : 'H.265 (HEVC): not in this FFmpeg' }, { value: 'av1', label: options.capabilities.av1 ? 'AV1' : 'AV1: not in this FFmpeg' },
          ]} />
        </Row>
        <Row label="File type" desc="MP4 plays everywhere, and holds fewer kinds of sound and subtitles. MKV holds anything.">
          <Select value={p.container} onChange={(e) => change({ ...p, container: e.target.value })} options={[{ value: 'keep', label: 'Keep MP4 and MKV as they are; make others MKV' }, { value: 'mkv', label: 'MKV' }, { value: 'mp4', label: 'MP4' }]} />
        </Row>
        <Row dim={!converting} label="Quality" desc="Lower numbers look better and take more room.">
          <Select disabled={!converting} value={String(p.video.quality)} onChange={(e) => video({ quality: Number(e.target.value) })} options={[...new Set([18, 21, 23, 26, 28, p.video.quality || 23])].sort((a, b) => a - b).map((q) => ({ value: String(q), label: `${q}: ${q <= 18 ? 'looks the same as the source' : q <= 21 ? 'very good' : q <= 23 ? 'good' : q <= 26 ? 'smaller' : 'smallest'}` }))} />
        </Row>
        <Row dim={!converting} label="Speed" desc="Slower makes smaller files for the same quality.">
          <Segmented label="Speed" value={(p.video.speed || 'balanced') as 'faster' | 'balanced' | 'slower'} onChange={(v) => video({ speed: v })} options={[{ value: 'faster', label: 'Faster' }, { value: 'balanced', label: 'Balanced' }, { value: 'slower', label: 'Slower' }]} />
        </Row>
        <Row dim={!converting} label="Largest picture" desc="Anything taller is made this size.">
          <Select disabled={!converting} value={String(p.video.maxHeight)} onChange={(e) => video({ maxHeight: Number(e.target.value) })} options={[{ value: '0', label: 'Keep its size' }, { value: '2160', label: '4K (2160p)' }, { value: '1080', label: '1080p' }, { value: '720', label: '720p' }, { value: '480', label: '480p' }]} />
        </Row>
        <Row label="HDR pictures" desc={options.capabilities.toneMap ? 'Ordinary pictures play on every screen, and lose the extra brightness and colour.' : 'Ordinary pictures play on every screen, but this server’s FFmpeg can’t make them, so those files would be left alone.'}>
          <Select value={p.video.hdr} onChange={(e) => video({ hdr: e.target.value })} options={[{ value: 'keep', label: 'Keep HDR' }, { value: 'tonemap', label: 'Make an ordinary picture' }, { value: 'refuse', label: 'Leave those files alone' }]} />
        </Row>
        <Row label="Dolby Vision" desc="It only survives if the picture isn’t converted. Files with no ordinary HDR layer inside (profile 5) can’t be converted either way.">
          <Select value={p.video.dolbyVision} onChange={(e) => video({ dolbyVision: e.target.value })} options={[{ value: 'keep', label: 'Keep it: leave a file alone rather than lose it' }, { value: 'base', label: 'Convert from the ordinary layer, losing it' }]} />
        </Row>
        <Row dim={!converting} label="Make 10-bit pictures 8-bit" desc="Many older devices can’t show 10-bit. Some smooth gradients may show bands.">
          <Switch label="Make 10-bit pictures 8-bit" disabled={!converting} checked={p.video.eightBit} onChange={(v) => video({ eightBit: v })} />
        </Row>
        <Row dim={!converting} label="Deinterlace" desc="Old TV recordings and DVDs store two half-pictures per frame. This makes whole ones.">
          <Switch label="Deinterlace" disabled={!converting} checked={p.video.deinterlace} onChange={(v) => video({ deinterlace: v })} />
        </Row>
        <Row dim={!converting} label="Make a varying frame rate constant" desc="Phone videos and screen recordings often vary, and some devices stutter on them. Left off, it stays as it is, and sound stays in step either way.">
          <Switch label="Make a varying frame rate constant" disabled={!converting} checked={p.video.constantRate} onChange={(v) => video({ constantRate: v })} />
        </Row>
        <Row label="3D films" desc="One eye makes an ordinary film, which is the same picture to anyone without 3D glasses.">
          <Select value={p.video.threeD} onChange={(e) => video({ threeD: e.target.value })} options={[{ value: 'keep', label: 'Keep both eyes' }, { value: 'flatten', label: 'Keep one eye' }, { value: 'refuse', label: 'Leave those files alone' }]} />
        </Row>
        <Row label="Files with more than one picture" desc="Some files hold other camera angles or a small inset picture. Most players only show the main one, which is the largest unless the file says otherwise. Cover pictures are always kept.">
          <Select value={p.video.extraVideo} onChange={(e) => video({ extraVideo: e.target.value })} options={[{ value: 'keep', label: 'Keep them all' }, { value: 'drop', label: 'Keep only the main picture' }]} />
        </Row>
        <Row dim={!converting} label="Smallest saving worth keeping" desc="If a new file doesn’t come out at least this much smaller, the old one stays and it isn’t tried again.">
          <Select disabled={!converting} value={String(p.video.minSavingPercent)} onChange={(e) => video({ minSavingPercent: Number(e.target.value) })} options={[...new Set([0, 5, 10, 20, 30, p.video.minSavingPercent])].sort((a, b) => a - b).map((n) => ({ value: String(n), label: n === 0 ? 'Any size' : `${n}% smaller` }))} />
        </Row>
      </details>

      <details className="ltc-more big">
        <summary>Sound details</summary>
        <Row label="Which sound tracks" desc="At least one track always stays, whatever you choose here.">
          <Select value={p.audio.keep} onChange={(e) => audio({ keep: e.target.value })} options={[{ value: 'all', label: 'Keep them all' }, { value: 'languages', label: 'Only these languages' }]} />
        </Row>
        {p.audio.keep === 'languages' && <LanguageList label="Languages to keep, in this order" list={p.audio.languages} onChange={(languages) => audio({ languages })} />}
        {p.audio.keep === 'languages' && (
          <Row label="Also keep the original language" desc="Whatever the film was made in, if the file says or its first track shows it.">
            <Switch label="Also keep the original language" checked={p.audio.keepOriginal} onChange={(v) => audio({ keepOriginal: v })} />
          </Row>
        )}
        <Row label="Plays first" desc="The language chosen when a viewer hasn’t said what they want.">
          <Select value={p.audio.default} onChange={(e) => audio({ default: e.target.value })}>
            <option value="">What the file says</option>
            {LANGUAGES.map(([c, n]) => <option key={c} value={c}>{n}</option>)}
          </Select>
        </Row>
        <Row label="Tracks that don’t say their language">
          <Select value={p.audio.untagged} onChange={(e) => audio({ untagged: e.target.value })} options={yesNo} />
        </Row>
        <Row label="Label those tracks as" desc="If you know what they are.">
          <Select value={p.audio.untaggedAs} onChange={(e) => audio({ untaggedAs: e.target.value })}>
            <option value="">Leave them unlabelled</option>
            {LANGUAGES.map(([c, n]) => <option key={c} value={c}>{n}</option>)}
          </Select>
        </Row>
        <Row label="Commentary tracks"><Select value={p.audio.commentary} onChange={(e) => audio({ commentary: e.target.value })} options={yesNo} /></Row>
        <Row label="Audio description" desc="A voice describing what’s on screen, for people who can’t see it."><Select value={p.audio.description} onChange={(e) => audio({ description: e.target.value })} options={yesNo} /></Row>
        <Row label="Sound format" desc="AAC plays everywhere. Dolby Digital Plus keeps surround and passes through to most sound systems. Opus is smallest, and needs MKV.">
          <Select value={p.audio.codec} onChange={(e) => audio({ codec: e.target.value })} options={[{ value: 'keep', label: 'Leave as it is' }, { value: 'aac', label: 'AAC' }, { value: 'eac3', label: 'Dolby Digital Plus' }, { value: 'ac3', label: 'Dolby Digital' }, ...(options.capabilities.opus ? [{ value: 'opus', label: 'Opus' }] : [])]} />
        </Row>
        <Row label="Lossless sound" desc="TrueHD, DTS-HD MA, FLAC and PCM sound exactly like the master, and often take as much room as a small film.">
          <Select value={p.audio.lossless} onChange={(e) => audio({ lossless: e.target.value })} options={[{ value: 'keep', label: 'Keep' }, { value: 'convert', label: 'Convert to the format above' }]} />
        </Row>
        <Row label="Object sound (Atmos, DTS:X)" desc="Its height and object information doesn’t survive conversion. Tracks left as they are keep it.">
          <Select value={p.audio.objects} onChange={(e) => audio({ objects: e.target.value })} options={[{ value: 'keep', label: 'Keep' }, { value: 'convert', label: 'Convert, losing it' }]} />
        </Row>
        <Row label="Most channels" desc="Wider tracks are converted down.">
          <Select value={String(p.audio.maxChannels)} onChange={(e) => audio({ maxChannels: Number(e.target.value) })} options={[{ value: '0', label: 'No limit' }, { value: '8', label: '7.1' }, { value: '6', label: '5.1' }, { value: '2', label: 'Stereo' }]} />
        </Row>
        <Row label="Add a stereo track beside surround" desc="Where a language has only surround sound, so that phones, headphones and TVs without a sound system have something simple to play.">
          <Switch label="Add a stereo track beside surround" checked={p.audio.addStereo} onChange={(v) => audio({ addStereo: v })} />
        </Row>
        <Row dim={!p.audio.addStereo} label="Put stereo ahead of surround" desc="Plain devices start with the first track.">
          <Switch label="Put stereo ahead of surround" disabled={!p.audio.addStereo} checked={p.audio.stereoFirst} onChange={(v) => audio({ stereoFirst: v })} />
        </Row>
      </details>

      <details className="ltc-more big">
        <summary>Subtitle details</summary>
        <Row label="Which subtitles">
          <Select value={p.subtitles.keep} onChange={(e) => subs({ keep: e.target.value })} options={[{ value: 'all', label: 'Keep them all' }, { value: 'languages', label: 'Only these languages' }, { value: 'none', label: 'Remove them' }]} />
        </Row>
        {p.subtitles.keep === 'languages' && <LanguageList label="Subtitle languages to keep, in this order" list={p.subtitles.languages} onChange={(languages) => subs({ languages })} />}
        <Row label="Forced subtitles" desc="Only the lines in another language, such as a character speaking French in an English film. Burned-in ones can’t be turned off, and burning means converting the picture.">
          <Select value={p.subtitles.forced} onChange={(e) => subs({ forced: e.target.value })} options={[{ value: 'keep', label: 'Keep as tracks' }, { value: 'burn-images', label: 'Burn in if stored as pictures and the file type needs it' }, { value: 'burn', label: 'Always burn into the picture' }, { value: 'drop', label: 'Remove' }]} />
        </Row>
        <Row label="SDH subtitles" desc="With sound descriptions, for people who can’t hear."><Select value={p.subtitles.sdh} onChange={(e) => subs({ sdh: e.target.value })} options={yesNo} /></Row>
        <Row label="Subtitles that don’t say their language"><Select value={p.subtitles.untagged} onChange={(e) => subs({ untagged: e.target.value })} options={yesNo} /></Row>
        <Row label="Subtitles stored as pictures" desc="Blu-ray and DVD subtitles are pictures, and they can’t become text here: that would need character recognition, which this doesn’t do. An MP4 can’t hold them at all.">
          <Select value={p.subtitles.images} onChange={(e) => subs({ images: e.target.value })} options={[{ value: 'keep', label: 'Keep them, staying in MKV if need be' }, { value: 'export', label: 'Save Blu-ray ones as a file beside the video' }, { value: 'drop', label: 'Remove' }]} />
        </Row>
        <Row label="Text subtitles" desc="Plain text is the most widely shown, and loses colours and positioning. An MP4 always converts them to its own kind.">
          <Select value={p.subtitles.text} onChange={(e) => subs({ text: e.target.value })} options={[{ value: 'keep', label: 'Keep as they are' }, { value: 'srt', label: 'Make them plain text' }]} />
        </Row>
        <Row label="Subtitle files beside the video" desc="Files such as Film.en.srt. Left where they are, they keep working, since the name stays the same. Moved inside, the file is no longer needed.">
          <Select value={p.subtitles.sidecars} onChange={(e) => subs({ sidecars: e.target.value })} options={[{ value: 'leave', label: 'Leave them where they are' }, { value: 'embed', label: 'Move text ones inside the new file' }]} />
        </Row>
      </details>
    </div>
  )
}

function LanguageList({ label, list, onChange }: { label: string; list: string[]; onChange: (l: string[]) => void }) {
  const add = (code: string) => { if (code && !list.includes(code)) onChange([...list, code]) }
  const move = (i: number, by: number) => { const n = [...list]; const [x] = n.splice(i, 1); n.splice(i + by, 0, x); onChange(n) }
  return (
    <Row stack label={label}>
      <div className="ltc-langs">
        {list.length === 0 && <p className="ltc-muted">None yet. Add one from the list.</p>}
        <ol>
          {list.map((c, i) => (
            <li key={c}>
              <span>{languageName(c)}</span>
              <Button size="sm" variant="ghost" disabled={i === 0} aria-label={`Move ${languageName(c)} up`} onClick={() => move(i, -1)}>Up</Button>
              <Button size="sm" variant="ghost" disabled={i === list.length - 1} aria-label={`Move ${languageName(c)} down`} onClick={() => move(i, 1)}>Down</Button>
              <Button size="sm" variant="ghost" aria-label={`Remove ${languageName(c)}`} onClick={() => onChange(list.filter((x) => x !== c))}>Remove</Button>
            </li>
          ))}
        </ol>
        <Select aria-label="Add a language" value="" onChange={(e) => add(e.target.value)}>
          <option value="">Add a language…</option>
          {LANGUAGES.filter(([c]) => !list.includes(c)).map(([c, n]) => <option key={c} value={c}>{n}</option>)}
        </Select>
      </div>
    </Row>
  )
}

// What would become of one file.
function TryOneFile({ lib, policy }: { lib: Library; policy: Policy }) {
  const [q, setQ] = useState('')
  const [items, setItems] = useState<{ id: number; title: string; kind: string }[] | null>(null)
  const [item, setItem] = useState(0)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [sample, setSample] = useState<Sample | null>(null)
  const [busy, setBusy] = useState('')
  useEffect(() => {
    let gone = false
    const t = setTimeout(() => {
      api.get<{ id: number; title: string; kind: string }[]>(`/libtranscode/libraries/${lib.id}/items?q=${encodeURIComponent(q)}`).then((l) => {
        if (gone) return
        setItems(l)
        setItem((cur) => (l.some((x) => x.id === cur) ? cur : (l[0]?.id ?? 0)))
      }).catch(() => { if (!gone) setItems([]) })
    }, q ? 250 : 0)
    return () => { gone = true; clearTimeout(t) }
  }, [lib.id, q])
  // A changed recipe makes what's shown out of date.
  useEffect(() => { setPreview(null); setSample(null) }, [policy, item])
  const show = async () => {
    setBusy('preview')
    try { setPreview(await api.post<Preview>(`/libtranscode/libraries/${lib.id}/preview`, { itemId: item, policy })); setSample(null) } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const measure = async () => {
    setBusy('sample')
    try { setSample(await api.post<Sample>(`/libtranscode/libraries/${lib.id}/sample`, { itemId: item, policy })) } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const convertible = preview?.parts.some((p) => p.plan.verdict === 'convert' || p.plan.verdict === 'remux')
  return (
    <Section title="Try it on one file" desc="Shows what would become of each track. Nothing is changed.">
      <div className="ltc-pick">
        <Field label="Find a file"><Input value={q} placeholder="Part of its title" onChange={(e) => setQ(e.target.value)} /></Field>
        <Field label="File">
          <Select value={String(item)} disabled={!items?.length} onChange={(e) => setItem(Number(e.target.value))}>
            {items?.length === 0 && <option value="0">Nothing found</option>}
            {items?.map((i) => <option key={i.id} value={i.id}>{i.title}</option>)}
          </Select>
        </Field>
      </div>
      <p className="ltc-actions">
        <Button variant="primary" disabled={!item} busy={busy === 'preview'} onClick={show}>Show what would happen</Button>
        {convertible && <Button busy={busy === 'sample'} onClick={measure}>Time a 20-second test</Button>}
      </p>
      {sample && (
        <Notice tone="ok" title="Measured on this machine.">
          20 seconds took {fmtDuration(Math.max(1, Math.round(sample.took)))}, or {sample.speed.toFixed(1)} times real time. The whole film would take about {fmtDuration(Math.round(sample.estFileSec))} and come to about {fmtBytes(sample.estFileBytes)}. {sample.note}
        </Notice>
      )}
      <div aria-live="polite">
        {preview?.parts.map((part, i) => <PlanView key={part.path} plan={part.plan} size={part.size} heading={preview.parts.length > 1 ? `Part ${i + 1}` : ''} />)}
      </div>
    </Section>
  )
}

const VERDICTS: Record<string, { label: string; tone: 'ok' | 'info' | 'warn' }> = {
  convert: { label: 'Will be converted', tone: 'info' },
  remux: { label: 'Will be tidied', tone: 'ok' },
  skip: { label: 'Nothing to do', tone: 'ok' },
  refuse: { label: 'Left alone', tone: 'warn' },
}

const ACTION_WORDS: Record<string, string> = { keep: 'Keep', convert: 'Convert', drop: 'Remove', add: 'Add', burn: 'Burn in', export: 'Save beside', embed: 'Move inside', leave: 'Leave' }
const KIND_WORDS: Record<string, string> = { video: 'Picture', cover: 'Cover', audio: 'Sound', subtitle: 'Subtitles', attachment: 'Attached files', data: 'Other', file: 'Files beside it' }

function PlanView({ plan, size, heading }: { plan: Step['plan']; size: number; heading?: string }) {
  const v = VERDICTS[plan.verdict] ?? VERDICTS.skip
  const kinds = useMemo(() => {
    const out: [string, typeof plan.steps][] = []
    for (const st of plan.steps) {
      const at = out.find(([k]) => k === st.kind)
      if (at) at[1].push(st)
      else out.push([st.kind, [st]])
    }
    return out
  }, [plan])
  return (
    <div className="ltc-plan">
      {heading && <h3>{heading}</h3>}
      <Notice tone={v.tone} title={v.label}>{plan.reason}</Notice>
      {plan.warnings.map((w) => <Notice key={w} tone="warn">{w}</Notice>)}
      {plan.notes.map((w) => <p key={w} className="ltc-note"><IconInfo size={16} /> {w}</p>)}
      {(plan.verdict === 'convert' || plan.verdict === 'remux') && plan.estBytes > 0 && (
        <p className="ltc-muted">Rough estimate: {fmtBytes(size)} becomes about {fmtBytes(plan.estBytes)}, taking about {fmtDuration(Math.max(60, Math.round(plan.estSec)))} here. A 20-second test measures it properly.</p>
      )}
      {kinds.map(([kind, steps]) => (
        <div key={kind} className="ltc-steps">
          <h4>{KIND_WORDS[kind] ?? kind}</h4>
          <ul>
            {steps.map((st, i) => (
              <li key={i} className={'ltc-step ' + st.action}>
                <span className="ltc-step-act">{ACTION_WORDS[st.action] ?? st.action}</span>
                <span className="ltc-step-what">
                  <span>{st.from}{st.to && st.action !== 'keep' ? <> <span className="ltc-arrow" aria-hidden="true">→</span><span className="ltc-sr"> becomes </span>{st.to}</> : null}</span>
                  {st.why && <small>{st.why}</small>}
                </span>
              </li>
            ))}
          </ul>
        </div>
      ))}
    </div>
  )
}

// The dry run, and running it for real.
function Whole({ lib, policy, output, dirty, settings, save, saving, onEnded }: { lib: Library; policy: Policy; output: string; dirty: boolean; settings: Settings; save: () => void; saving: boolean; onEnded: () => void }) {
  const a = useActivity()
  const [runs, setRuns] = useState<Run[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [busy, setBusy] = useState('')
  const load = useCallback(() => {
    setFailed(false)
    api.get<Run[]>(`/libtranscode/runs?library=${lib.id}`).then(setRuns).catch(() => setFailed(true))
  }, [lib.id])
  useEffect(load, [load])
  const live = a.tasks.find((t) => t.type === 'convert' && t.libraryId === lib.id)
  const going = runs?.some((r) => r.state === 'running' || r.state === 'queued')
  // The activity stream says when something has moved. Without it (it can be turned off),
  // asking now and then while something's going is all there is.
  const liveId = live?.id
  useEffect(() => { load(); if (!liveId) onEnded() }, [load, liveId]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    if (!going || (a.access.visible && a.connected)) return
    const t = setInterval(load, 5000)
    return () => clearInterval(t)
  }, [going, a.access.visible, a.connected, load])
  const dry = async () => {
    setBusy('dry')
    try { await api.post(`/libtranscode/libraries/${lib.id}/runs`, { dry: true, policy, output }); load() } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const lastDry = runs?.find((r) => r.dry && r.state === 'done')
  const latest = runs?.find((r) => r.state !== 'running' && r.state !== 'queued')
  const start = async () => {
    const s = lastDry?.summary
    const count = s ? s.convert + s.remux : 0
    const after = { keep: 'Old files then move to a hidden folder beside them, until you decide.', replace: 'Old files are then deleted for good.', beside: 'Old files stay in use, with a copy written beside each one.' }[lib.rule.output] ?? ''
    if (!await ask({
      title: `Convert ${lib.name}?`,
      body: `${s ? `The last check found ${count.toLocaleString()} files to change.` : 'You haven’t checked this library yet, so there’s no estimate.'} Each new file is checked before anything happens to the old one. ${after} You can pause or cancel it here or in the activity panel.`,
      confirm: 'Start converting',
    })) return
    setBusy('start')
    try { await api.post(`/libtranscode/libraries/${lib.id}/runs`, { dry: false }); load() } catch (e) { toast((e as Error).message) } finally { setBusy('') }
  }
  const blocked = !settings.enabled ? 'Turn library transcoding on first, at the top of this page.' : lib.blocked
  return (
    <Section title="This whole library" desc="A check looks at every file and changes nothing. It’s the way to see what you’d get before you start.">
      <p className="ltc-actions">
        <Button disabled={busy === 'dry' || !!live} busy={busy === 'dry'} icon={<IconRefresh size={16} />} onClick={dry}>Check this library</Button>
        <Button variant="primary" disabled={!!blocked || dirty || !!live || busy === 'start' || !lib.rule.saved} busy={busy === 'start'} onClick={start}>Start converting</Button>
      </p>
      {blocked && <p className="ltc-muted">{blocked}</p>}
      {!blocked && dirty && <p className="ltc-muted">Save your changes first: a run does what’s saved.</p>}
      {!blocked && !dirty && !lib.rule.saved && <p className="ltc-actions"><span className="ltc-muted">This library has no recipe saved yet. A run does what’s saved.</span> <Button size="sm" busy={saving} onClick={save}>Save this recipe</Button></p>}
      {live && <LiveRun task={live} />}
      {failed && <LoadError what="The results" onRetry={load} />}
      {runs && latest && !live && <RunResult key={latest.id} run={latest} heading={latest.dry ? 'Latest check' : latest.auto ? 'Latest automatic conversion' : 'Latest conversion'} />}
      {runs && <Past runs={runs.filter((r) => r.id !== latest?.id)} />}
    </Section>
  )
}

function LiveRun({ task: t }: { task: Task }) {
  const [busy, setBusy] = useState(false)
  const act = async (what: 'pause' | 'resume' | 'cancel') => {
    setBusy(true)
    try { await taskAction(t.id, what) } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <div className="ltc-live" aria-live="polite">
      <div className="ltc-live-head"><b>{t.title}</b><span>{t.state === 'paused' ? 'Paused' : t.state === 'queued' ? 'Waiting' : `${Math.max(0, Math.round(t.percent))}%`}</span></div>
      <Progress label={t.title} value={t.percent >= 0 ? t.percent : undefined} />
      {t.phase && <div className="ltc-muted">{t.phase}</div>}
      {t.item && <div className="ltc-muted ltc-path">{t.item}</div>}
      {!!t.etaSec && <div className="ltc-muted">About {fmtDuration(t.etaSec)} left</div>}
      {!!t.problemCount && <div className="ltc-muted">{t.problemCount} {t.problemCount === 1 ? 'problem' : 'problems'} so far. They’re in the activity panel.</div>}
      <p className="ltc-actions">
        {t.canPause && <Button size="sm" disabled={busy} icon={<IconPause size={16} />} onClick={() => act('pause')}>Pause</Button>}
        {t.canResume && <Button size="sm" disabled={busy} icon={<IconPlay size={16} />} onClick={() => act('resume')}>Resume</Button>}
        {t.canCancel && <Button size="sm" variant="ghost" disabled={busy} icon={<IconStop size={16} />} onClick={() => act('cancel')}>Cancel</Button>}
      </p>
    </div>
  )
}

const STATE_WORDS: Record<string, string> = { planned: 'To do', pending: 'Waiting', running: 'Converting', done: 'Done', skipped: 'Nothing to do', refused: 'Left alone', failed: 'Failed', changed: 'Changed meanwhile', notworth: 'Not worth it', postponed: 'Being played', undone: 'Put back' }

function Tile({ value, label }: { value: ReactNode; label: string }) {
  return <div className="ltc-tile"><b>{value}</b><span>{label}</span></div>
}

function RunResult({ run, heading }: { run: Run; heading?: string }) {
  const s = run.summary
  const [files, setFiles] = useState<FileRow[] | null>(null)
  const [total, setTotal] = useState(0)
  const [state, setState] = useState('')
  const [page, setPage] = useState(0)
  const [failed, setFailed] = useState(false)
  const PAGE = 50
  const load = useCallback(() => {
    setFailed(false)
    api.get<{ files: FileRow[]; total: number }>(`/libtranscode/runs/${run.id}/files?limit=${PAGE}&offset=${page * PAGE}&state=${state}`)
      .then((r) => { setFiles(r.files); setTotal(r.total) }).catch(() => setFailed(true))
  }, [run.id, page, state])
  useEffect(load, [load])
  const changed = s.convert + s.remux
  const filters = run.dry
    ? [['', 'All'], ['planned', 'To do'], ['skipped', 'Nothing to do'], ['refused', 'Left alone']]
    : [['', 'All'], ['done', 'Done'], ['failed', 'Failed'], ['notworth', 'Not worth it'], ['postponed', 'Being played'], ['skipped', 'Nothing to do'], ['refused', 'Left alone']]
  return (
    <div className="ltc-result">
      {heading && <h3>{heading}</h3>}
      <div className="ltc-tiles">
        <Tile value={s.files.toLocaleString()} label="files looked at" />
        <Tile value={s.convert.toLocaleString()} label="to convert" />
        <Tile value={s.remux.toLocaleString()} label="only to tidy" />
        <Tile value={s.skip.toLocaleString()} label="nothing to do" />
        <Tile value={s.refuse.toLocaleString()} label="left alone" />
        {!run.dry && <Tile value={s.done.toLocaleString()} label="done" />}
        {!run.dry && !!s.notWorth && <Tile value={s.notWorth.toLocaleString()} label="came out no smaller" />}
        {!run.dry && !!s.postponed && <Tile value={s.postponed.toLocaleString()} label="being played" />}
        {!run.dry && !!s.failed && <Tile value={s.failed.toLocaleString()} label="failed" />}
      </div>
      {run.dry && changed > 0 && (
        <>
          <p>Rough estimate: the library goes from <b>{fmtBytes(s.bytesBefore)}</b> to about <b>{fmtBytes(s.estBytes)}</b>, taking about <b>{fmtDuration(Math.max(60, Math.round(s.estSec)))}</b> of processor work. Pauses for people watching come on top. Try a file above for a real measurement.</p>
          {s.drives?.map((d) => (
            <Notice key={d.folder} tone={d.enough ? 'ok' : 'danger'} title={d.enough ? 'Enough room' : 'Not enough room'}>
              {d.folder}: {fmtBytes(d.free)} free, and this may need about {fmtBytes(d.needs)} while working. {d.enough ? '' : 'Files that don’t fit will wait and be reported. Free up some space, or choose to replace originals.'}
            </Notice>
          ))}
        </>
      )}
      {!run.dry && s.done > 0 && <p>Files that were changed now take <b>{fmtBytes(s.bytesAfter)}</b>.</p>}
      {run.dry && changed === 0 && <Notice tone="ok" title="Nothing to do">Every file here already matches this recipe, or is being left alone for a reason shown below.</Notice>}
      <Chips label="Show">
        {filters.map(([v, l]) => <Chip key={v} selected={state === v} onClick={() => { setState(v); setPage(0) }}>{l}</Chip>)}
      </Chips>
      {failed && <LoadError what="This list" onRetry={load} />}
      {files && files.length === 0 && <p className="ltc-muted">No files here.</p>}
      <ul className="ltc-files">
        {files?.map((f) => (
          <li key={f.id}>
            <details className="ltc-more">
              <summary>
                <span className="ltc-file-name">{f.path.split(/[\\/]/).pop()}</span>
                <span className={'ltc-chip ' + f.state}>{f.state === 'pending' && run.state !== 'running' && run.state !== 'queued' ? 'Not done' : STATE_WORDS[f.state] ?? f.state}</span>
              </summary>
              <p className="ltc-muted ltc-path">{f.path}</p>
              {f.message && <p>{f.message}</p>}
              {f.state === 'done' && f.sizeAfter > 0 && <p className="ltc-muted">{fmtBytes(f.sizeBefore)} became {fmtBytes(f.sizeAfter)}.</p>}
              {f.plan && <PlanView plan={f.plan} size={f.sizeBefore} />}
            </details>
          </li>
        ))}
      </ul>
      {total > PAGE && (
        <p className="ltc-actions">
          <Button size="sm" disabled={page === 0} onClick={() => setPage(page - 1)}>Earlier</Button>
          <span className="ltc-muted">{page * PAGE + 1} to {Math.min((page + 1) * PAGE, total)} of {total.toLocaleString()}</span>
          <Button size="sm" disabled={(page + 1) * PAGE >= total} onClick={() => setPage(page + 1)}>Later</Button>
        </p>
      )}
    </div>
  )
}

const RUN_WORDS: Record<string, string> = { queued: 'Waiting', running: 'Going', done: 'Finished', failed: 'Failed', cancelled: 'Cancelled', interrupted: 'Stopped by a restart' }

function Past({ runs }: { runs: Run[] }) {
  if (!runs.length) return null
  return (
    <details className="ltc-more big">
      <summary>Earlier runs ({runs.length})</summary>
      <ul className="ltc-files">
        {runs.map((r) => (
          <li key={r.id}>
            <details className="ltc-more">
              <summary>
                <span className="ltc-file-name">{r.dry ? 'Check' : r.auto ? 'Automatic conversion' : 'Conversion'}, {new Date(r.createdAt * 1000).toLocaleString()}</span>
                <span className={'ltc-chip ' + r.state}>{RUN_WORDS[r.state] ?? r.state}</span>
              </summary>
              <RunResult run={r} />
            </details>
          </li>
        ))}
      </ul>
    </details>
  )
}

// Originals kept, and copies waiting for a decision.
function Leftovers({ lib }: { lib: Library }) {
  const [files, setFiles] = useState<FileRow[] | null>(null)
  const [busy, setBusy] = useState(0)
  const load = useCallback(() => {
    api.get<{ files: FileRow[]; totalBytes: number }>(`/libtranscode/leftovers?library=${lib.id}`).then((r) => setFiles(r.files)).catch(() => setFiles([]))
  }, [lib.id])
  useEffect(load, [load])
  if (!files?.length) return null
  const act = async (f: FileRow, what: string) => {
    setBusy(f.id)
    try {
      if (what === 'undo') await api.post(`/libtranscode/files/${f.id}/undo`)
      else if (what === 'use') await api.post(`/libtranscode/files/${f.id}/use-copy`)
      else await api.del(`/libtranscode/files/${f.id}/leftover`)
      load()
    } catch (e) { toast((e as Error).message) } finally { setBusy(0) }
  }
  const remove = async (f: FileRow) => {
    const kept = !!f.keptPath
    if (!await ask({ title: kept ? 'Delete the original for good?' : 'Delete the converted copy?', body: kept ? 'It can’t be put back after this.' : 'The original stays as it is, and in use.', confirm: 'Delete', danger: true })) return
    act(f, 'delete')
  }
  return (
    <Section title="Originals and copies" desc="What’s waiting for a decision. Kept originals are in a hidden folder beside each film, and take up room until you delete them.">
      <ul className="ltc-files">
        {files.map((f) => (
          <li key={f.id} className="ltc-left">
            <div>
              <div className="ltc-file-name">{f.path.split(/[\\/]/).pop()}</div>
              <div className="ltc-muted">{f.keptPath ? `Original kept: ${fmtBytes(f.sizeBefore)}. The new file is ${fmtBytes(f.sizeAfter)}.` : `A copy is waiting beside it: ${fmtBytes(f.sizeAfter)}, against ${fmtBytes(f.sizeBefore)}.`}</div>
            </div>
            <div className="ltc-actions">
              {f.keptPath
                ? <Button size="sm" disabled={busy === f.id} onClick={() => act(f, 'undo')}>Put the original back</Button>
                : <Button size="sm" disabled={busy === f.id} icon={<IconCheck size={16} />} onClick={() => act(f, 'use')}>Use the copy</Button>}
              <Button size="sm" variant="ghost" disabled={busy === f.id} icon={<IconTrash size={16} />} onClick={() => remove(f)}>{f.keptPath ? 'Delete the original' : 'Delete the copy'}</Button>
            </div>
          </li>
        ))}
      </ul>
    </Section>
  )
}
