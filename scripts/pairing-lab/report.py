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
