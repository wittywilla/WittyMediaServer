// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState, type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { IconCheck, IconDownload, IconEdit, IconFilm, IconMore, IconPlay, IconPlus, IconRefresh, IconSearch, IconSettings, IconTrash } from '../icons'
import { useApp } from '../state'
import {
  Button, Card, Chip, Chips, DataTable, Dialog, EmptyState, Field, IconButton, Input, LinkButton, LoadError, Menu, Notice, PosterCard, Progress, ProgressRing, Segmented, Select, Sheet, Shelf,
  Skeleton, SkeletonGrid, Spinner, Switch, Tabs, Textarea, Tooltip, ask, toast, type Column,
} from '../ui'
import { Placeholder } from '../components/common'
import { t, tk, tx } from '../i18n'

// Every shared part in one place, in whatever theme is in use, for administrators
// (/design). Look here first when something in the design changes. docs/DESIGN.md
// is the written version of this page.

// The English names are the anchors; they're shown with tx().
const SECTIONS = [tk('Colour'), tk('Type'), tk('Buttons'), tk('Fields'), tk('Choices'), tk('Overlays'), tk('Messages'), tk('Cards'), tk('States'), tk('Progress'), tk('Tables')]

function Part({ title, note, children }: { title: string; note?: ReactNode; children: ReactNode }) {
  return (
    <section className="ds-part" id={title.toLowerCase()} aria-labelledby={'ds-' + title}>
      <h2 id={'ds-' + title}>{tx(title)}</h2>
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
const columns = (): Column<Film>[] => [
  { key: 'title', header: t('Title'), cell: (f) => f.title, sort: (a, b) => a.title.localeCompare(b.title) },
  { key: 'year', header: t('Year'), cell: (f) => f.year, sort: (a, b) => a.year - b.year },
  { key: 'size', header: t('Size'), align: 'end', cell: (f) => t('{size} GB', { size: f.size }), sort: (a, b) => a.size - b.size },
]

export default function DesignPage() {
  const { user } = useApp()
  const [sw, setSw] = useState(true)
  const [seg, setSeg] = useState<'day' | 'week' | 'month'>('week')
  const [tab, setTab] = useState<'one' | 'two' | 'three'>('one')
  const [chips, setChips] = useState<string[]>([tk('Comedy')])
  const [tags, setTags] = useState([tk('Kids'), tk('Family')])
  const [dialog, setDialog] = useState(false)
  const [sheet, setSheet] = useState(false)
  const [name, setName] = useState('')
  if (!user?.isAdmin) return <Navigate to="/" replace />
  const toggleChip = (c: string) => setChips((l) => (l.includes(c) ? l.filter((x) => x !== c) : [...l, c]))
  return (
    <div className="ds-page">
      <header className="toolbar"><h1>{t('Design system')}</h1><span className="count">{t('Every shared part, in the theme in use')}</span></header>
      <nav className="ds-toc" aria-label={t('On this page')}>{SECTIONS.map((s) => <a key={s} href={'#' + s.toLowerCase()}>{tx(s)}</a>)}</nav>

      <Part title={tk('Colour')} note={t('Set by the theme. Hover, selected and tinted colours are worked out from these.')}>
        <div className="ds-swatches">{SWATCHES.map((c) => <div key={c} className="ds-swatch"><i data-swatch={c} style={{ background: `var(--${c})` }} /><code>--{c}</code></div>)}</div>
      </Part>

      <Part title={tk('Type')} note={t('One scale, in rem, so it follows the reader’s own text size and grows at ten feet.')}>
        {TYPE.map((sz) => <div key={sz} className="ds-type" style={{ fontSize: `var(--text-${sz})` }}><code>--text-{sz}</code> {t('Fetching a film from the shelf')}</div>)}
      </Part>

      <Part title={tk('Buttons')} note={t('One primary per view: the accent means “do this next”.')}>
        <Row label={t('Variants')}><Button variant="primary">{t('Primary')}</Button><Button>{t('Default')}</Button><Button variant="ghost">{t('Ghost')}</Button><Button variant="danger">{t('Danger')}</Button></Row>
        <Row label={t('Sizes')}><Button size="sm">{t('Small')}</Button><Button>{t('Medium')}</Button><Button size="big">{t('Large')}</Button></Row>
        <Row label={t('With icons')}><Button variant="primary" icon={<IconPlay />}>{t('Play')}</Button><Button icon={<IconDownload />}>{t('Download')}</Button><LinkButton to="/" variant="ghost" icon={<IconFilm />}>{t('A link')}</LinkButton></Row>
        <Row label={t('States')}><Button disabled>{t('Disabled')}</Button><Button busy>{t('Saving')}</Button></Row>
        <Row label={t('Icon only')}><IconButton label={t('Edit')}><IconEdit /></IconButton><IconButton label={t('Search')} round variant="default"><IconSearch /></IconButton><IconButton label={t('Delete')} variant="danger"><IconTrash /></IconButton><IconButton label={t('Refresh')} size="sm"><IconRefresh /></IconButton></Row>
        <Row label={t('Hint')}><Tooltip text={t('Tooltips appear on hover and on keyboard or remote focus')}><Button>{t('Hover or focus me')}</Button></Tooltip></Row>
      </Part>

      <Part title={tk('Fields')} note={t('Field joins the label, hint and error to its control for screen readers.')}>
        <div className="ds-form">
          <Field label={t('Name')} hint={t('As it appears on the home page')}><Input value={name} onChange={(e) => setName(e.target.value)} placeholder={t('Movies')} /></Field>
          <Field label={t('Folder')} required error={t('That folder doesn’t exist')}><Input defaultValue="/media/nowhere" /></Field>
          <Field label={t('Kind')}><Select defaultValue="movies" options={[{ value: 'movies', label: t('Movies') }, { value: 'shows', label: t('TV shows') }]} /></Field>
          <Field label={t('Notes')}><Textarea rows={2} defaultValue={t('Anything to remember')} /></Field>
          <Field label={t('Disabled')}><Input disabled defaultValue={t('Can’t be changed')} /></Field>
        </div>
        <Row label={t('Switch')}><Switch checked={sw} onChange={setSw} label={t('Example switch')} /><Switch checked={false} disabled onChange={() => {}} label={t('Disabled switch')} /></Row>
      </Part>

      <Part title={tk('Choices')} note={t('Arrow keys move within each; at the ends they carry on to what’s beside it.')}>
        <Row label={t('Segmented')}><Segmented label={t('Range')} value={seg} onChange={setSeg} options={[{ value: 'day', label: t('Day') }, { value: 'week', label: t('Week') }, { value: 'month', label: t('Month') }]} /></Row>
        <Tabs label={t('Example tabs')} value={tab} onChange={setTab} items={[{ value: 'one', label: t('Overview') }, { value: 'two', label: t('Cast') }, { value: 'three', label: t('Files') }]} />
        <Row label={t('Chips')}><Chips label={t('Genres')}>{[tk('Comedy'), tk('Drama'), tk('Horror')].map((c) => <Chip key={c} selected={chips.includes(c)} onClick={() => toggleChip(c)}>{tx(c)}</Chip>)}</Chips></Row>
        <Row label={t('Removable')}><Chips label={t('Tags')}>{tags.map((tag) => <Chip key={tag} removeLabel={t('Remove {name}', { name: tx(tag) })} onRemove={() => setTags(tags.filter((x) => x !== tag))}>{tx(tag)}</Chip>)}</Chips></Row>
      </Part>

      <Part title={tk('Overlays')} note={t('Rendered into the page’s top layer, so nothing can cover or clip them. Escape or Back closes; focus goes back.')}>
        <Row>
          <Button onClick={() => setDialog(true)}>{t('Open a dialog')}</Button>
          <Button onClick={() => setSheet(true)}>{t('Open a sheet')}</Button>
          <Button onClick={async () => { toast((await ask({ title: t('Remove this library?'), body: t('Your files stay where they are; only the server forgets them.'), confirm: t('Remove'), danger: true })) ? t('Removed') : t('Kept')) }}>{t('Ask a question')}</Button>
          <Menu label={t('Example menu')} align="start" trigger={(p) => <Button {...p} icon={<IconMore />}>{t('Open a menu')}</Button>} items={[
            { key: 'h', kind: 'heading', label: t('Library') },
            { key: 'scan', label: t('Scan now'), icon: <IconRefresh />, onSelect: () => toast(t('Scanning')) },
            { key: 'set', label: t('Settings'), icon: <IconSettings />, to: '/settings', hint: t('A link in a menu') },
            { key: 's', kind: 'separator' },
            { key: 'del', label: t('Remove'), icon: <IconTrash />, danger: true, onSelect: () => toast(t('Removed')) },
            { key: 'off', label: t('Not available'), disabled: true },
          ]} />
        </Row>
        {dialog && <Dialog title={t('A dialog')} description={t('It takes the keyboard while it’s open.')} onClose={() => setDialog(false)} actions={<><Button variant="ghost" onClick={() => setDialog(false)}>{t('Cancel')}</Button><Button variant="primary" onClick={() => setDialog(false)}>{t('Save')}</Button></>}><Field label={t('Anything')}><Input autoFocus /></Field></Dialog>}
        {sheet && <Sheet title={t('A sheet')} onClose={() => setSheet(false)} actions={<Button variant="primary" onClick={() => setSheet(false)}>{t('Done')}</Button>}><p>{t('Beside the page on wide screens, up from the bottom on phones.')}</p></Sheet>}
      </Part>

      <Part title={tk('Messages')} note={t('A toast for what has just happened; a notice for what stays true.')}>
        <Row label={t('Toasts')}>
          <Button onClick={() => toast(t('Library saved'))}>{t('Plain')}</Button>
          <Button onClick={() => toast(t('Saved'), { tone: 'ok' })}>{t('Done')}</Button>
          <Button onClick={() => toast(t('That didn’t work'), { tone: 'danger' })}>{t('Failed')}</Button>
          <Button onClick={() => toast(t('Theme removed'), { action: { label: t('Undo'), run: () => toast(t('Restored')) } })}>{t('With an action')}</Button>
        </Row>
        <div className="ds-stack">
          <Notice title={t('New here')}>{t('A notice stays on the page until it no longer applies.')}</Notice>
          <Notice tone="ok">{t('The last scan found everything.')}</Notice>
          <Notice tone="warn" title={t('Running low')} action={<Button size="sm">{t('Look')}</Button>}>{t('The disk has {size} left.', { size: t('{size} GB', { size: 2 }) })}</Notice>
          <Notice tone="danger" title={t('Couldn’t reach the tuner')}>{t('Check that it’s switched on.')}</Notice>
        </div>
      </Part>

      <Part title={tk('Cards')} note={t('Poster cards move with the pointer or remote: see Settings, Appearance.')}>
        <Row label={t('Card')}><Card title={t('A card')} desc={t('A surface one step up from the page.')} actions={<IconButton label={t('More')} size="sm"><IconMore /></IconButton>}><p className="ds-note">{t('Anything goes in it.')}</p></Card></Row>
        <Shelf title={t('A shelf')} to="/" toLabel={t('See all')}>
          {['The Lantern Keeper', 'North of Nowhere', 'A Quiet Harbour', 'Glass Mountain', 'Paper Tide', 'Second Summer', 'Hollow Bay', 'The Long Road'].map((title, i) => (
            <PosterCard key={title} to="/" title={title} sub={String(2000 + i * 3)} name={title} art={<Placeholder title={title} />} progress={i === 1 ? 40 : undefined} badges={i === 2 ? <span className="badge seen" role="img" aria-label={t('Watched')}><IconCheck /></span> : undefined} />
          ))}
        </Shelf>
        <Row label={t('Shapes')}>
          <PosterCard shape="wide" onClick={() => toast(t('Chosen'))} title={t('Wide, for episodes')} sub={t('A button, not a link')} art={<Placeholder title="wide" />} />
          <PosterCard shape="square" onClick={() => toast(t('Chosen'))} title={t('Square, for albums')} art={<Placeholder title="square" />} />
        </Row>
      </Part>

      <Part title={tk('States')} note={t('Every page has these three besides ready: loading, empty and failed.')}>
        <Row label={t('Loading')}><Spinner /><div className="ds-skel"><Skeleton w="60%" h="1.25rem" /><Skeleton lines={3} /><Skeleton w={48} h={48} round /></div></Row>
        <SkeletonGrid count={4} />
        <div className="ds-states">
          <EmptyState icon={<IconFilm />} title={t('No films yet')} text={t('Add a folder of films to get started.')} action={<Button variant="primary" icon={<IconPlus />}>{t('Add a library')}</Button>} />
          <LoadError what={t('This page')} onRetry={() => toast(t('Trying again'))} />
        </div>
      </Part>

      <Part title={tk('Progress')}>
        <div className="ds-stack">
          <Progress label={t('Scanning, known')} value={40} showValue />
          <Progress label={t('Done')} value={100} tone="ok" showValue />
          <Progress label={t('Almost full')} value={86} tone="warn" showValue />
          <Progress label={t('Working')} />
        </div>
        <Row label={t('Ring')}><ProgressRing label={t('Forty percent')} value={40} /><ProgressRing label={t('Working')} /><ProgressRing label={t('Big')} value={75} size={40} /></Row>
      </Part>

      <Part title={tk('Tables')} note={t('Sort from the header. On phones each row stacks, with the column’s name beside each value.')}>
        <Card flush><DataTable caption={t('Example films')} columns={columns()} rows={FILMS} rowKey={(f) => f.id} initialSort={{ key: 'year', desc: true }} /></Card>
      </Part>
    </div>
  )
}
