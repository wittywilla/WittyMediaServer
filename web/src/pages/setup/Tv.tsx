import AddLiveTv from '../tv/AddLiveTv'
import { LaterNote, StepHead, type StepProps } from './parts'

// Live TV, for a server with WittyTuner: the same guided setup Settings has, which ends by
// coming back here. Skipping it changes nothing.
export function Tv({ info, next, back }: StepProps) {
  return (
    <section>
      <StepHead title="Live TV" lead="Add a TV tuner, an IPTV playlist or a single stream now, or leave it for later. Encrypted channels are shown as encrypted and never decrypted." />
      <AddLiveTv onFinish={next} onLeave={back} />
      <LaterNote info={info} />
    </section>
  )
}
