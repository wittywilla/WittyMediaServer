import { Notice } from '../ui'

// A folder on another machine, chosen from a device that isn't the server: refused for a good
// reason, as the server would sign in there with its own account. This lays out the ways forward.
// The server's own desktop has no such limit, so nobody there sees it.
export default function OtherMachineHelp() {
  return (
    <Notice tone="warn" title="That folder is on another machine, so it can’t be chosen from this device">
      <p>To reach it, the server would have to sign in to that machine with its own account, and it won’t do that for a request from another device. There are three ways to use it:</p>
      <ol className="su-ways">
        <li><b>Use the server’s own screen.</b> Go to the computer the server runs on, open this same page in a browser there, and add the folder. It’s allowed from there.</li>
        <li><b>Make it look like a local folder.</b> On the server, give the share a drive letter on Windows, or mount it on Linux, FreeBSD or a Mac, then choose that here. If the server runs as a service on Windows, it may not see a drive letter you made for your own account, so use one of the other two ways.</li>
        <li><b>Change a setting, if this is your own storage.</b> Add <code>"allowNetworkPaths": true</code> to the server’s config file and restart it. Do this only if you trust everyone with an account that can change settings.</li>
      </ol>
      <p>A library that’s already on a share keeps working, whichever you choose.</p>
    </Notice>
  )
}
