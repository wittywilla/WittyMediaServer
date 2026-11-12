// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 WittyWilla and contributors
import { Link } from 'react-router-dom'
import { Button } from '../../ui'
import { LaterNote, StepActions, StepHead, type StepProps } from './parts'

// Step: Remote access and apps. Reaching the server away from home, pairing a phone or a TV, and sharing between servers
// are each settings of their own; this says where, so nothing is left to find later.
export function Remote({ info, next, back }: StepProps) {
  return (
    <section>
      <StepHead title="Remote access and apps" lead="Use the server away from home, and on your phones, tablets and TVs. None of this is needed to start watching on this network." />
      <ul className="tvs-list">
        <li className="tvs-item"><div className="tvs-item-text"><b>Away from home</b><span>Turn on Remote Access, and choose how the server is reached. <Link to="/settings/remote">Settings, Remote Access</Link></span></div></li>
        <li className="tvs-item"><div className="tvs-item-text"><b>Pair an app</b><span>Phones and TVs connect with a code or a QR code. <Link to="/pair">Pair a device</Link></span></div></li>
      </ul>
      <LaterNote info={info} />
      <StepActions back={back}><Button variant="primary" size="big" onClick={next}>Continue</Button></StepActions>
    </section>
  )
}
