#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""The pairing lab's reports.

  report.py case <case folder> <exit code>    writes result.json beside steps.jsonl
  report.py summary <results folder>          summary.md, junit.xml and a table on stdout; exit 1 if any case failed
  report.py timeline <case folder>            both sides' steps, in order, as text
"""
import html
import json
import os
import sys

# Networks with no WittyRelay of the owner's, and those where no way through can exist (see case()).
NO_OWN_RELAY = {"direct", "public-cone", "public-sym", "docker-unpub"}  # docker-home: a code there is typed at home
EXPLAINED = {"public-cone", "public-sym", "docker-unpub"}

KEY_STEPS = ["invite-made", "connector-start", "connected", "enrol", "join-answer", "identity", "pair-redeem",
             "use-after-pairing", "reopen-identity", "reopen-use"]


def steps(folder):
    out = []
    try:
        with open(os.path.join(folder, "steps.jsonl")) as f:
            for line in f:
                line = line.strip()
                if line:
                    try:
                        out.append(json.loads(line))
                    except ValueError:
                        pass
    except OSError:
        pass
    out.sort(key=lambda s: s.get("t", 0))
    return out


def case(folder, rc):
    ss = steps(folder)
    name = os.path.basename(folder.rstrip("/"))
    failed = next((s for s in ss if s.get("ok") is False and s.get("step") not in ("state",)), None)
    result = next((s for s in ss if s.get("side") == "phone" and s.get("step") == "result"), None)
    timings = {}
    for s in ss:
        st = s.get("step")
        if st in KEY_STEPS and st not in timings:
            timings[st] = s.get("t")
    rc = int(rc)
    r = {
        "case": name,
        "ok": rc == 0,
        # 4: the case was stopped from outside (lab.sh's time limit), which says the pairing never finished.
        "outcome": "pass" if rc == 0 else ("timeout" if rc == 4 else "lab-error" if rc >= 3 else "fail"),
        "seconds": round((ss[-1]["t"] if ss else 0) / 1000.0, 1),
        "failedStep": failed.get("step") if failed else None,
        "why": (result or {}).get("detail") if rc else None,
        "failedDetail": failed.get("detail") if failed else None,
        "at": timings,
    }
    parts = name.split(".")
    net, way = (parts[0], parts[4]) if len(parts) == 5 else ("", "")
    phone = [s for s in ss if s.get("side") == "phone" and isinstance(s.get("detail"), dict)]
    if rc in (1, 4) and way == "code" and net in NO_OWN_RELAY:
        # A code from away with no WittyRelay of the owner's: a TV's shown code found only through the public
        # address book, known to be slow and unreliable. Recorded, not a gate. (With the owner's relay it is.)
        r["outcome"], r["ok"] = "known-unreliable", True
    if rc == 1 and net in EXPLAINED:
        # No way through can exist here: only a public relay, which carries no requests, and no direct path (in
        # the lab no hole punch can succeed; on real networks one often does). Pairing can't finish; what is
        # checked is that the phone is told why, in the connector's own words (X-Witty-Connect-Error and its
        # body), never a bare timeout, and in a container that the server names the port to publish.
        # Every failed identity check (the first try, and a retry within 30 s) must carry the connector's named
        # reason, public-relay-only, and its words; any failed check without them is a failure of this case.
        failed_ids = [s for s in phone if s["step"] in ("identity", "reopen-identity") and s.get("ok") is False]
        named = [s for s in failed_ids if s["detail"].get("connectorError")
                 and "public-relay-only" in str(s["detail"].get("body", ""))
                 and "only through a public relay" in str(s["detail"].get("body", ""))]
        told = named if failed_ids and len(named) == len(failed_ids) else []
        bare = [s for s in failed_ids if s not in named]
        r["explainedTries"] = len(failed_ids)
        if way == "retry" and len(failed_ids) < 2:
            told = []  # a retry case must show the reason on both tries
        hint = net != "docker-unpub" or any(
            "isn't published from Docker" in str((s.get("detail") or {}).get("hint", ""))
            for s in ss if s.get("step") == "connect-status")
        r["explained"] = {"connectorWords": bool(told), "bareTimeout": bool(bare), "portHint": hint}
        if told and not bare and hint:
            r["outcome"], r["ok"] = "explained", True
    if rc == 0 and way == "three":
        # Three invites in 30 s: the relay never answered "busy", and the enrol says the relay had the device
        # listed before it answered (relayReady), so the first dial through it works.
        busy = "busy" in open(os.path.join(folder, "relay.log"), errors="replace").read() if os.path.exists(
            os.path.join(folder, "relay.log")) else False
        ready = any(s["step"] == "enrol" and isinstance(s["detail"].get("answer"), dict)
                    and s["detail"]["answer"].get("relayReady") is True for s in phone)
        r["three"] = {"relayBusy": busy, "relayReady": ready}
        if busy or not ready:
            r["outcome"], r["ok"] = "fail", False
    with open(os.path.join(folder, "result.json"), "w") as f:
        json.dump(r, f, indent=2)
    return r


def timeline(folder):
    for s in steps(folder):
        d = s.get("detail")
        d = "" if d is None else json.dumps(d)[:220]
        print("%8.2fs  %-6s %-22s %-5s %s" % (s.get("t", 0) / 1000.0, s.get("side", ""), s.get("step", ""),
                                            "ok" if s.get("ok") else "FAIL", d))


def summary(root):
    rows = []
    for name in sorted(os.listdir(root)):
        p = os.path.join(root, name, "result.json")
        if os.path.isfile(p):
            with open(p) as f:
                rows.append(json.load(f))
    bad = [r for r in rows if not r["ok"]]
    lines = ["# Pairing lab: %d cases, %d passed, %d failed" % (len(rows), len(rows) - len(bad), len(bad)), "",
             "| case | outcome | seconds | failed at | why |", "|---|---|---|---|---|"]
    for r in rows:
        why = r.get("why") or ""
        if not isinstance(why, str):
            why = json.dumps(why)
        lines.append("| %s | %s | %s | %s | %s |" % (r["case"], r["outcome"], r["seconds"], r.get("failedStep") or "",
                                                     why.replace("|", "/")[:160]))
    md = "\n".join(lines) + "\n"
    with open(os.path.join(root, "summary.md"), "w") as f:
        f.write(md)
    x = ['<?xml version="1.0" encoding="UTF-8"?>',
         '<testsuite name="pairing-lab" tests="%d" failures="%d">' % (len(rows), len(bad))]
    for r in rows:
        x.append('  <testcase classname="pairing" name="%s" time="%s">' % (html.escape(r["case"]), r["seconds"]))
        if not r["ok"]:
            x.append('    <failure message="%s">%s</failure>' % (html.escape(str(r.get("failedStep"))),
                                                                html.escape(json.dumps(r, indent=1))))
        x.append("  </testcase>")
    x.append("</testsuite>")
    with open(os.path.join(root, "junit.xml"), "w") as f:
        f.write("\n".join(x) + "\n")
    print(md)
    gh = os.environ.get("GITHUB_STEP_SUMMARY")
    if gh:
        with open(gh, "a") as f:
            f.write(md)
    return 1 if bad else 0


if __name__ == "__main__":
    cmd = sys.argv[1] if len(sys.argv) > 1 else ""
    if cmd == "case":
        case(sys.argv[2], sys.argv[3])
    elif cmd == "summary":
        sys.exit(summary(sys.argv[2]))
    elif cmd == "timeline":
        timeline(sys.argv[2])
    else:
        print(__doc__)
        sys.exit(2)
