# Remote access

Watch and listen when you are away from home, without opening your server to the whole internet by
accident. Everything here happens in **Settings → Remote Access**, and everything is **off until you
turn it on**.

- [Which way should I choose?](#which-way-should-i-choose)
- [Tailscale or NetBird](#tailscale-or-netbird)
- [Forwarding a port on your router](#forwarding-a-port-on-your-router)
- [A reverse proxy you already run](#a-reverse-proxy-you-already-run)
- [A rented server (VPS)](#a-rented-server-vps)
- [Testing it from outside](#testing-it-from-outside)
- [Limits for viewers away from home](#limits-for-viewers-away-from-home)
- [Keeping it safe](#keeping-it-safe)
- [The apps](#the-apps)
- [What contacts what](#what-contacts-what)
- [How the server decides who is at home](#how-the-server-decides-who-is-at-home)
- [When it doesn't work](#when-it-doesnt-work)

## Which way should I choose?

| Way in | Good for | You need | Open to the internet? |
|---|---|---|---|
| **Tailscale** or **NetBird** (easiest) | Your own family's devices | An app on each device | **No.** Only your own signed-in devices can connect |
| **Forward a port on your router** | Any device, nothing to install | A domain name, a router that allows it, a public internet address | The sign-in page, over HTTPS |
| **Your own reverse proxy** | People who already run one | Caddy, nginx or similar | The sign-in page, over HTTPS |
| **A rented server (VPS)** | Keeping your home address private, or a provider that shares addresses | A small rented server and a domain name | The rented server; nothing at home |

If you aren't sure, choose **Tailscale**. It works even when your internet provider shares one address
between customers (see [below](#when-it-doesnt-work)), when port forwarding is impossible.

A **hosted relay** is shown greyed out on the page: it's a possible future paid service. It doesn't
exist, and nothing on your server uses it.

**The switch:** while *Remote Access* is off, the server refuses every connection from outside your
home network, including through a VPN, even if your router or firewall lets it through. Devices at home
are never affected.

## Tailscale or NetBird

These connect your own devices into a private network. Nothing is opened on your router, and video is
encrypted between your devices. You make a free account with them; they are separate services that are
not part of WittyMediaServer. NetBird can also be run entirely by yourself.

1. On the computer running the server, install [Tailscale](https://tailscale.com/download) or
   [NetBird](https://netbird.io/download) and sign in. The Remote Access page notices it and shows the
   computer's new address, such as `100.101.102.103`.
2. Install the same app on your phone or laptop, and sign in with the same account.
3. In *Settings → Remote Access*, turn on **Remote Access**.
4. On your phone, with the VPN app on, open `http://100.101.102.103:9797` (use your address). Bookmark
   it, or enter it in the WittyMedia app.
5. [Test it](#testing-it-from-outside).

The address starts with `http://`, not `https://`, and that's fine here: the VPN encrypts everything
itself.

Devices on your VPN count as *away from home*: they get the [limits](#limits-for-viewers-away-from-home)
and always need a password.

## Forwarding a port on your router

Anyone who has the address can reach the sign-in page, so this way needs HTTPS and strong passwords.
The server enforces the HTTPS part: it refuses unencrypted connections from the internet.

1. **Get a name for your home.** Home internet addresses change, so use a free dynamic DNS name (for
   example from DuckDNS or No-IP) or a domain of your own, pointing at your router's outside address.
2. **Choose a certificate**, under *Forward a port on your router*:
   - **Let's Encrypt** (free, renews by itself): enter your domain name and accept their agreement.
     Let's Encrypt learns your domain name and any email you give, and every certificate is listed
     publicly, so your domain name becomes public.
   - **My own certificate**: enter the full paths of the certificate and key files. When something renews
     the files, the server picks them up within a minute.

   This opens the HTTPS port, **9443** by default, beside the home port (9797), which stays as it is.
3. **Forward the port.** On your router, forward outside port **443** (TCP) to this computer's port
   **9443**. The router's manual calls this "port forwarding" or "virtual server". **Forward only this
   port**, never 9797.

   Or turn on **Do this automatically**, and the server asks the router to do it with UPnP or NAT-PMP,
   and closes it again when turned off or stopped. It's off by default, some routers don't offer it, and
   only the HTTPS port is ever opened. **Check my router** only asks the router what it can do, and
   changes nothing.

   Let's Encrypt connects to your outside port 443 to check that the domain is yours, so that port must
   lead here. With your own certificate, any outside port works.
4. Turn on **Remote Access**.
5. Press **Get the certificate now** if you chose Let's Encrypt, then [test it](#testing-it-from-outside).

On Windows, the firewall asks the first time the HTTPS port opens: allow it on *Private networks*.

## A reverse proxy you already run

Your proxy handles HTTPS and passes requests to the server's home port. Two settings matter, both under
*A reverse proxy you already run*:

- **The outside address**, such as `https://media.example.com`.
- **The proxy's address as this server sees it.** On the same machine, that's `127.0.0.1`. Forwarded
  headers are believed from these machines **only, and from nobody by default**. Until yours is listed,
  everyone behind it counts as one visitor from the internet: safe, but sign-in limits then hit everybody
  at once. The page offers to trust a proxy when it sees one at home.

The proxy must set `X-Forwarded-For` and `X-Forwarded-Proto`, as all the common ones do. If it doesn't
send `X-Forwarded-For`, everyone looks like the proxy itself, so don't list a proxy that doesn't. The
`Forwarded` header is never used.

**Caddy** (gets its own certificate, and sets the headers):

```caddyfile
media.example.com {
    reverse_proxy 127.0.0.1:9797
}
```

**nginx:**

```nginx
server {
    listen 443 ssl;
    server_name media.example.com;
    ssl_certificate     /etc/ssl/media/fullchain.pem;
    ssl_certificate_key /etc/ssl/media/privkey.pem;
    client_max_body_size 0;

    location / {
        proxy_pass http://127.0.0.1:9797;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Forwarded-Host $host;
        proxy_read_timeout 120s;
        proxy_buffering off;
    }
}
```

The proxy's access log records full addresses. The web app keeps sign-ins out of them when it is used
over HTTPS, but other apps may not, so treat those logs as private.

## A rented server (VPS)

**Your server at home, with a rented one in front.** Visitors reach the rented server; it passes them on
over a private tunnel, so no port opens at home and your home address stays private.

1. Rent a small server and point your domain name at it.
2. Install Tailscale or NetBird on both machines (or WireGuard, if you know it).
3. On the rented server, install Caddy and pass visitors to your home computer's private address:
   ```caddyfile
   media.example.com {
       reverse_proxy 100.101.102.103:9797
   }
   ```
4. In *Settings → Remote Access*, enter the outside address, and the rented server's address on the
   private network as the proxy.
5. Turn on **Remote Access**, and [test it](#testing-it-from-outside).

**The server itself on a rented machine.** There's no home network to set it up from, so either:

- use an SSH tunnel (`ssh -L 9797:127.0.0.1:9797 you@server`), open `http://localhost:9797` and go through
  setup and Settings as if at home; or
- start it with `-remote-access` (or `WMS_REMOTE_ACCESS=1`, or `"remoteAccess": true` in the
  [config file](CONFIG.md)). **Do this behind HTTPS, and create the first account right away:** the first
  person to reach a new server becomes its administrator.

Setting `publicUrl` also turns remote access on, so servers already set up behind a proxy keep working
after an update, but list the proxy in `trustedProxies` too.

## Testing it from outside

A server can't really tell from inside whether the internet can reach it, so the test uses **your own
phone**. No service on the internet is involved, and nothing is sent anywhere.

1. Press **Start the test**. The page shows a QR code and a secret address, good for 15 minutes.
2. On your phone, **turn off Wi-Fi** so it uses mobile data (keep your VPN on, if you use one).
3. Scan the code. The page on your phone says whether it worked, and the settings page updates by itself
   with where the connection came from and how.

The test page shows nothing about your server. A test from your home network is noticed, and doesn't
count.

**Check from this computer** looks at what can be seen from inside: whether your domain name leads to your
router, whether the HTTPS port is open, and whether the address answers. It only uses your computer's usual
name server. Many routers won't connect a computer at home to their own outside address, so a failure of the
last step doesn't mean it fails from outside.

The status at the top of the page shows what the server has really seen. It says *Reachable* only after a
passed test or a real encrypted connection from outside.

## Limits for viewers away from home

Home connections are often slow when sending. **Highest resolution** and **Most data per stream** apply to
anyone away from home, including over a VPN. Playback at home is never affected.

- Video over the limit is converted, with every encoder told in the way it really obeys: hardware encoders
  switch from constant quality to a target bitrate when limited. [Expert options](TRANSCODING.md) can't lift
  the limit (though options that make an encoder ignore limits may).
- A file over the limit isn't sent as it is, unless it fits.
- What is sent is also held to the limit, with about a quarter to spare so players can get ahead, so a file
  can't be fetched faster by asking for it directly.
- A viewer with limits never shares a stream with one without.

Choose a limit below your connection's upload speed, divided by how many people may watch at once.

## Keeping it safe

Always on:

- **Encryption from the internet.** Unencrypted requests from the internet are refused or sent to your HTTPS
  address, so a wrongly forwarded plain port can't expose your password. Browsers that have used HTTPS keep
  using it for a week (HSTS).
- **Sign-in limits.** After 5 wrong tries for one name from one address, each further try waits longer, up to 15
  minutes. Away from home there are also limits on one address trying many names (20 wrong tries, an IPv6 network
  counts as one address) and one name tried from many addresses (30). **Nobody on the internet can lock you
  out at home.** Wrong tries are shown on the page, where you can also clear the locks. Only a few passwords are
  checked at once, so a flood of attempts can't take the whole machine.
- **Cookies** are `HttpOnly`, `SameSite=Lax` and, over HTTPS, `Secure`.
- **Other websites** can't read answers from the server unless they hold a sign-in token, and can't act with
  your cookie. This matters at home too: any page you visit could otherwise ask a server on your network who its
  profiles are.
- **Password-free Kids and Guest profiles** only work at home, and profile names are not shown elsewhere unless
  you turn that on under *Settings → Profiles*. The version isn't shown to anyone not signed in on the internet.

For you to do:

- **Use a long password.** Remote Access warns when an administrator's password is weak. How strong a password
  is can only be known when someone signs in with it, so an account shows as *not checked yet* until then.
- **Turn on two-factor sign-in**, under *Settings → Sign-in & security*. Signing in then also asks for a 6-digit
  code from an authenticator app on your phone. It's optional, for each adult account. You get 8 recovery codes,
  for a lost phone. Keep them somewhere safe.
- **Look at your devices** on the same page, and sign out what you don't recognise. *People connected from
  outside* on the Remote Access page shows everyone's, for the administrator.

**Lost your phone and your recovery codes?** An administrator can turn off two-factor sign-in for someone else
under *Sign-in & security*. For an administrator, on the server itself, stop the server and run:

```bash
wms reset-2fa -data <your data folder> <name>
```

## The apps

The apps ask for your server's address, which can be either kind:

- **Desktop:** *Server → Change server…*. It warns before using an unencrypted address outside a private network.
- **Mobile:** asked the first time it starts; the sign-in screen has **Change server**.

## What contacts what

The server contacts nobody for remote access unless you choose one of these:

| You chose | Who is contacted | What they learn |
|---|---|---|
| Let's Encrypt | Let's Encrypt, to get and renew the certificate | Your domain name, any email you gave, your internet address. Certificates are public |
| Automatic port mapping, or *Check my router* | Your router, on your home network only | That this computer wants a port open |
| Tailscale, NetBird | These are their apps, not part of the server | See their privacy policies |
| *Check from this computer* | The name server your computer already uses, and your own address | Your domain name |

Nothing else. The outside test uses your phone.

## How the server decides who is at home

- **At home:** this computer, private addresses (`192.168.x.x`, `10.x.x.x`, `172.16–31.x.x`, `fc00::/7`, link-local),
  and IPv6 addresses on this computer's own network, if that network card also has a private IPv4 address.
- **VPN:** `100.64.0.0/10` and `fd7a:115c:a1e0::/48`, the addresses Tailscale and NetBird use.
- **Internet:** everything else.

Under *Advanced* you can add networks to either side. List a **WireGuard** or other VPN with private addresses
under *Count as away from home*, or it looks like home.

Forwarded headers are read only from the proxies you listed, from the right: everything to the left of the last
address that isn't a trusted proxy could have been written by the visitor. A request with forwarded headers from
anyone else counts as coming from the internet, even from this computer.

Some setups hide the real address, so outside visitors **look like they're at home**: some routers rewrite the
address on forwarded ports, and Docker Desktop shows every connection as coming from its own network. Such a
visitor could use password-free profiles. If your router does this, or on Docker Desktop, use a VPN or a reverse
proxy with `-network host`, and test with your phone: the test result shows how a connection was seen.

## When it doesn't work

| What you see | What it means | What to do |
|---|---|---|
| "Only your home network" | Remote Access is off | Turn it on |
| "This server is only available on its home network" in a browser | The server sees you as outside, and Remote Access is off | Turn it on from home. Behind a proxy, also list it as trusted |
| "This server only accepts encrypted connections…" | You came in over plain HTTP from the internet | Use the `https://` address; on the router, forward the HTTPS port and not 9797 |
| "Your router's outside address is shared" | Your provider shares one address between customers (CGNAT), or there's a second router in front | Port forwarding can't work. Use Tailscale, NetBird or a rented server, or ask your provider for a public address |
| "port … can't be opened" | Something else uses the HTTPS port; on Linux, ports below 1024 need extra rights | Choose another port |
| Let's Encrypt fails | Its servers couldn't reach port 443, or the name doesn't lead to your router | Check the name and the forward. Let's Encrypt limits failed tries to a few an hour, so wait before trying again |
| "Requests are arriving through a reverse proxy…" | A proxy that isn't trusted | Press **Trust**, if it's yours |
| Someone reached "the plain, unencrypted port" | Port 9797 is open on your router | Close it: only the HTTPS port should be forwarded |
| Too many wrong tries | Sign-in limits | Wait, try from home, or press **Let everyone try again** |
| The test works, but video stutters | Not enough upload speed | Lower the [limits](#limits-for-viewers-away-from-home) |
