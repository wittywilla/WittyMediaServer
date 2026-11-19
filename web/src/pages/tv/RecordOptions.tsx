// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { useState } from 'react'
import { api } from '../../api'
import { announce } from '../../a11y'
import { toast } from '../../state'
import { Button, Dialog, Segmented, Select } from '../../ui'
import { fmtDay, fmtSpan, type GuideChannel, type Programme, type RecordingRule } from './tvApi'
import { t, tn } from '../../i18n'

// Recording, with its choices (tv-dvr-rules-2): a few minutes more at either end, and for every
// airing, only new episodes, on any channel, and how many to keep. The same dialog changes a
// series already set up, from the recordings page.

const PADS = [0, 1, 2, 5, 10, 15, 30, 60]
const KEEPS = [0, 1, 2, 3, 5, 10, 20]
const padWord = (m: number) => (m === 0 ? t('None') : t('{n} min', { n: m }))

type Props = { onClose: () => void; onSaved?: () => void } & (
  | { ch: GuideChannel; p: Programme; series: boolean; rule?: undefined }
  | { rule: RecordingRule; ch?: undefined; p?: undefined; series?: undefined }
)

export default function RecordOptions(props: Props) {
  const { rule, onClose, onSaved } = props
  const [series, setSeries] = useState(rule ? true : !!props.series)
  const [padBefore, setPadBefore] = useState(rule?.padBefore ?? 0)
  const [padAfter, setPadAfter] = useState(rule?.padAfter ?? 0)
  const [newOnly, setNewOnly] = useState(rule?.newOnly ?? false)
  const [anyChannel, setAnyChannel] = useState(rule?.anyChannel ?? false)
  const [keep, setKeep] = useState(rule?.keep ?? 0)
  const [busy, setBusy] = useState(false)
  const title = rule ? rule.title : props.p!.title
  const save = async () => {
    setBusy(true)
    try {
      const pads = { padBefore, padAfter }
      const choices = { ...pads, newOnly, anyChannel, keep }
      if (rule) {
        await api.put(`/recording-rules/${rule.id}`, choices)
        toast(t('{title}: changed', { title }))
      } else if (series) {
        await api.post('/recording-rules', { channel: props.ch!.key, title, ...choices })
        toast(anyChannel ? t('Every airing of {title}, on any channel, will be recorded', { title }) : t('Every airing of {title} will be recorded', { title }))
      } else {
        const p = props.p!
        await api.post('/recordings', { channel: props.ch!.key, title, sub: p.sub, start: p.start, stop: p.end, ...pads })
        toast(t('Recording {title}', { title }))
      }
      announce(rule ? t('{title} changed', { title }) : t('Recording {title}', { title }))
      onSaved?.()
      onClose()
    } catch (e) { toast((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <Dialog className="ctv-details" title={rule ? t('Series: {title}', { title }) : t('Record {title}', { title })} onClose={onClose}
      actions={<>
        <Button variant="ghost" onClick={onClose}>{t('Cancel')}</Button>
        <Button variant="primary" busy={busy} onClick={() => void save()}>{rule ? t('Save') : series ? t('Record every airing') : t('Record')}</Button>
      </>}>
      {!rule && <p className="muted">{t('Channel {number}, {name}', { number: props.ch!.number, name: props.ch!.name })} · {fmtDay(props.p!.start)}, {fmtSpan(props.p!)}</p>}
      {rule && <p className="muted">{rule.anyChannel ? t('On any channel') : t('On {channel}', { channel: rule.channel })}</p>}
      {!rule && (
        <Segmented<'one' | 'series'> label={t('What to record')} value={series ? 'series' : 'one'} onChange={(v) => setSeries(v === 'series')} className="ctv-tabs"
          options={[{ value: 'one', label: t('This programme') }, { value: 'series', label: t('Every airing') }]} />
      )}
      <div className="ctv-rec-opts">
        <label>{t('Start early')}
          <Select value={padBefore} onChange={(e) => setPadBefore(Number(e.target.value))}>
            {PADS.map((m) => <option key={m} value={m}>{padWord(m)}</option>)}
          </Select>
        </label>
        <label>{t('Keep going after it ends')}
          <Select value={padAfter} onChange={(e) => setPadAfter(Number(e.target.value))}>
            {PADS.map((m) => <option key={m} value={m}>{padWord(m)}</option>)}
          </Select>
        </label>
      </div>
      {series && (
        <fieldset className="ctv-rec-series">
          <legend>{t('Every airing')}</legend>
          <label><input type="checkbox" checked={newOnly} onChange={(e) => setNewOnly(e.target.checked)} /> {t('New episodes only: skip one this has already recorded')}</label>
          <label><input type="checkbox" checked={anyChannel} onChange={(e) => setAnyChannel(e.target.checked)} /> {t('On any channel, not only {channel}', { channel: rule ? rule.channel : props.ch!.name })}</label>
          <label>{t('Keep')}
            <Select value={keep} onChange={(e) => setKeep(Number(e.target.value))}>
              {KEEPS.map((n) => <option key={n} value={n}>{n === 0 ? t('Every recording') : tn('The newest one', 'The newest {count}', n)}</option>)}
            </Select>
          </label>
          {keep > 0 && <small className="muted">{t('Older recordings this series made are deleted when a newer one has finished.')}</small>}
        </fieldset>
      )}
      <p className="muted ctv-note">{t('Starting early and keeping going use the tuner for longer, so another recording or someone watching may not get one.')}</p>
    </Dialog>
  )
}
