# Remote access: a security guide

This guide explains how to reach your server when you're away from home, and what can go wrong when you do.
It's written the way a security researcher would look at your setup: **assume someone is trying to get in, and
choose what still holds up when they do.** You don't need any IT experience. Every term is explained, and every
step is spelled out.

> [!CAUTION]
> **READ THIS BEFORE YOU TURN ANYTHING ON**
>
> **The safest remote access is none.** WittyMediaServer works fully at home with everything on this page turned
> off, and that's how it starts.
>
> **WittyMediaServer is alpha software.** It hasn't had an independent security audit. Our own code reviews keep
> finding real bugs, including security bugs, and there are surely more we haven't found. Treat every protection
> described here as something that helps, not a guarantee.
>
> **A server on the internet gets attacked. This isn't paranoia.** Automated programs constantly scan every
> address on the internet. They don't care who you are. Expect the first probes soon after you open a port, and
> expect them to continue for as long as it stays open.
>
> **If someone breaks in, they may get:** your accounts and everyone's watch history, your saved API keys, your
> media and anything else your server's user account can read, and a foothold inside your home network, next to
> your other computers, phones and cameras.
>
> **This software comes with no warranty. You run it, so what you expose, who you let in and what you host are
> your responsibility.** The WittyMediaServer project doesn't host, see or check any of it, can't protect a server
> you've exposed, and isn't responsible for what happens to it. The law where you and your visitors are applies,
> including to sharing media. See *Settings → Legal*. This guide is general advice: it isn't legal advice, or a
> professional security assessment of your home.

**In a hurry?** Use [Tailscale](#the-recommended-way-a-private-network). Don't forward ports. Turn on
[two-factor sign-in](#step-2-lock-down-your-accounts). Then run [the phone test](#testing-it-from-outside).

## Contents

- [The words you need](#the-words-you-need)
- [Who you're defending against](#who-youre-defending-against)
- [Which way should I choose?](#which-way-should-i-choose)
- [Things you must never do](#things-you-must-never-do)
- **Do these first, whichever way you choose**
  - [Step 1: Do you really need this?](#step-1-do-you-really-need-this)
  - [Step 2: Lock down your accounts](#step-2-lock-down-your-accounts)
  - [Step 3: Reduce what an intruder would get](#step-3-reduce-what-an-intruder-would-get)
- **The ways in, safest first**
  - [The recommended way: a private network](#the-recommended-way-a-private-network)
  - [A rented server (VPS)](#a-rented-server-vps)
  - [A reverse proxy you already run](#a-reverse-proxy-you-already-run)
  - [Forwarding a port on your router](#forwarding-a-port-on-your-router)
- **After it works**
  - [Testing it from outside](#testing-it-from-outside)
  - [Unsafe settings, and what each one risks](#unsafe-settings-and-what-each-one-risks)
  - [Letting other people in](#letting-other-people-in)
  - [Regular checks](#regular-checks)
  - [If you think someone got in](#if-you-think-someone-got-in)
- **Reference**
  - [What the server protects by itself, and what it doesn't](#what-the-server-protects-by-itself-and-what-it-doesnt)
  - [Limits for viewers away from home](#limits-for-viewers-away-from-home)
  - [The apps](#the-apps)
  - [Games away from home](#games-away-from-home)
  - [What contacts what](#what-contacts-what)
  - [How the server decides who is at home](#how-the-server-decides-who-is-at-home)
  - [When it doesn't work](#when-it-doesnt-work)

## The words you need

| Word | What it means |
|---|---|
| **Router** | The box your internet comes through. It keeps the internet out of your home network unless told otherwise |
| **Home network** | Your devices connected to that router, by Wi-Fi or cable. Anyone on your Wi-Fi counts as "at home" |
| **Address (IP address)** | A device's number on a network. Your home devices usually have private ones such as `192.168.1.20`. Your router has a public one that the whole internet can see |
| **Port** | A numbered door on an address. WittyMediaServer's home door is **9797**, and its encrypted door is **9443** by default |
| **Port forwarding** | Telling your router to send visitors from the internet through a door to a device at home. **This is what makes your server visible to the whole internet** |
| **VPN / private network** | An encrypted connection that joins your own devices together. Tailscale and NetBird are the ones recommended here. The internet can't see a server reached this way |
| **HTTPS** | Encrypted web traffic, the `https://` at the start of an address. It stops people on the way from reading your password. **It doesn't stop anyone from trying to sign in** |
| **Certificate** | The file that makes HTTPS work and proves the address really is yours |
| **Domain name** | A name such as `media.example.com` that points at an address |
| **Reverse proxy** | A program that stands in front of the server, takes visitors from the internet, and passes them on |
| **VPS** | A small computer you rent in a data center |
| **Two-factor sign-in** | Signing in needs your password and a 6-digit code from an app on your phone |

## Who you're defending against

A good defense starts by naming the attacker. These are the realistic ones, most likely first.

| Who | What they do | What stops them |
|---|---|---|
| **Automated scanners** | Find open ports across the internet, identify the software and try known bugs. It costs them nothing | Not being on the internet at all: a [private network](#the-recommended-way-a-private-network). Otherwise, staying up to date |
| **Password guessers** | Try passwords leaked from other websites against your sign-in page | A password you use nowhere else, plus [two-factor sign-in](#step-2-lock-down-your-accounts) |
| **Someone you invited, or a device they lost** | A friend's reused password, a stolen phone still signed in, a shared link that got passed on | [Accounts with limits](#letting-other-people-in), never admin. [Regular checks](#regular-checks) of who's connected |
| **Someone tricking you** | Sends a pairing code or link and asks you to approve it | Only ever approve a code you started yourself |
| **Someone on your network** | A visitor on your Wi-Fi, or a hacked device at home. At home the server trusts more | A guest Wi-Fi for visitors. Passwords on every adult account |
| **Someone targeting you personally** | An ex, a neighbor, a person you've annoyed online. They know you and may know your passwords | New passwords, two-factor sign-in, no port forwarding. **If your safety is at stake, get professional help, and don't rely on this guide** |

## Which way should I choose?

| | Way in | Seen by the internet? | Skill needed | Our view |
|---|---|---|---|---|
| 🟢 | **Stay at home** (Remote Access off) | **No** | None | Safest. The default |
| 🟢 | **[Tailscale or NetBird](#the-recommended-way-a-private-network)** | **No.** Only your own signed-in devices can connect | Low: install an app | **Use this.** It's right for almost everyone |
| 🟡 | **[A rented server (VPS)](#a-rented-server-vps)** | The rented server, not your home | High, and you now look after a server on the internet | For people who need visitors with no app, and know how to keep a server safe |
| 🟡 | **[Your own reverse proxy](#a-reverse-proxy-you-already-run)** | **Yes**, the sign-in page | High | Only if you already run one and keep it updated |
| 🔴 | **[Forwarding a port](#forwarding-a-port-on-your-router)** | **Yes**, the sign-in page, directly from your home | Medium to do, high to do safely | **Not recommended.** The easiest way to get this wrong |

**If you aren't sure, choose Tailscale.** It's easier than port forwarding, and it also works when your internet
provider shares one address between customers, where port forwarding can't work at all.

The hosted relay shown greyed out on the page doesn't exist. It's a possible future service, and nothing on your
server uses it.

**The master switch:** while *Remote Access* is off, the server refuses every connection from outside your home
network, including through a VPN, even if your router or firewall lets it through. Devices at home aren't affected.

## Things you must never do

> [!CAUTION]
> **None of these is ever the right fix, however much a forum post insists.**
>
> - **Never forward port 9797.** It's the unencrypted door, and it's meant only for your home network.
> - **Never turn on "DMZ" on your router for this computer.** It exposes every port on the machine to the internet,
>   not only this server's.
> - **Never forward Sunshine's ports, and especially never port 47990.** Its web page can run commands on that PC.
>   See [Games away from home](#games-away-from-home).
> - **Never click through a browser's certificate warning** to reach your own server when you're away. That warning
>   may be the only sign that someone is reading your password.
> - **Never turn off your computer's firewall** to make this work. Fix the rule instead.
> - **Never run the server as administrator or root** because something didn't work. A bug then becomes control of
>   the whole machine.
> - **Never reuse a password** for this server, and never use a password that has been in a leak.
> - **Never give anyone your admin account**, or make a friend an administrator.
> - **Never approve a pairing code** you didn't start yourself, whoever asks.
> - **Never expose a server that hasn't finished setup.** The first person to complete setup becomes its administrator.
> - **Never post your server's address** publicly, or in a screenshot, a forum post or a support request.
> - **Never send your data folder or database to anyone.** It holds your settings and API keys. For help, use the
>   support bundle in *Settings → WittyLog*, and read it before you send it.

## Step 1: Do you really need this?

Ask this honestly, because every option below adds risk you don't have at home.

- **Watching in another room:** you don't need any of this. That's already your home network.
- **A trip or two a year:** turn Remote Access on before you go and off when you're back.
- **You regularly watch away from home:** use a [private network](#the-recommended-way-a-private-network).
- **Family in another house:** a private network still works. Each person installs one app and signs in.

## Step 2: Lock down your accounts

Do all of this **before** you turn on Remote Access.

1. **Use a long, unique password on every adult account.** Four or more unrelated random words is good, or better,
   let a password manager make one. *Remote Access* warns about a weak administrator password, but it only learns
   how strong a password is when someone signs in with it, so an account shows as *not checked yet* until then.
2. **Turn on two-factor sign-in.** This is the most useful thing you can do after a good password.
   1. Install an authenticator app on your phone, such as Aegis, Ente Auth, Google Authenticator or Microsoft
      Authenticator.
   2. Go to *Settings → Sign-in & security → Two-factor sign-in* and follow the steps.
   3. **Save your 8 recovery codes somewhere that isn't your phone**, such as on paper in a safe place. They get you
      back in if you lose your phone.
   4. Ask every adult on your server to do the same. It's set up per person, and nothing forces it.
3. **Use a normal account day to day.** Make a second adult account with no admin rights for watching, and keep the
   administrator account for changing settings. Signing in as admin on hotel Wi-Fi or a friend's TV is a risk you can
   simply avoid.
4. **Delete accounts nobody uses.** Every account is another password that can be guessed.
5. **Look at *Settings → Sign-in & security → Your devices*.** Sign out anything you don't recognize.
6. **Protect your own email and your phone.** A private network's account usually hangs off them, so anyone who gets
   into that account can add a device to your private network.

**Lost your phone and your recovery codes?** An administrator can turn off two-factor sign-in for someone else under
*Sign-in & security*. For an administrator, on the server itself, stop the server and run:

```bash
wms reset-2fa -data <your data folder> <name>
```

## Step 3: Reduce what an intruder would get

Assume that one day someone gets in. These decide how bad that day is.

1. **Update first, and keep updating.** Attackers read the same fix notes you do. Check *Settings → About* and install
   updates soon after they appear. The same goes for your computer's system and your router.
2. **Run the server as a normal user**, not an administrator. On Linux, give it a user of its own.
3. **Don't let it read what it doesn't need.** Point libraries at media folders only, and never at a whole drive, your
   home folder or a folder with documents and photos you'd hate to lose.
4. **Make media read-only for the server if you can.** This stops an intruder deleting your collection. Saving posters
   and info files beside your media then won't work, so turn that off under *Settings → Metadata agents → Saving
   information*. Uploads to those folders won't work either.
5. **Keep backups that the server can't reach.** A copy on a drive you unplug can't be deleted or encrypted by someone
   who breaks in.
6. **Turn off what you don't use:**
   - Share links and uploads: *Settings → Sharing & photos*.
   - Custom game cores stay off unless you allow them, and that's the right setting: they're programs from outside this
     project, running on your server.
   - Game PCs: see [Letting other people in](#letting-other-people-in).
7. **Change your router's admin password, and turn off its "remote administration".** A router you can manage from the
   internet is a serious target in its own right.
8. **Put visitors on guest Wi-Fi.** The server trusts your home network more, and your router's guest network is the
   easy way to keep visitors off it.

## The recommended way: a private network

**Tailscale** and **NetBird** connect your own devices into a private network. **Nothing is opened on your router**, so
scanners on the internet find nothing, and video is encrypted between your devices. Both are separate services and
aren't part of WittyMediaServer. You make an account with them and rely on them to decide which devices join, so read
their terms and privacy policies. Plans and prices can change, so check those too. NetBird can also be run entirely by
yourself, if you know how.

**Before you start:** do [Step 2](#step-2-lock-down-your-accounts) and [Step 3](#step-3-reduce-what-an-intruder-would-get).

1. **On the computer running the server**, install [Tailscale](https://tailscale.com/download) or
   [NetBird](https://netbird.io/download), and sign in. Use an account protected with two-factor sign-in.
2. **Keep it running.**
   - **Windows with Tailscale:** in its tray menu, turn on *Run unattended*, so it stays connected when nobody is signed
     in to the computer.
   - **Tailscale:** by default a device's sign-in expires after some months. In its admin console, turn off key expiry
     for the server, or remote access will stop by itself while you're away.
3. **Find the server's new address.** *Settings → Remote Access* notices the VPN and shows it, such as
   `100.101.102.103`.
4. **On your phone or laptop**, install the same app and sign in with the **same account**.
5. **In *Settings → Remote Access***, turn on **Remote Access**.
6. **On your phone, with the VPN app on,** open `http://100.101.102.103:9797`, using your own address. Bookmark it, or
   enter it in the WittyMedia app.
7. **[Test it](#testing-it-from-outside).**

The address starts with `http://`, not `https://`, and that's fine here, because the VPN encrypts everything itself.
This is the one situation where an unencrypted address is OK.

Devices on your VPN count as *away from home*: they get the [limits](#limits-for-viewers-away-from-home) and always need
a password.

> [!WARNING]
> **Ways to undo this safety:**
> - **Tailscale Funnel** publishes a port to the whole internet. Don't use it for this server, or your private server
>   becomes a public one.
> - **Sharing a device with someone** lets them reach that machine. Check what else it offers before you do, and use
>   their access controls to allow only port 9797.
> - **Everyone on your private network can reach the server's sign-in page.** Remove old phones and laptops from the
>   device list. A forgotten device is a way in.

**Why not a "tunnel" service?** Some companies offer a public address that tunnels to your home with no open port. It
feels like a private network, but **it's a public sign-in page on the internet, the same as port forwarding**. Their
servers can often also see everything passing through, including passwords, and their terms may restrict streaming video.

## A rented server (VPS)

> [!WARNING]
> **Advanced. You're now running a computer on the public internet, and keeping it safe is your job.** It needs system
> updates, a firewall and SSH keys. If those words are new to you, use a [private network](#the-recommended-way-a-private-network)
> instead. **Anyone can still reach your sign-in page**, so everything under [port forwarding](#forwarding-a-port-on-your-router)
> about passwords and attacks applies here too.

**Your server at home, with a rented one in front.** Visitors reach the rented server, and it passes them on over a private
tunnel. No port opens at home, and your home address stays private. This is the better of the two.

1. Rent a small server and point your domain name at it.
2. On the rented server: install updates, turn on automatic security updates, allow SSH sign-in by key only, and turn on a
   firewall that allows only ports 22, 80 and 443.
3. Install Tailscale or NetBird on both machines (or WireGuard, if you know it).
4. On the rented server, install Caddy, and pass visitors to your home computer's private address:
   ```caddyfile
   media.example.com {
       reverse_proxy 100.101.102.103:9797
   }
   ```
5. In *Settings → Remote Access*, enter the outside address, and the rented server's address on the private network as the
   proxy.
6. Turn on **Remote Access**, and [test it](#testing-it-from-outside).

**The server itself on a rented machine.** There's no home network to set it up from. Your media and accounts then live on
a computer you don't physically control, so think about who runs it and what their terms allow.

- **Safer: use an SSH tunnel.** Run `ssh -L 9797:127.0.0.1:9797 you@server`, open `http://localhost:9797`, and go through
  setup and Settings as if at home.
- **Riskier: start it with `-remote-access`** (or `WMS_REMOTE_ACCESS=1`, or `"remoteAccess": true` in the
  [config file](CONFIG.md)). This turns on Remote Access and locks the switch on the page.

> [!CAUTION]
> **`-remote-access` on a server that isn't set up yet:** whoever completes setup becomes its administrator. From outside,
> setup needs the setup code on the server's console, and wrong codes are limited, but **don't leave it sitting there.**
> Have HTTPS in front of it first, then create the first account right away. **Setting `publicUrl` also turns remote access
> on**, so servers already behind a proxy keep working after an update. Don't set it by accident, and list the proxy in
> `trustedProxies` too.

**Docker on a rented server:** Docker's published ports can go around firewalls such as `ufw`. Publish only to this machine
with `127.0.0.1:9797:9797`, and let the reverse proxy reach it there. Then check from another network that port 9797 doesn't
answer.

## A reverse proxy you already run

> [!WARNING]
> **Advanced, and on the internet.** This is for people who already run a reverse proxy and keep it updated. **Your sign-in
> page is public, and everything under [port forwarding](#forwarding-a-port-on-your-router) about passwords and attacks applies
> here too.** Getting the trusted proxy setting wrong can make strangers look like they're at home.

Your proxy handles HTTPS and passes requests to the server's home port. Two settings matter, both under *A reverse proxy you
already run*:

- **The outside address**, such as `https://media.example.com`.
- **The proxy's address as this server sees it.** On the same machine, that's `127.0.0.1`. Forwarded headers are believed from
  these machines **only, and from nobody by default**. Until yours is listed, everyone behind it counts as one visitor from the
  internet: safe, but sign-in limits then hit everybody at once. The page offers to trust a proxy when it sees one at home.

> [!CAUTION]
> **List only the exact address of your own proxy.** Never list a whole network you don't fully control. A trusted machine can
> claim any visitor address, including a home address, and **someone claiming to be at home can see profile names and use
> password-free profiles.**

The proxy must set `X-Forwarded-For` and `X-Forwarded-Proto`, as all the common ones do. If it doesn't send `X-Forwarded-For`,
everyone looks like the proxy itself, so don't list a proxy that doesn't. The `Forwarded` header is never used.

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

**Treat the proxy's access logs as private.** They record full addresses. The web app keeps sign-ins out of them when it's used
over HTTPS, but other apps may not.

`client_max_body_size 0` lets large uploads through. If you don't use uploads, set a limit, and turn uploads off too.

## Forwarding a port on your router

> [!CAUTION]
> **NOT RECOMMENDED. THIS PUTS YOUR SIGN-IN PAGE ON THE PUBLIC INTERNET, RUNNING FROM INSIDE YOUR HOME.**
>
> - **Anyone in the world can reach it.** Scanners will find it, and keep trying for as long as it's open.
> - **Any serious bug in this alpha software, or in what it's built on, may be reachable by strangers**, with no password needed.
>   HTTPS and a strong password don't protect against that kind of bug.
> - **Getting in means being inside your home network**, next to your other devices.
> - **Using Let's Encrypt makes your domain name public**, because every certificate is listed in public logs. Scanners watch
>   those logs, so a new name can get visitors very quickly.
> - **Your home address is exposed** to anyone who looks up that name.
> - **Your internet provider's terms may forbid running a server.** Check them.
>
> **Use a [private network](#the-recommended-way-a-private-network) unless you can say exactly why you can't.** "It's easier" isn't
> a good reason: setting up a private network is less work than doing this safely.

If you go ahead, do **all** of this. Do [Step 2](#step-2-lock-down-your-accounts) and
[Step 3](#step-3-reduce-what-an-intruder-would-get) first: treat two-factor sign-in on every adult account as required here.

The server enforces one part itself: it refuses unencrypted connections from the internet.

1. **Check that it can work at all.** In the *Forward a port on your router* section, press **Check my router**. This only asks the
   router what it can do, and changes nothing. If it says your outside address is shared (CGNAT), port forwarding can't work.
   [Use a private network](#the-recommended-way-a-private-network).

2. **Give this computer a fixed home address.** Otherwise the router may later send the internet to a different device.
   1. Find the computer's address. **Windows:** open Command Prompt and run `ipconfig`, then look for *IPv4 Address*. **Mac:**
      *System Settings → Network → your connection → Details*. **Linux:** run `ip addr`.
   2. Sign in to your router. The address is usually on its label, often `192.168.1.1` or `192.168.0.1`. **If it still has the
      password from the label or the factory, change that first.**
   3. Find the setting called *DHCP reservation*, *address reservation* or *static lease*, and reserve this address for the
      computer. Router menus differ, so check the manual for your model.

3. **Get a name for your home.** Home internet addresses change, so use a dynamic DNS name (for example from DuckDNS or No-IP) or a
   domain of your own, pointing at your router's outside address. Dynamic DNS needs something to keep the name up to date, usually
   a setting on your router or a small program from that service. The server doesn't do this.

4. **Choose a certificate**, under *Forward a port on your router*:
   - **Let's Encrypt** (free, renews by itself): enter your domain name and accept their agreement. Let's Encrypt learns your domain
     name and any email you give, and the certificate is listed publicly.
   - **My own certificate**: enter the full paths of the certificate and key files. When something renews the files, the server picks
     them up within a minute. **Don't use a self-signed certificate:** you'd have to teach yourself to click through the warning that
     protects you.

   This opens the HTTPS port, **9443** by default, beside the home port (9797), which stays as it is. On Windows, the firewall asks the
   first time this opens: allow it on *Private networks* only.

5. **Forward exactly one port.** On your router, forward outside port **443** (TCP) to this computer's fixed address, port **9443**.
   The manual calls this "port forwarding" or "virtual server". Let's Encrypt connects to your outside port 443 to check that the
   domain is yours, so that port must lead here. With your own certificate, any outside port works.

   **Forward only this port. Never forward 9797.** Don't choose "all ports", and don't use "DMZ".

6. **Turn on Remote Access.** Press **Get the certificate now** if you chose Let's Encrypt.

7. **[Test it](#testing-it-from-outside) right away, and read how the connection was seen.** If a phone on mobile data shows as *at home*,
   **stop and undo the forward**: see the warning in that section.

8. **Keep watching it.** Do the [regular checks](#regular-checks). Close the forward when you won't need it for a while.

> [!WARNING]
> **"Do this automatically" (UPnP or NAT-PMP).** The server can ask the router to open the port for you. It's off by default, only the
> HTTPS port is ever opened, and the server closes it again when this is turned off or the server stops. **We suggest leaving it off and
> forwarding by hand**, so you know exactly what's open. UPnP also lets programs on your home network open ports without asking, which
> is why many security guides recommend turning it off on the router altogether. If this feature fails, don't turn on UPnP on your router
> to fix it. Forward the port by hand instead.

**Undoing all this:** turn off **Remote Access**, delete the rule on your router, then check from your phone on mobile data that your
`https://` address no longer answers.

## Testing it from outside

Do this every time you change anything. A server can't really tell from inside whether the internet can reach it, so the test uses
**your own phone**. No service on the internet is involved, and nothing is sent anywhere.

1. Press **Start the test**. The page shows a QR code and a secret address, good for 15 minutes.
2. On your phone, **turn off Wi-Fi** so it uses mobile data (keep your VPN on, if you use one).
3. Scan the code. The page on your phone says whether it worked, and the settings page updates by itself with where the connection came
   from and how.

The test page shows nothing about your server. A test from your home network is noticed, and doesn't count.

> [!CAUTION]
> **Read how the connection was seen. It's the most important check on this page.** If your phone on mobile data shows as *at home*,
> **your server thinks strangers are family.** They could see profile names and use password-free Kids and Guest profiles. Some routers
> and Docker Desktop cause this by hiding visitors' real addresses (see
> [How the server decides](#how-the-server-decides-who-is-at-home)). **Turn off Remote Access until it's fixed.** A private network avoids
> the problem.

**Check from this computer** looks at what can be seen from inside: whether your domain name leads to your router, whether the HTTPS port is
open, and whether the address answers. It only uses your computer's usual name server. Many routers won't connect a computer at home to their
own outside address, so a failure of the last step doesn't mean it fails from outside.

The status at the top of the page shows what the server has really seen. It says *Reachable* only after a passed test or a real encrypted
connection from outside.

**For a port you forwarded, also check what you exposed.** From your phone on mobile data, try `http://<your address>:9797`. It should **not**
show your server. If it does, you've forwarded the wrong port, so close it now.

## Unsafe settings, and what each one risks

Each of these exists for a reason, and each one gives something up. Leave them off unless you can say why you need them.

| Setting | Where | What it risks | Do this instead |
|---|---|---|---|
| 🔴 **Also count as my home network** | *Remote Access → Advanced* | **Anyone from those addresses is treated as family**: they see profile names and can use password-free profiles, with no HTTPS requirement. A wrong range can mean strangers | List only networks you fully control. Most people should leave it empty |
| 🔴 **Trusted reverse proxies**, listed too widely | *Remote Access → Advanced* | **Those machines can claim any visitor address**, so an attacker can claim to be at home and avoid sign-in limits | Only your proxy's exact address |
| 🔴 **`-remote-access`, `WMS_REMOTE_ACCESS=1`, `remoteAccess`, or `publicUrl`** | Start-up options and [config file](CONFIG.md) | Remote Access is on, and the switch on the page can't turn it off | Use the switch on the page. See [A rented server](#a-rented-server-vps) |
| 🔴 **Custom game cores** | *Settings → Gaming* | They're programs from outside this project, running on your server. A sandbox limits them, but limits can have bugs | Leave it off, or only use cores you trust, and never from a stranger's link |
| 🔴 **Modules from someone's fork** | *Settings → Modules* | You're trusting that person with code on your server. The project doesn't check it | Use the project's own modules |
| 🟠 **Show profiles outside your home** | *Settings* | Strangers can see who lives in your house, and names help password guessing | Leave it off. People can type their name |
| 🟠 **Do this automatically** (port mapping) | *Remote Access* | You may forget a port is open | [Forward by hand](#forwarding-a-port-on-your-router), or don't |
| 🟠 **Game PCs: who may connect, PCs on this server's own network, PCs over the internet** | *Settings → Game PCs* | By default, any adult with no limits can make the server connect to a Sunshine PC on your home network or the internet, and it then handles data from a machine it doesn't control | If you've invited people, allow only admins, and turn off what you don't use |
| 🟠 **Ports other than Sunshine's usual one** | *Settings → Game PCs* | Lets adult accounts make the server knock on other services on your network | Leave it off |
| 🟠 **UDP port for games** | *Settings → Gaming*, *Settings → Game PCs* | A second port open on the internet | Leave it empty unless games won't connect. See [Games away from home](#games-away-from-home) |
| 🟡 **Share links** (on by default) | *Settings → Sharing & photos* | **Anyone who has the link can open it, with no account.** Links get forwarded, and end up in chats and browser histories | Share rarely, and remove a link when it's done. Turn the feature off if you don't use it |
| 🟡 **Allow uploads** (on by default) | *Settings → Sharing & photos* | Someone who gets into an account may be able to fill your disk | Turn it off if you don't use it |
| 🟡 **Update checks** | *Settings → About*, *Settings → Modules* | **Turning these off is the unsafe choice.** You'd miss fixes | Keep them on. Install updates soon |
| 🟡 **Reset** | *Settings → Reset* | Anyone with an admin password can wipe the server from anywhere | A strong admin password with two-factor sign-in. Keep backups |

## Letting other people in

Everyone you invite adds a password you don't control and devices you can't see.

- **Never make them an administrator.** Make an adult account with limits, and allow only the libraries they should see, under
  *Settings → Profiles*.
- **Ask them to use a unique password and two-factor sign-in.** You can't force it, but you can decline to invite someone who won't.
- **Prefer a private network for family.** On Tailscale, share only this machine, and use their access controls to allow only port 9797.
- **Kids and Guest profiles with no password work only at home.** That's on purpose. To use one away from home, give it a password.
- **Approve pairing codes only if you started them.** A TV showing a code asks you to approve it, and approving signs that device in as
  the account you choose. Someone can send you their code and ask you to "just approve this". Say no.
- **When someone stops using it**, delete their account or sign out their devices, under *Remote Access → People connected from outside*.
- **You're responsible for what you share and who you share it with**, including having the rights to do so. See *Settings → Legal*.

## Regular checks

**Once a month, or after anything looks odd:**

1. **Update** the server, your computer and your router.
2. **Look at *Remote Access → People connected from outside*.** Do you recognize every device and place? Sign out anything you don't.
3. **Look at *Sign-in protection*.** A few wrong sign-ins are normal once you're on the internet. A sudden flood, or wrong tries for a name
   only you use, deserves a closer look.
4. **Read *Settings → WittyLog*** for sign-ins and warnings you can't explain.
5. **Check your router's forwarding rules.** Only what you meant to open should be there.
6. **Check your private network's device list.** Remove old phones and laptops.
7. **Run [the phone test](#testing-it-from-outside) again** after changing or moving your router or other network gear.
8. **Ask whether you still need this.** If not, turn it off.

## If you think someone got in

Signs include a device or sign-in you don't recognize, an account you didn't make, settings you didn't change, a share link you didn't create,
files missing or changed, or sign-ins from an unexpected place. Move quickly, and in this order.

1. **Cut it off.** Turn off **Remote Access** and remove any forward on your router. If you can't reach the settings, unplug the computer from the
   network. **Don't delete anything yet.**
2. **Save the evidence.** Download the support bundle and logs from *Settings → WittyLog*, and note the date and what you saw.
3. **From a different, clean device, change passwords** for your email and your private network's account, and anything that shared a password with
   this server.
4. **Check the computer.** Run a full malware scan, and look for programs and accounts you don't recognize. A scan finding nothing doesn't prove the
   machine is clean. **If you have any real reason to think programs were run on it, the only trustworthy fix is to reinstall the system from scratch**
   and restore your media from a backup you trust.
5. **On the server,** sign out every device, delete accounts you don't recognize, delete all share links, change every password and turn on two-factor
   sign-in. Remove anything under custom cores in *Settings → Gaming* and in *Settings → Modules* that you didn't add.
6. **Replace your API keys:** TMDB, TheTVDB, IGDB, RAWG, ScreenScraper, TheGamesDB and any others you added. Revoke the old ones on those services' sites.
7. **Consider a clean start.** *Settings → Reset* gives you a new install. **Reset doesn't clean the computer**, and if you keep a copy, that copy may
   hold whatever the intruder changed. Don't bring it back without good reason.
8. **Check your other devices** on the home network, and change your router's password.
9. **Don't turn Remote Access back on the same way.** Use a [private network](#the-recommended-way-a-private-network).
10. **Report it if it matters.** If personal or financial information may have been taken, or someone is targeting you, think about telling the relevant
    services or the police. If you found a security bug in WittyMediaServer, report it privately, as [SECURITY.md](../SECURITY.md) describes, and don't post
    the details publicly.

## What the server protects by itself, and what it doesn't

**Always on:**

- **Encryption from the internet.** Unencrypted requests from the internet are refused or sent to your HTTPS address, so a wrongly forwarded plain port can't
  expose your password. Browsers that have used HTTPS keep using it for a week (HSTS).
- **Sign-in limits.** After 5 wrong tries for one name from one address, each further try waits longer, up to 15 minutes. Away from home there are also limits
  on one address trying many names (20 wrong tries, an IPv6 network counts as one address) and one name tried from many addresses (30). **Nobody on the internet
  can lock you out at home.** Wrong tries are shown on the page, where you can also clear the locks. Only a few passwords are checked at once, so a flood of
  attempts can't take the whole machine.
- **Cookies** are `HttpOnly`, `SameSite=Lax` and, over HTTPS, `Secure`.
- **Other websites** can't read answers from the server unless they hold a sign-in token, and can't act with your cookie. This matters at home too: any page you
  visit could otherwise ask a server on your network who its profiles are.
- **Password-free Kids and Guest profiles** only work at home, and profile names aren't shown elsewhere unless you turn that on. The version isn't shown to anyone
  not signed in on the internet.

**What these don't do. A researcher reads this part first.**

- **HTTPS protects the trip, not the server.** It keeps passwords private on the way. It doesn't stop a stranger reaching the sign-in page, or a bug being used.
- **Sign-in limits slow guessing. They don't stop a password that's right.** A password leaked from another site can work on the first try. Two-factor sign-in is what
  covers that, and it's optional and per person.
- **Some addresses work with no sign-in**, as they must: the sign-in page, basic server information, themes, pairing and share links. Each is more code a stranger
  can reach.
- **There's no web application firewall, and no automatic blocking of bad addresses.**
- **Server updates aren't automatic.** A fixed bug stays open on your server until you install the fix.
- **A sandbox limits helpers and game cores. It isn't perfect**, and its protection varies by operating system.
- **The server can't see your router, your other devices or a stolen phone.**

## Limits for viewers away from home

Home connections are often slow when sending. **Highest resolution** and **Most data per stream** apply to anyone away from home, including over a VPN. Playback at home
is never affected.

- Video over the limit is converted, with every encoder told in the way it really obeys: hardware encoders switch from constant quality to a target bitrate when limited.
  [Expert options](TRANSCODING.md) can't lift the limit (though options that make an encoder ignore limits may).
- A file over the limit isn't sent as it is, unless it fits.
- What is sent is also held to the limit, with about a quarter to spare so players can get ahead, so a file can't be fetched faster by asking for it directly.
- A viewer with limits never shares a stream with one without.

Choose a limit below your connection's upload speed, divided by how many people may watch at once.

## The apps

The apps ask for your server's address, which can be either kind:

- **Desktop:** *Server → Change server…*. It warns before using an unencrypted address outside a private network. **Take that warning seriously.**
- **Mobile:** asked the first time it starts; the sign-in screen has **Change server**.

Check a device's address before you sign in on it, and sign out of any device you don't own.

## Games away from home

Games use WebRTC, which needs UDP. Most homes need nothing extra: it often finds its own way, with a STUN server if you've set one.

> [!WARNING]
> **Avoid forwarding a UDP port if you can.** It's a second door from the internet. If games won't connect without it, set a fixed port under *Settings → Gaming* or
> *Settings → Game PCs*, forward only that port, and close it when you're done.

**A gaming PC at home doesn't need to be on the internet at all**, because the server reaches the PC for you. Keep it that way.

> [!CAUTION]
> **Never forward Sunshine's ports to the internet. Never forward port 47990**, its web page, which can run commands on that PC. For a PC elsewhere, such as a friend's, use a
> private network instead. See [Game PCs](GAME-RIGS.md).

## What contacts what

The server contacts nobody for remote access unless you choose one of these:

| You chose | Who is contacted | What they learn |
|---|---|---|
| Let's Encrypt | Let's Encrypt, to get and renew the certificate | Your domain name, any email you gave, your internet address. Certificates are public |
| Automatic port mapping, or *Check my router* | Your router, on your home network only | That this computer wants a port open |
| Tailscale, NetBird | These are their apps, not part of the server | See their privacy policies |
| *Check from this computer* | The name server your computer already uses, and your own address | Your domain name |

Nothing else for remote access. The outside test uses your phone. Metadata services and other online features are separate, and are listed in *Settings → Legal*.

## How the server decides who is at home

This decides who gets trusted, so it's worth understanding.

- **At home:** this computer, private addresses (`192.168.x.x`, `10.x.x.x`, `172.16–31.x.x`, `fc00::/7`, link-local), and IPv6 addresses on this computer's own network, if that
  network card also has a private IPv4 address.
- **VPN:** `100.64.0.0/10` and `fd7a:115c:a1e0::/48`, the addresses Tailscale and NetBird use.
- **Internet:** everything else.

Under *Advanced* you can add networks to either side. List a **WireGuard** or other VPN with private addresses under *Count as away from home*, **or it looks like home**.

Forwarded headers are read only from the proxies you listed, from the right: everything to the left of the last address that isn't a trusted proxy could have been written by the
visitor. A request with forwarded headers from anyone else counts as coming from the internet, even from this computer.

> [!CAUTION]
> **Some setups hide the real address, so outside visitors look like they're at home.** Some routers rewrite the address on forwarded ports, and Docker Desktop shows every connection
> as coming from its own network. **Such a visitor could use password-free profiles.** If your router does this, or on Docker Desktop, use a VPN or a reverse proxy with
> `-network host`, and test with your phone: the test result shows how a connection was seen.

## When it doesn't work

**Slow down here.** Most security mistakes get made while trying to fix something. If a step asks you to turn off a protection, that's the wrong step.

| What you see | What it means | What to do |
|---|---|---|
| "Only your home network" | Remote Access is off | Turn it on |
| "This server is only available on its home network" in a browser | The server sees you as outside, and Remote Access is off | Turn it on from home. Behind a proxy, also list it as trusted |
| "This server only accepts encrypted connections…" | You came in over plain HTTP from the internet | Use the `https://` address; on the router, forward the HTTPS port and **not 9797** |
| "Your router's outside address is shared" | Your provider shares one address between customers (CGNAT), or there's a second router in front | Port forwarding can't work. Use Tailscale, NetBird or a rented server, or ask your provider for a public address |
| "port … can't be opened" | Something else uses the HTTPS port; on Linux, ports below 1024 need extra rights | Choose another port. **Don't run the server as root** to get around it |
| Let's Encrypt fails | Its servers couldn't reach port 443, or the name doesn't lead to your router | Check the name and the forward. Let's Encrypt limits failed tries to a few an hour, so wait before trying again |
| "Requests are arriving through a reverse proxy…" | A proxy that isn't trusted | Press **Trust**, **only if it's yours**. If you don't run a proxy, find out what this is before doing anything else |
| Someone reached "the plain, unencrypted port" | Port 9797 is open on your router | **Close it now.** Only the HTTPS port should be forwarded |
| Too many wrong tries | Sign-in limits | Wait, try from home, or press **Let everyone try again**. **If it wasn't you, someone's guessing**: check that password is strong and turn on two-factor sign-in |
| A browser warns about the certificate | The certificate is wrong or expired, or someone's in the way | **Don't continue.** Check from another network and look at the certificate's status on the page |
| A phone on mobile data shows as *at home* | Something hides visitors' real addresses | **Turn off Remote Access.** See [How the server decides](#how-the-server-decides-who-is-at-home) |
| The test works, but video stutters | Not enough upload speed | Lower the [limits](#limits-for-viewers-away-from-home) |
