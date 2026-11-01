import { useState, type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { IconCheck, IconDownload, IconEdit, IconFilm, IconMore, IconPlay, IconPlus, IconRefresh, IconSearch, IconSettings, IconTrash } from '../icons'
import { useApp } from '../state'
import {
  Button, Card, Chip, Chips, DataTable, Dialog, EmptyState, Field, IconButton, Input, LinkButton, LoadError, Menu, Notice, PosterCard, Progress, ProgressRing, Segmented, Select, Sheet, Shelf,
  Skeleton, SkeletonGrid, Spinner, Switch, Tabs, Textarea, Tooltip, ask, toast, type Column,
} from '../ui'
import { Placeholder } from '../components/common'

// Every shared part in one place, in whatever theme is in use, for administrators
// (/design). Look here first when something in the design changes. docs/DESIGN.md
// is the written version of this page.

const SECTIONS = ['Colour', 'Type', 'Buttons', 'Fields', 'Choices', 'Overlays', 'Messages', 'Cards', 'States', 'Progress', 'Tables']

function Part({ title, note, children }: { title: string; note?: ReactNode; children: ReactNode }) {
  return (
    <section className="ds-part" id={title.toLowerCase()} aria-labelledby={'ds-' + title}>
      <h2 id={'ds-' + title}>{title}</h2>
      {note && <p className="ds-note">{note}</p>}
      {children}
    </section>
  )
}

const Row = ({ label, children }: { label?: string; children: ReactNode }) => (
  <div className="ds-row">{label && <span className="ds-label">{label}</span>}<div className="ds-items">{children}</div></div>
)

const SWATCHES = ['bg', 'bg-2', 'bg-3', 'bg-4', 'fg', 'fg-2', 'fg-3', 'accent', 'accent-2', 'accent-text', 'accent-soft', 'danger', 'ok', 'warn', 'border-strong', 'control-line']
const TYPE = ['2xs', 'xs', 'sm', 'md', 'base', 'lg', 'xl', '2xl', '3xl']
type Film = { id: number; title: string; year: number; size: number }
const FILMS: Film[] = [
  { id: 1, title: 'The Lantern Keeper', year: 2019, size: 8.2 },
  { id: 2, title: 'North of Nowhere', year: 2004, size: 4.1 },
  { id: 3, title: 'A Quiet Harbour', year: 2023, size: 12.7 },
]
const COLUMNS: Column<Film>[] = [
  { key: 'title', header: 'Title', cell: (f) => f.title, sort: (a, b) => a.title.localeCompare(b.title) },
  { key: 'year', header: 'Year', cell: (f) => f.year, sort: (a, b) => a.year - b.year },
  { key: 'size', header: 'Size', align: 'end', cell: (f) => `${f.size} GB`, sort: (a, b) => a.size - b.size },
]

export default function DesignPage() {
  const { user } = useApp()
  const [sw, setSw] = useState(true)
  const [seg, setSeg] = useState<'day' | 'week' | 'month'>('week')
  const [tab, setTab] = useState<'one' | 'two' | 'three'>('one')
  const [chips, setChips] = useState<string[]>(['Comedy'])
  const [tags, setTags] = useState(['Kids', 'Family'])
  const [dialog, setDialog] = useState(false)
  const [sheet, setSheet] = useState(false)
  const [name, setName] = useState('')
  if (!user?.isAdmin) return <Navigate to="/" replace />
  const toggleChip = (c: string) => setChips((l) => (l.includes(c) ? l.filter((x) => x !== c) : [...l, c]))
  return (
    <div className="ds-page">
      <header className="toolbar"><h1>Design system</h1><span className="count">Every shared part, in the theme in use</span></header>
      <nav className="ds-toc" aria-label="On this page">{SECTIONS.map((s) => <a key={s} href={'#' + s.toLowerCase()}>{s}</a>)}</nav>

      <Part title="Colour" note="Set by the theme. Hover, selected and tinted colours are worked out from these.">
        <div className="ds-swatches">{SWATCHES.map((c) => <div key={c} className="ds-swatch"><i data-swatch={c} style={{ background: `var(--${c})` }} /><code>--{c}</code></div>)}</div>
      </Part>

      <Part title="Type" note="One scale, in rem, so it follows the reader's own text size and grows at ten feet.">
        {TYPE.map((t) => <div key={t} className="ds-type" style={{ fontSize: `var(--text-${t})` }}><code>--text-{t}</code> Fetching a film from the shelf</div>)}
      </Part>

      <Part title="Buttons" note="One primary per view: the accent means “do this next”.">
        <Row label="Variants"><Button variant="primary">Primary</Button><Button>Default</Button><Button variant="ghost">Ghost</Button><Button variant="danger">Danger</Button></Row>
        <Row label="Sizes"><Button size="sm">Small</Button><Button>Medium</Button><Button size="big">Large</Button></Row>
        <Row label="With icons"><Button variant="primary" icon={<IconPlay />}>Play</Button><Button icon={<IconDownload />}>Download</Button><LinkButton to="/" variant="ghost" icon={<IconFilm />}>A link</LinkButton></Row>
        <Row label="States"><Button disabled>Disabled</Button><Button busy>Saving</Button></Row>
        <Row label="Icon only"><IconButton label="Edit"><IconEdit /></IconButton><IconButton label="Search" round variant="default"><IconSearch /></IconButton><IconButton label="Delete" variant="danger"><IconTrash /></IconButton><IconButton label="Refresh" size="sm"><IconRefresh /></IconButton></Row>
        <Row label="Hint"><Tooltip text="Tooltips appear on hover and on keyboard or remote focus"><Button>Hover or focus me</Button></Tooltip></Row>
      </Part>

      <Part title="Fields" note="Field joins the label, hint and error to its control for screen readers.">
        <div className="ds-form">
          <Field label="Name" hint="As it appears on the home page"><Input value={name} onChange={(e) => setName(e.target.value)} placeholder="Movies" /></Field>
          <Field label="Folder" required error="That folder doesn't exist"><Input defaultValue="/media/nowhere" /></Field>
          <Field label="Kind"><Select defaultValue="movies" options={[{ value: 'movies', label: 'Movies' }, { value: 'shows', label: 'TV shows' }]} /></Field>
          <Field label="Notes"><Textarea rows={2} defaultValue="Anything to remember" /></Field>
          <Field label="Disabled"><Input disabled defaultValue="Can't be changed" /></Field>
        </div>
        <Row label="Switch"><Switch checked={sw} onChange={setSw} label="Example switch" /><Switch checked={false} disabled onChange={() => {}} label="Disabled switch" /></Row>
      </Part>

      <Part title="Choices" note="Arrow keys move within each; at the ends they carry on to what's beside it.">
        <Row label="Segmented"><Segmented label="Range" value={seg} onChange={setSeg} options={[{ value: 'day', label: 'Day' }, { value: 'week', label: 'Week' }, { value: 'month', label: 'Month' }]} /></Row>
        <Tabs label="Example tabs" value={tab} onChange={setTab} items={[{ value: 'one', label: 'Overview' }, { value: 'two', label: 'Cast' }, { value: 'three', label: 'Files' }]} />
        <Row label="Chips"><Chips label="Genres">{['Comedy', 'Drama', 'Horror'].map((c) => <Chip key={c} selected={chips.includes(c)} onClick={() => toggleChip(c)}>{c}</Chip>)}</Chips></Row>
        <Row label="Removable"><Chips label="Tags">{tags.map((t) => <Chip key={t} removeLabel={`Remove ${t}`} onRemove={() => setTags(tags.filter((x) => x !== t))}>{t}</Chip>)}</Chips></Row>
      </Part>

      <Part title="Overlays" note="Rendered into the page's top layer, so nothing can cover or clip them. Escape or Back closes; focus goes back.">
        <Row>
          <Button onClick={() => setDialog(true)}>Open a dialog</Button>
          <Button onClick={() => setSheet(true)}>Open a sheet</Button>
          <Button onClick={async () => { toast((await ask({ title: 'Remove this library?', body: 'Your files stay where they are; only the server forgets them.', confirm: 'Remove', danger: true })) ? 'Removed' : 'Kept') }}>Ask a question</Button>
          <Menu label="Example menu" align="start" trigger={(p) => <Button {...p} icon={<IconMore />}>Open a menu</Button>} items={[
            { key: 'h', kind: 'heading', label: 'Library' },
            { key: 'scan', label: 'Scan now', icon: <IconRefresh />, onSelect: () => toast('Scanning') },
            { key: 'set', label: 'Settings', icon: <IconSettings />, to: '/settings', hint: 'A link in a menu' },
            { key: 's', kind: 'separator' },
            { key: 'del', label: 'Remove', icon: <IconTrash />, danger: true, onSelect: () => toast('Removed') },
            { key: 'off', label: 'Not available', disabled: true },
          ]} />
        </Row>
        {dialog && <Dialog title="A dialog" description="It takes the keyboard while it's open." onClose={() => setDialog(false)} actions={<><Button variant="ghost" onClick={() => setDialog(false)}>Cancel</Button><Button variant="primary" onClick={() => setDialog(false)}>Save</Button></>}><Field label="Anything"><Input autoFocus /></Field></Dialog>}
        {sheet && <Sheet title="A sheet" onClose={() => setSheet(false)} actions={<Button variant="primary" onClick={() => setSheet(false)}>Done</Button>}><p>Beside the page on wide screens, up from the bottom on phones.</p></Sheet>}
      </Part>

      <Part title="Messages" note="A toast for what has just happened; a notice for what stays true.">
        <Row label="Toasts">
          <Button onClick={() => toast('Library saved')}>Plain</Button>
          <Button onClick={() => toast('Saved', { tone: 'ok' })}>Done</Button>
          <Button onClick={() => toast('That didn’t work', { tone: 'danger' })}>Failed</Button>
          <Button onClick={() => toast('Theme removed', { action: { label: 'Undo', run: () => toast('Restored') } })}>With an action</Button>
        </Row>
        <div className="ds-stack">
          <Notice title="New here">A notice stays on the page until it no longer applies.</Notice>
          <Notice tone="ok">The last scan found everything.</Notice>
          <Notice tone="warn" title="Running low" action={<Button size="sm">Look</Button>}>The disk has 2 GB left.</Notice>
          <Notice tone="danger" title="Couldn’t reach the tuner">Check that it’s switched on.</Notice>
        </div>
      </Part>

      <Part title="Cards" note="Poster cards move with the pointer or remote: see Settings, Appearance.">
        <Row label="Card"><Card title="A card" desc="A surface one step up from the page." actions={<IconButton label="More" size="sm"><IconMore /></IconButton>}><p className="ds-note">Anything goes in it.</p></Card></Row>
        <Shelf title="A shelf" to="/" toLabel="See all">
          {['The Lantern Keeper', 'North of Nowhere', 'A Quiet Harbour', 'Glass Mountain', 'Paper Tide', 'Second Summer', 'Hollow Bay', 'The Long Road'].map((t, i) => (
            <PosterCard key={t} to="/" title={t} sub={String(2000 + i * 3)} name={t} art={<Placeholder title={t} />} progress={i === 1 ? 40 : undefined} badges={i === 2 ? <span className="badge seen" role="img" aria-label="Watched"><IconCheck /></span> : undefined} />
          ))}
        </Shelf>
        <Row label="Shapes">
          <PosterCard shape="wide" onClick={() => toast('Chosen')} title="Wide, for episodes" sub="A button, not a link" art={<Placeholder title="wide" />} />
          <PosterCard shape="square" onClick={() => toast('Chosen')} title="Square, for albums" art={<Placeholder title="square" />} />
        </Row>
      </Part>

      <Part title="States" note="Every page has these three besides ready: loading, empty and failed.">
        <Row label="Loading"><Spinner /><div className="ds-skel"><Skeleton w="60%" h="1.25rem" /><Skeleton lines={3} /><Skeleton w={48} h={48} round /></div></Row>
        <SkeletonGrid count={4} />
        <div className="ds-states">
          <EmptyState icon={<IconFilm />} title="No films yet" text="Add a folder of films to get started." action={<Button variant="primary" icon={<IconPlus />}>Add a library</Button>} />
          <LoadError what="This page" onRetry={() => toast('Trying again')} />
        </div>
      </Part>

      <Part title="Progress">
        <div className="ds-stack">
          <Progress label="Scanning, known" value={40} showValue />
          <Progress label="Done" value={100} tone="ok" showValue />
          <Progress label="Almost full" value={86} tone="warn" showValue />
          <Progress label="Working" />
        </div>
        <Row label="Ring"><ProgressRing label="Forty percent" value={40} /><ProgressRing label="Working" /><ProgressRing label="Big" value={75} size={40} /></Row>
      </Part>

      <Part title="Tables" note="Sort from the header. On phones each row stacks, with the column's name beside each value.">
        <Card flush><DataTable caption="Example films" columns={COLUMNS} rows={FILMS} rowKey={(f) => f.id} initialSort={{ key: 'year', desc: true }} /></Card>
      </Part>
    </div>
  )
}
