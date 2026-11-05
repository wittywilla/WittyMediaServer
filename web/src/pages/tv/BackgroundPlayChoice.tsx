import { setBackgroundChoice, useBackgroundPlay } from './live'

// "Keep playing when this tab is in the background", the same words in WittyTV's settings and
// in the player's. It's this device's, unlike the rest of WittyTV's look, which follows the
// account. See backgroundPlay.ts.
export default function BackgroundPlayChoice() {
  const { keep, choice, serverDefault } = useBackgroundPlay()
  return (
    <fieldset className="ctv-choice">
      <legend>Playing</legend>
      <div className="ctv-choice-checks">
        <label>
          <input type="checkbox" checked={keep} onChange={(e) => setBackgroundChoice(e.target.checked ? '1' : '0')} /> Keep playing when this tab is in the background
        </label>
      </div>
      <small>
        On, a channel carries on with its sound in a background tab or a minimised window. Off, it stops when you switch away, which frees a tuner,
        and starts again at what's on now when you come back. This device only.
        {choice && keep !== serverDefault && <> <button type="button" className="ctv-link-btn" onClick={() => setBackgroundChoice(null)}>Go back to this server's choice ({serverDefault ? 'on' : 'off'})</button></>}
      </small>
    </fieldset>
  )
}
