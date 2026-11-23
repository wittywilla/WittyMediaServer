// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useEffect, useMemo, useState, type CSSProperties, type ReactNode } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import { api, isNativeShell, profileName, serverBase, setServerBase } from '../../../api'
import * as Icons from '../../../icons'
import { IconLogout, IconRight, IconUsers } from '../../../icons'
import { Avatar } from '../../AuthPage'
import { Brand, BrandMark } from '../../../components/common'
import { useApp } from '../../../state'
import { featureOff, useMyLimits } from '../../../myLimits'
import { settingsPageHidden } from '../../../profileWords'
import { AUTO_THEME, CUSTOM_THEME, deviceTheme } from '../../../theme'
import { useFeatures } from '../../../sdui/useSdui'
import { useSwitchProfile } from '../../../shell/SwitchProfile'
import { useSignOut } from '../../../shell/Account'
import { ask } from '../../../ui'
import { TV_LEVELS } from '../../../tvmode/levelTable'
import { OnAComputerDialog, tvElsewhere } from '../../../tvmode/OnAComputer'
import { setTvMode } from '../../../tvmode/mode'
import { t, tx } from '../../../i18n'
import { menuGroups, rowFor, rowLevel, rowSummary, type MenuGroup, type MenuRow, type MenuWho } from './model'
import { SETTINGS_MENU } from './table'
import './menu.css'

// The settings menu, drawn from the server's one list (settingsmenu.json) as the Android phone app draws its You
// page: group headings, then rows with a tinted icon tile, the name, what is set and a chevron. Three ways:
//   list  a phone, and the right of the wide page: each group one card of rows, as the phone app;
//   tv    TV mode: each row a tile of its own, as Android TV and Apple TV draw it;
//   nav   beside a settings page on a wide screen: the same rows, smaller, the open one marked.
// A row a television can't change (the TV table's desktopOnly) stays in its place, says what is set, and opens
// the "change this on a computer" panel with the address of that setting and its QR code.

type Variant = 'list' | 'tv' | 'nav'

// The desktop app's own rows (WittyMediaClient desktop/preload.js): its list of servers, From away, its log.
type DesktopSettings = { info: () => Promise<{ servers: number; away: boolean; awayWords?: string }>; open: (id: string) => void }
const desktop = (): DesktopSettings | null => ((window as unknown as { wittyClient?: { settings?: DesktopSettings } }).wittyClient?.settings) || null

const NATIVE_HERE = new Set(['servers', 'away', 'appLog'])

export type SettingsMenuState = {
  groups: MenuGroup[]
  value: (row: MenuRow) => string
  tv: boolean
}

// useSettingsMenu: the rows this person sees here, and what each says.
export function useSettingsMenu(tv: boolean): SettingsMenuState {
  const { user, libraries, version, serverTheme, update } = useApp()
  const limits = useMyLimits()
  const features = useFeatures()
  const [twoStep, setTwoStep] = useState<boolean | null | undefined>(undefined) // null: the server doesn't offer it
  const [devices, setDevices] = useState<number | null>(null)
  const [themes, setThemes] = useState<{ id: string; name: string }[]>([])
  const [looks, setLooks] = useState(0)
  const [app, setApp] = useState<{ servers: number; away: boolean; awayWords?: string } | null>(null)
  const adult = !!user && (user.isAdmin || !user.kind || user.kind === 'adult')
  useEffect(() => {
    if (!user) return
    let gone = false
    if (adult) api.get<{ on: boolean }>('/me/2fa').then((r) => { if (!gone) setTwoStep(!!r.on) }).catch(() => { if (!gone) setTwoStep(null) })
    api.get<unknown[]>('/sessions').then((l) => { if (!gone) setDevices(l.length) }).catch(() => {})
    api.get<{ themes: { id: string; name: string }[] }>('/themes').then((r) => { if (!gone) setThemes(r.themes) }).catch(() => {})
    api.get<{ experiences: unknown[] }>('/sdui/experiences').then((r) => { if (!gone) setLooks(r.experiences?.length || 0) }).catch(() => {})
    desktop()?.info().then((r) => { if (!gone) setApp(r) }).catch(() => {})
    return () => { gone = true }
  }, [user?.id, adult]) // eslint-disable-line react-hooks/exhaustive-deps
  const off = useMemo(() => {
    const s = new Set<string>()
    for (const f of ['sharing', 'games', 'remoteControl', 'syncplay', 'activity']) if (featureOff(limits, f)) s.add(f)
    if (user && settingsPageHidden('sharing', user)) s.add('sharing')
    if (user && settingsPageHidden('activity', user)) s.add('activity')
    return s
  }, [limits, user])
  const has = (f: string) => !!features?.has(f)
  const who: MenuWho = {
    admin: !!user?.isAdmin, adult, tv, twoStep: twoStep !== null,
    games: libraries.some((l) => l.type === 'games' && l.console !== 'rig'),
    app: !!desktop() || isNativeShell(), servers: !!desktop(), fromAway: !!app?.away,
    looks: looks > 1, layoutEditor: has('sdui-experiences'), moduleSettings: has('module-settings'),
    remote: has('remote-protocol'), parties: has('syncplay'), off,
  }
  // The web opens a row by its page; of the apps' own screens, only the desktop app's rows, inside the desktop app.
  const groups = menuGroups(SETTINGS_MENU, who, (r) => !!r.path || (!!r.native && NATIVE_HERE.has(r.native) && !!desktop()))
  const mine = deviceTheme()
  const themeName = mine === AUTO_THEME ? t('Automatic') : mine === CUSTOM_THEME ? t('Your own') : themes.find((x) => x.id === (mine || serverTheme))?.name || ''
  const value = (row: MenuRow): string => {
    switch (row.value) {
      case 'name': return user ? profileName(user) : ''
      case 'twoStep': return twoStep === undefined || twoStep === null ? '' : twoStep ? t('On') : t('Off')
      case 'devices': return devices === null ? '' : String(devices)
      case 'theme': return themeName
      case 'version': return update && user?.isAdmin ? t('Update available') : version || ''
      case 'servers': return app ? String(app.servers) : ''
      case 'away': return app?.awayWords || ''
      default: return ''
    }
  }
  return { groups, value, tv }
}

function RowIcon({ row, small }: { row: MenuRow; small?: boolean }) {
  const Ic = (Icons as unknown as Record<string, (p: { size?: number }) => ReactNode>)['Icon' + row.icon.web] || Icons.IconSettings
  const tint = row.tint === 'danger' ? 'var(--danger)' : row.tint
  return <span className="sm-ic" style={{ '--tint': tint } as CSSProperties} aria-hidden="true">{Ic({ size: small ? 16 : 20 })}</span>
}

// What a TV says is set on a row it can't change: the first line of the setting's summary, else where to change it.
function useRowSummaries(rows: MenuRow[], on: boolean): Record<string, string> {
  const [lines, setLines] = useState<Record<string, string>>({})
  const ids = on ? [...new Set(rows.map((r) => rowSummary(TV_LEVELS, r)).filter(Boolean))] : []
  const key = ids.join('|')
  useEffect(() => {
    if (!key) return
    let gone = false
    for (const id of key.split('|')) {
      api.get<{ lines: string[] }>('/sdui/tv-summary/' + encodeURIComponent(id))
        .then((r) => { if (!gone && r.lines?.[0]) setLines((was) => ({ ...was, [id]: r.lines[0] })) })
        .catch(() => { /* the row still says where to change it */ })
    }
    return () => { gone = true }
  }, [key])
  return lines
}

// SettingsRows: the groups, drawn one of the three ways.
export function SettingsRows({ menu, variant, firstFocus }: { menu: SettingsMenuState; variant: Variant; firstFocus?: boolean }) {
  const loc = useLocation()
  const nav = useNavigate()
  const [elsewhere, setElsewhere] = useState<MenuRow | null>(null)
  const rows = menu.groups.flatMap((g) => g.rows)
  const blocked = (r: MenuRow) => menu.tv && rowLevel(TV_LEVELS, r) === 'desktopOnly'
  const summaries = useRowSummaries(rows.filter(blocked), menu.tv)
  const here = variant === 'nav' ? rowFor(menu.groups, loc.pathname) : null
  const open = (r: MenuRow) => {
    if (blocked(r)) { setElsewhere(r); return }
    if (r.native && NATIVE_HERE.has(r.native) && desktop()) { desktop()!.open(r.native); return }
    if (r.path) nav(r.path)
  }
  const cls = variant === 'tv' ? 'tvm-row' : 'sm-row'
  return (
    <div className={'sm-groups sm-' + variant}>
      {menu.groups.map((g, gi) => (
        <section key={g.id} className="sm-group" aria-label={tx(g.title)} data-settings-group={g.id}>
          <h2 className="sm-group-h">{tx(g.title)}</h2>
          <div className="sm-rows" data-nav-group>
            {g.rows.map((r, ri) => {
              const away = blocked(r)
              const shown = away ? summaries[rowSummary(TV_LEVELS, r)] || tvElsewhere() : menu.value(r)
              const body = (
                <>
                  <RowIcon row={r} small={variant === 'nav'} />
                  <span className="sm-name">{tx(r.title)}</span>
                  {shown && variant !== 'nav' && <span className={'sm-value' + (away ? ' sm-elsewhere' : '')}>{shown}</span>}
                  <IconRight className="sm-go" />
                </>
              )
              const props = {
                className: cls + (r.tint === 'danger' ? ' danger' : '') + (here?.id === r.id ? ' on' : ''),
                'data-row': r.id,
                'data-tv-first': (firstFocus && gi === 0 && ri === 0) || undefined,
                'aria-current': here?.id === r.id ? ('page' as const) : undefined,
                title: variant === 'nav' && shown ? `${tx(r.title)}: ${shown}` : undefined,
              }
              // A page is a link (it can be opened in a new tab); the panel and the desktop app's own screens are buttons.
              return away || !r.path || (r.native && NATIVE_HERE.has(r.native) && desktop())
                ? <button key={r.id} type="button" {...props} onClick={() => open(r)}>{body}</button>
                : <Link key={r.id} to={r.path} {...props}>{body}</Link>
            })}
          </div>
        </section>
      ))}
      {elsewhere && <OnAComputerDialog title={tx(elsewhere.title)} part={elsewhere.part} path={elsewhere.path} summary={rowSummary(TV_LEVELS, elsewhere)} onClose={() => setElsewhere(null)} />}
    </div>
  )
}

// The person: their picture, names and role, and Switch profile. On a wide screen and a TV it also has Sign out
// (and Leave TV mode), as the Android TV and Apple TV You pages do; a phone has Sign out under This server.
export function SettingsPerson({ wide, tv }: { wide: boolean; tv: boolean }) {
  const { user } = useApp()
  const sw = useSwitchProfile()
  const out = useOut()
  const nav = useNavigate()
  if (!user) return null
  const shown = profileName(user)
  const role = user.isAdmin ? t('Administrator') : user.kind === 'kids' ? t('Kids') : user.kind === 'guest' ? t('Guest') : t('Adult')
  return (
    <section className={'sm-person' + (wide ? ' wide' : '')} aria-label={shown}>
      <Avatar name={shown || '?'} avatar={user.avatar} big />
      <div className="sm-person-name">{shown}</div>
      <div className="sm-person-sub">@{user.name} · {role}</div>
      <div className="sm-person-actions" data-nav-group>
        <button type="button" className="btn sm-pill" onClick={sw.start}><IconUsers /> {t('Switch profile')}</button>
        {sw.dialog}
        {tv && <button type="button" className="btn sm-pill" onClick={() => { setTvMode(false); nav('/', { replace: true }) }}>{t('Leave TV mode')}</button>}
        {wide && <button type="button" className="btn sm-pill danger" onClick={out}><IconLogout /> {t('Sign out')}</button>}
      </div>
    </section>
  )
}

function useOut() {
  const signOut = useSignOut()
  return async () => { if (await ask({ title: t('Sign out?'), confirm: t('Sign out'), danger: true })) signOut() }
}

// This server: its mark and name, its address and fingerprint, then Change server (in an app) and Sign out on a phone.
export function ThisServer({ signOut }: { signOut: boolean }) {
  const { brandName } = useApp()
  const out = useOut()
  const [print, setPrint] = useState('')
  useEffect(() => { api.get<{ fingerprint?: string }>('/capabilities').then((c) => setPrint(c.fingerprint || '')).catch(() => {}) }, [])
  const address = (serverBase() || window.location.origin).replace(/\/+$/, '')
  const change = () => { setServerBase(''); window.location.reload() }
  return (
    <section className="sm-group sm-server" aria-label={t('This server')}>
      <h2 className="sm-group-h">{t('This server')}</h2>
      <div className="sm-server-card">
        <BrandMark />
        <div className="sm-server-text">
          <b><Brand name={brandName} /></b>
          <span>{address}</span>
          {print && <small>{t('Server fingerprint {fingerprint}', { fingerprint: print })}</small>}
        </div>
      </div>
      {(isNativeShell() || signOut) && (
        <div className="sm-server-actions">
          {isNativeShell() && <button type="button" className="btn wide" onClick={change}>{t('Change server')}</button>}
          {signOut && <button type="button" className="btn wide danger" onClick={out}>{t('Sign out')}</button>}
        </div>
      )}
    </section>
  )
}

// SettingsHome: the whole page. A phone: one column, the person at the top. A wide screen and a TV: the adapted
// version Apple TV's You page made, the person on the left third and the groups on the right two-thirds.
export function SettingsHome({ wide, tv, menu }: { wide: boolean; tv: boolean; menu: SettingsMenuState }) {
  if (!wide) {
    return (
      <div className="sm-home">
        <h1 className="sr-only">{t('Settings')}</h1>
        <SettingsPerson wide={false} tv={false} />
        <SettingsRows menu={menu} variant="list" />
        <ThisServer signOut />
      </div>
    )
  }
  return (
    <div className={'sm-home wide' + (tv ? ' tv' : '')}>
      <h1 className="sr-only">{t('Settings')}</h1>
      <div className="sm-left"><SettingsPerson wide tv={tv} /></div>
      <div className="sm-right">
        <SettingsRows menu={menu} variant={tv ? 'tv' : 'list'} firstFocus={tv} />
        <ThisServer signOut={false} />
      </div>
    </div>
  )
}

// SettingsHomePage: the page with the menu worked out here (You on a phone, You in TV mode).
export function SettingsHomePage({ wide, tv }: { wide: boolean; tv: boolean }) {
  const menu = useSettingsMenu(tv)
  return <SettingsHome wide={wide} tv={tv} menu={menu} />
}
