#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""The pairing lab's real phone: the Android app (a released APK or a build) on an emulator that runs
inside the phone's network namespace, so everything it sends goes through that network's router.
case.sh starts the emulator and runs this in the same namespace, as the emulator's user.

  emuphone.py --serial emulator-5770 --apk app.apk --mode link --link 'wittymedia://p2p/…' \
      --state fresh --steps steps.jsonl --t0 <ms> --out <folder>

States:
  fresh     the app installed from nothing (data cleared).
  upgrade   --old-apk installed first, signed in at home (--home http://192.168.3.1:19890 with
            --user/--password; the lab unplugs that cable when this writes <out>/at-home.done), then
            --apk installed over it, as an update does.

Ways: link (the invite pasted into "Join with a link or code", as a scanned QR code fills it) or code
(the phone shows a code: written to --codefile for the lab to type into the server).

Every step goes to --steps as {"t","side":"phone","step","ok","detail"}, with a screenshot and the
screen's words in <out>/emu-NN-<step>.png|.txt. The outcome is the app's own: its home after
signing in, or the words it shows (such as "Couldn't reach your server from away") and how soon.
"""
import argparse, json, os, re, subprocess, sys, time, xml.etree.ElementTree as ET

PKG = 'com.wittywilla.wittymediaclient'
p = argparse.ArgumentParser()
p.add_argument('--serial', required=True)
p.add_argument('--apk', required=True)
p.add_argument('--old-apk', default='')
p.add_argument('--mode', default='link')
p.add_argument('--link', default='')
p.add_argument('--linkfile', default='', help='wait for the link in this file (written when the lab makes the invite)')
p.add_argument('--state', default='fresh')
p.add_argument('--steps', required=True)
p.add_argument('--t0', type=int, default=0)
p.add_argument('--out', required=True)
p.add_argument('--codefile', default='')
p.add_argument('--home', default='')
p.add_argument('--user', default='owner')
p.add_argument('--password', default='')
p.add_argument('--wait', type=int, default=150, help='seconds the app is given to finish pairing')
A = p.parse_args()
T0 = A.t0 or int(time.time() * 1000)
shot_n = 0
# What the app says when pairing fails (shared/data/strings.en.json, errAwayFailed and its neighbours).
FAILURES = r"Couldn't reach your server from away|isn't an invite|couldn.t be (used|reached)|refused|expired|isn.t valid|wasn.t accepted|Something went wrong|can.t be reached"


def step(name, ok, detail=None):
    with open(A.steps, 'a') as f:
        f.write(json.dumps({'t': int(time.time() * 1000) - T0, 'at': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
                            'side': 'phone', 'step': name, 'ok': ok, 'detail': detail}) + '\n')


def adb(*a, check=True, timeout=180):
    r = subprocess.run(['adb', '-s', A.serial, *a], capture_output=True, text=True, timeout=timeout)
    if check and r.returncode != 0:
        raise RuntimeError(f"adb {' '.join(a)}: {r.stdout.strip()} {r.stderr.strip()}")
    return r.stdout


def sh(cmd, **k):
    return adb('shell', cmd, **k)


def nodes():
    for _ in range(10):
        # The old dump goes first: a failed dump must not leave the previous screen to be read.
        xml = sh('rm -f /sdcard/u.xml; uiautomator dump /sdcard/u.xml >/dev/null 2>&1; cat /sdcard/u.xml 2>/dev/null', check=False)
        try:
            root = ET.fromstring(xml[xml.index('<'):])
            break
        except (ValueError, ET.ParseError):
            time.sleep(0.5)
    else:
        return []
    out = []
    for e in root.iter('node'):
        b = [int(x) for x in re.findall(r'\d+', e.get('bounds', '[0,0][0,0]'))]
        out.append({'text': e.get('text', ''), 'desc': e.get('content-desc', ''), 'cls': e.get('class', ''),
                    'clickable': e.get('clickable') == 'true', 'b': b})
    return out


def texts(ns=None):
    return [x['text'] or x['desc'] for x in (ns if ns is not None else nodes()) if x['text'] or x['desc']]


def dismiss(ns):
    """A system "isn't responding" dialog over everything (the launcher, on a busy machine) is closed; one about this
    app is waited out, as that would be ours to fix. Says whether there was one."""
    msg = next((x['text'] for x in ns if "isn't responding" in x['text'] or 'keeps stopping' in x['text']), None)
    if not msg:
        return False
    ours = 'Witty' in msg
    btn = next((x for x in ns if x['text'] == ('Wait' if ours else 'Close app')), None)
    print('system dialog:', msg, flush=True)
    if ours:
        # The app hung: written down once per occurrence, with what the system knows about it.
        global anrs
        anrs += 1
        step('app-not-responding', False, {'dialog': msg, 'count': anrs})
        if anrs == 1:
            shot('app-not-responding')
            save_logs('anr-1')
    if btn:
        press(btn)
    return True


anrs = 0


def save_logs(tag):
    """The device's log and, where adb can be root (an emulator's default image), the system's ANR traces."""
    try:
        with open(os.path.join(A.out, f'logcat-{tag}.txt'), 'w') as f:
            f.write(adb('logcat', '-d', '-v', 'threadtime', check=False, timeout=60))
        adb('root', check=False, timeout=30)
        time.sleep(2)
        traces = sh('ls -t /data/anr/ 2>/dev/null | head -2', check=False).split()
        for t in traces:
            with open(os.path.join(A.out, f'anr-{tag}-{t}.txt'), 'w') as f:
                f.write(sh(f'cat /data/anr/{t}', check=False, timeout=60))
    except Exception as e:  # noqa: BLE001
        print('no logs:', e, flush=True)


def find(pattern, ns=None):
    r = re.compile(pattern, re.I)
    if ns is None:
        ns = nodes()
        if dismiss(ns):
            ns = nodes()
    hits = [x for x in ns if r.search(x['text']) or r.search(x['desc'])]
    hits.sort(key=lambda x: (not x['clickable'], (x['b'][2] - x['b'][0]) * (x['b'][3] - x['b'][1]) <= 0))
    return hits[0] if hits else None


def wait(pattern, secs=30, what=None):
    end = time.time() + secs
    while True:
        x = find(pattern)
        if x:
            return x
        if time.time() > end:
            raise RuntimeError(f'waited {secs}s for {what or pattern}; on screen: {texts()[:25]}')
        time.sleep(1)


def press(x):
    b = x['b']
    sh(f'input tap {(b[0] + b[2]) // 2} {(b[1] + b[3]) // 2}')
    time.sleep(1)


def tap(pattern, secs=30):
    press(wait(pattern, secs))


def type_text(s):
    # In short pieces: a busy emulator's field drops characters typed faster than it redraws.
    # Inside single quotes nothing but the quote itself is special to the device's shell.
    s = s.replace("'", '')
    for i in range(0, len(s), 24):
        sh("input text '" + s[i:i + 24].replace(' ', '%s') + "'")
        time.sleep(0.2)


def type_into(pattern, s):
    fields = [x for x in nodes() if 'EditText' in x['cls']]
    r = re.compile(pattern, re.I)
    f = next((x for x in fields if r.search(x['text']) or r.search(x['desc'])), fields[0] if fields else None)
    if not f:
        raise RuntimeError(f'no text field; on screen: {texts()[:25]}')
    press(f)
    sh('input keyevent KEYCODE_MOVE_END')
    old = len(f['text']) + 2
    for _ in range(3):
        sh('input keyevent KEYCODE_MOVE_END')
        sh('input keyevent ' + ' '.join(['KEYCODE_DEL'] * old), check=False)
        type_text(s)
        now = next((x['text'] for x in nodes() if 'EditText' in x['cls'] and abs(x['b'][1] - f['b'][1]) < 60), '')
        if now == s or len(s) <= 20 or not now or 'assword' in pattern:
            return
        print('typed text differs, again:', now[:80], flush=True)
        old = len(now) + 20
    raise RuntimeError('the field never held what was typed')


def shot(name):
    global shot_n
    shot_n += 1
    base = os.path.join(A.out, f'emu-{shot_n:02d}-{re.sub(r"[^a-z0-9]+", "-", name.lower()).strip("-")}')
    try:
        with open(base + '.png', 'wb') as f:
            f.write(subprocess.run(['adb', '-s', A.serial, 'exec-out', 'screencap', '-p'], capture_output=True, timeout=30).stdout)
        with open(base + '.txt', 'w') as f:
            f.write('\n'.join(texts()))
    except Exception as e:  # noqa: BLE001
        print('no picture:', e)


def launch(clear=False):
    sh(f'am force-stop {PKG}', check=False)
    if clear:
        sh(f'pm clear {PKG}', check=False)
    sh(f'monkey -p {PKG} -c android.intent.category.LAUNCHER 1', check=False)
    time.sleep(3)
    past_welcome()


def on_welcome(ns=None):
    ns = ns if ns is not None else nodes()
    tiles = sum(1 for x in ns if re.match(r'^[\w ]+ layout$', x['text'] or x['desc']))
    return tiles >= 2 or bool(find('Choose a layout|Choose colours|Your look is set', ns))


def past_welcome():
    for _ in range(40):
        ns = nodes()
        if not on_welcome(ns):
            return
        btn = find('^(Use this|Continue|Next|Done)$', ns)
        if btn:
            press(btn)
        else:
            sh('input swipe 500 1600 500 400 150', check=False)


def install(apk, over=True):
    t = time.time()
    out = adb('install', '-r', '-d', apk, check=False, timeout=300) if over else ''
    if 'Success' not in out:
        adb('uninstall', PKG, check=False)
        out = adb('install', apk, timeout=300)
    ver = re.search(r'versionName=(\S+)', sh(f'dumpsys package {PKG} | grep versionName', check=False) or '')
    step('app-installed', True, {'apk': os.path.basename(apk), 'version': ver.group(1) if ver else '?', 'ms': int((time.time() - t) * 1000)})


def signed_in_home(ns):
    """The app's home after signing in: its tab bar, and no connect or sign-in screen."""
    return bool(find('^(Home|Library|Search|You)$', ns)) and not find('Join with a link or code|Enter an address|Sign in$|Password', ns)


def at_home():
    """0.6.0/0.6.1 at home: the server's address, then a sign-in with the password."""
    launch(clear=True)
    for _ in range(3):
        tap('^Enter an address$', 60)
        if find("Enter your server's address|Server address"):
            break
        sh('input keyevent KEYCODE_BACK', check=False)
    type_into('address', A.home.replace('http://', ''))
    sh('input keyevent KEYCODE_ENTER', check=False)
    if find('^Connect$'):
        tap('^Connect$')
    # A server with accounts first asks who is watching: the account's tile, then its password.
    end = time.time() + 60
    while time.time() < end:
        ns = nodes()
        if any('EditText' in x['cls'] for x in ns):
            break
        tile = find(f'^{re.escape(A.user)}$', ns) if find("Who.s watching", ns) else None
        x = tile or find('Use a password|Sign in with a password', ns)
        if x:
            press(x)
        else:
            time.sleep(1)
    fields = [x for x in nodes() if 'EditText' in x['cls']]
    if len(fields) >= 2:
        type_into('Name|User', A.user)
        type_into('Password', A.password)
    else:
        type_into('Password', A.password)
    sh('input keyevent KEYCODE_ENTER', check=False)
    if find('^Sign in$'):
        tap('^Sign in$')
    end = time.time() + 90
    while time.time() < end:
        past_welcome()
        if signed_in_home(nodes()):
            return
        time.sleep(1)
    raise RuntimeError('not signed in at home; on screen: ' + str(texts()[:25]))


def join_screen():
    launch(clear=(A.state == 'fresh'))
    # From a signed-in app (the upgrade), the way in is under the server list: back out to it.
    for _ in range(4):
        if find('Join with a link or code|Connect from anywhere'):
            break
        x = find('Change server|Switch server|Servers|Other server')
        if x:
            press(x)
        else:
            sh('input keyevent KEYCODE_BACK', check=False)
            time.sleep(1)
            if not find(PKG.split('.')[-1]) and PKG not in sh('dumpsys window | grep mCurrentFocus', check=False):
                sh(f'monkey -p {PKG} -c android.intent.category.LAUNCHER 1', check=False)
                time.sleep(2)
    tap('Join with a link or code|Connect from anywhere', 60)
    if find('Connect from anywhere') and not find('Invite link'):
        tap('Join with a link or code')


def outcome(since):
    """Waits for what the app ends on, and says how soon after `since` it got there."""
    end = time.time() + A.wait
    last = []
    while time.time() < end:
        past_welcome()
        ns = nodes()
        if dismiss(ns):
            continue
        last = texts(ns)
        bad = find(FAILURES, ns)
        if bad:
            return False, {'shown': bad['text'] or bad['desc'], 'afterMs': int((time.time() - since) * 1000), 'screen': last[:20]}
        if signed_in_home(ns):
            return True, {'afterMs': int((time.time() - since) * 1000)}
        time.sleep(0.7)
    return False, {'shown': 'nothing: still waiting', 'afterMs': int((time.time() - since) * 1000), 'screen': last[:20]}


def main():
    sh('settings put global window_animation_scale 1', check=False)
    if A.state == 'upgrade':
        install(A.old_apk, over=False)
        t = time.time()
        at_home()
        step('signed-in-at-home', True, {'ms': int((time.time() - t) * 1000)})
        shot('at-home')
        # Long enough for the app to put itself on the server's list, as it does at home.
        time.sleep(15)
        open(os.path.join(A.out, 'at-home.done'), 'w').close()
        for _ in range(60):
            if os.path.exists(os.path.join(A.out, 'away.done')):
                break
            time.sleep(1)
        step('away', True, None)
        install(A.apk, over=True)
    else:
        adb('uninstall', PKG, check=False)
        install(A.apk, over=False)
    join_screen()
    shot('join-screen')
    if A.mode == 'link':
        link = A.link
        while not link and A.linkfile:
            if os.path.exists(A.linkfile):
                link = open(A.linkfile).read().strip()
            else:
                time.sleep(0.5)
        t = time.time()
        type_into('Invite link|link', link)
        step('link-entered', True, {'ms': int((time.time() - t) * 1000)})
        sh('input keyevent KEYCODE_ENTER', check=False)
        if find('^(Join|Connect)$'):
            tap('^(Join|Connect)$')
        since = time.time()
        step('join-pressed', True, None)
    else:
        x = wait('Show a code|Use a code|a code instead|Get a code', 30)
        press(x)
        c = wait(r'^[0-9A-Z]{4}-[0-9A-Z]{4}$', 60, 'the code on screen')
        code = c['text'] or c['desc']
        step('join-code-shown', True, {'code': code})
        with open(A.codefile + '.tmp', 'w') as f:
            f.write(code)
        os.rename(A.codefile + '.tmp', A.codefile)
        since = time.time()
    ok, d = outcome(since)
    shot('outcome')
    save_logs('end')
    step('app-outcome', ok, d)
    step('result', ok, 'paired on the first try' if ok else f"the app showed: {d.get('shown')} after {d.get('afterMs')} ms")
    return 0 if ok else 1


if __name__ == '__main__':
    try:
        sys.exit(main())
    except Exception as e:  # noqa: BLE001
        shot('error')
        save_logs('error')
        step('result', False, 'lab: ' + str(e)[:400])
        sys.exit(1)
