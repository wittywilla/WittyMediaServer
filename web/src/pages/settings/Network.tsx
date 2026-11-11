// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { LoadError } from '../../components/common'
import { PortSetting, useNetwork } from '../../components/PortSetting'
import { Notice } from '../../ui'
import { PageHead, Row, Section } from './ui'

// Settings → Network: the port this server answers on at home, and what follows it.
export default function NetworkSettings() {
  const { view, setView, failed, retry } = useNetwork()
  if (!view) return failed ? <LoadError what="This page" onRetry={retry} /> : null
  const from = view.from === 'settings' ? view.settingsFile : view.from === 'default' ? 'Nothing sets it, so it’s the default, 9797' : ''
  return (
    <>
      <PageHead title="Network" desc="The port this server answers on at home. Browsers and apps use it in the address, such as http://192.168.1.20:9797." />
      <Section title="Port" desc="Change it if another program on this computer needs 9797, or you’d rather use a port of your own. The server checks that the new one is free, and moves to it when it restarts.">
        <PortSetting view={view} onSaved={setView} />
        <Row label="Addresses at home" desc={view.addresses.length ? 'Other devices on your home network open one of these.' : 'Right now this server only answers on this computer.'}>
          {view.addresses.length > 0 && <div className="port-addrs">{view.addresses.map((a) => <code key={a}>{a}</code>)}</div>}
        </Row>
        {!view.locked && (
          <Row label="Saved in" desc="A plain text file. You can change the port there too, in any text editor, and then restart the server.">
            <code className="port-file">{view.saveFile}</code>
          </Row>
        )}
        {from && view.from !== 'settings' && <Row label="Where it comes from" desc={from} />}
        {view.problems.length > 0 && (
          <Notice tone="warn" title="Some lines of the settings file were skipped">
            <ul className="port-problems">{view.problems.map((p) => <li key={p}>{p}</li>)}</ul>
          </Notice>
        )}
      </Section>
      <Section title="What follows the port" desc="Nothing else needs changing on this computer.">
        <ul className="port-follows">
          <li><b>Apps on your home network</b> find the server again by themselves. They ask on UDP port {view.discoveryPort}, which stays the same.</li>
          <li><b>The tray or menu-bar icon</b> opens the new address.</li>
          <li><b>Windows’ firewall</b>: the rule the installer made lets the server in on any port. Another firewall, or a router rule you made yourself, needs the new port.</li>
          <li><b>Remote Access</b> keeps its own HTTPS port{view.httpsPort ? ` (${view.httpsPort})` : ''}, so forwarding on your router stays as it is. A reverse proxy or a VPN address you set up yourself needs the new port.</li>
          <li><b>Bookmarks</b> and devices you set up by typing the address need the new one.</li>
        </ul>
      </Section>
    </>
  )
}
