// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { LoadError } from '../../components/common'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { Notice } from '../../ui'
import { PageHead, Row, Section } from './ui'
import { t, tk } from '../../i18n'

// Settings → Network: the port this server answers on at home, and what follows it.
export default function NetworkSettings() {
  const { view, setView, failed, retry } = useNetwork()
  if (!view) return failed ? <LoadError what={t('This page')} onRetry={retry} /> : null
  const from = view.from === 'settings' ? view.settingsFile : view.from === 'default' ? t('Nothing sets it, so it’s the default, 9797') : ''
  return (
    <>
      <PageHead title={tk('Network')} desc={t('The port this server answers on at home. Browsers and apps use it in the address, such as {example}.', { example: 'http://192.168.1.20:9797' })} />
      <Section title={tk('Port')} desc={t('Change it if another program on this computer needs 9797, or you’d rather use a port of your own. The server checks that the new one is free, and moves to it when it restarts.')}>
        <PortSetting view={view} onSaved={setView} />
        <Row label={t('Addresses at home')} desc={view.addresses.length ? t('Other devices on your home network open one of these.') : t('Right now this server only answers on this computer.')}>
          {view.addresses.length > 0 && <div className="port-addrs">{view.addresses.map((a) => <code key={a}>{a}</code>)}</div>}
        </Row>
        {!view.locked && (
          <Row label={t('Saved in')} desc={t('A plain text file. You can change the port there too, in any text editor, and then restart the server.')}>
            <code className="port-file">{view.saveFile}</code>
          </Row>
        )}
        {from && view.from !== 'settings' && <Row label={t('Where it comes from')} desc={from} />}
        {view.problems.length > 0 && (
          <Notice tone="warn" title={t('Some lines of the settings file were skipped')}>
            <ul className="port-problems">{view.problems.map((p) => <li key={p}>{p}</li>)}</ul>
          </Notice>
        )}
      </Section>
      <Section title={tk('What follows the port')} desc={t('Nothing else needs changing on this computer.')}>
        <ul className="port-follows">
          <li><b>{t('Apps on your home network')}</b> {t('find the server again by themselves. They ask on UDP port {port}, which stays the same.', { port: view.discoveryPort })}</li>
          <li><b>{t('The tray or menu-bar icon')}</b> {t('opens the new address.')}</li>
          <li><b>{t('Windows’ firewall')}</b>: {t('the rule the installer made lets the server in on any port. Another firewall, or a router rule you made yourself, needs the new port.')}</li>
          <li><b>{t('Remote access')}</b> {view.httpsPort ? t('keeps its own HTTPS port ({port}), so forwarding on your router stays as it is. A reverse proxy or a VPN address you set up yourself needs the new port.', { port: view.httpsPort }) : t('keeps its own HTTPS port, so forwarding on your router stays as it is. A reverse proxy or a VPN address you set up yourself needs the new port.')}</li>
          <li><b>{t('Bookmarks')}</b> {t('and devices you set up by typing the address need the new one.')}</li>
        </ul>
      </Section>
    </>
  )
}
