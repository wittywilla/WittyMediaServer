#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Draws every board of the Design 2 canvas to a picture, so the designs can be looked at without the canvas.

  python3 export-boards.py --runtime /path/to/support.js [--out ../images] [--chrome "C:/Program Files/Google/Chrome/Application/chrome.exe"]

The boards (../canvas/*.dc.html) are Design Component pages. They need the canvas's own runtime
script, which is not part of this repository (it belongs to the tool that hosts the canvas): save it
from the live canvas (artifact-type/dc-runtime.js) as support.js and pass it with --runtime. The
script copies the boards beside it in a scratch folder, serves them on a local port, and uses
Chrome or Edge in headless mode to take one picture per board at the size canvas.json gives it.
Windows, macOS and Linux work. Pictures are PNG; use --jpeg to make smaller JPEGs (needs Pillow).
"""
import argparse
import http.server
import json
import os
import platform
import shutil
import socketserver
import subprocess
import sys
import tempfile
import threading

HERE = os.path.dirname(os.path.abspath(__file__))


def find_chrome(given):
    if given:
        return given
    c = {
        "Darwin": ["/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"],
        "Windows": [r"C:\Program Files\Google\Chrome\Application\chrome.exe", r"C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe"],
    }.get(platform.system(), ["google-chrome", "chromium", "chromium-browser", "microsoft-edge"])
    for p in c:
        if os.path.exists(p) or shutil.which(p):
            return p
    sys.exit("No Chrome or Edge found: pass --chrome")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--runtime", required=True, help="the canvas runtime script, saved as support.js")
    ap.add_argument("--out", default=os.path.join(HERE, "..", "images"))
    ap.add_argument("--chrome")
    ap.add_argument("--jpeg", action="store_true")
    ap.add_argument("--port", type=int, default=19991)
    ap.add_argument("--only", help="only boards whose file name contains this")
    a = ap.parse_args()
    chrome = find_chrome(a.chrome)
    canvas = os.path.join(HERE, "..", "canvas")
    scratch = tempfile.mkdtemp(prefix="design2-")
    for f in os.listdir(canvas):
        shutil.copy(os.path.join(canvas, f), scratch)
    shutil.copy(a.runtime, os.path.join(scratch, "support.js"))
    handler = lambda *x, **k: http.server.SimpleHTTPRequestHandler(*x, directory=scratch, **k)
    srv = socketserver.TCPServer(("127.0.0.1", a.port), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    boards = json.load(open(os.path.join(canvas, "canvas.json")))["boards"]
    for name, b in boards.items():
        if b.get("page") == "build" or (a.only and a.only not in name):
            continue
        out = os.path.join(a.out, b.get("page", "looks"))
        os.makedirs(out, exist_ok=True)
        png = os.path.join(out, name.replace(".dc.html", ".png"))
        subprocess.run([chrome, "--headless=new", "--disable-gpu", "--hide-scrollbars", f"--user-data-dir={scratch}/profile", f"--screenshot={png}",
                        f"--window-size={b['w']},{b['h']}", "--virtual-time-budget=30000", f"http://127.0.0.1:{a.port}/{name}"],
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=240)
        if a.jpeg and os.path.exists(png):
            from PIL import Image
            im = Image.open(png).convert("RGB")
            if im.width > 1800:
                im = im.resize((1800, round(im.height * 1800 / im.width)))
            im.save(png[:-4] + ".jpg", quality=62)
            os.remove(png)
        print(name)
    srv.shutdown()


if __name__ == "__main__":
    main()
