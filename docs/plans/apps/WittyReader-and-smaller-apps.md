# WittyReader and the smaller apps: update plans

> **Status on 2026-11-04**
>
> **Done:** WittyReader's desktop app brings its window forward on a second start. **Planned:** [read aloud and your server's books](https://github.com/wittywilla/WittyMediaServer/discussions/64). WittyMediaRemoteMedia and the WittyDiagnoser app haven't started.

---

## WittyReader

**Now:** 0.1.1. A reader app for EPUB, PDF and CBZ on the device (built from WittyEbook, WittyPDF, WittyComics and WittyInk), desktop and a static web app with an offline service worker. Nothing leaves the device except an optional dictionary look-up. Not code-signed; macOS build never run.

| Release | Theme |
|---|---|
| 0.1.x | Single instance; macOS build run on a Mac |
| 0.2.0 | Optional sign-in to a WittyMediaServer to read its books and sync positions and notes (off by default; on-device stays the default) |
| 0.3.0 | Read-aloud with the device's own voices; Android and iOS (a web view wrapper first) |
| Later | Store listings; code signing |

---

## WittyMediaRemoteMedia

A repository exists with a README only (roadmap: "the basics of connecting NAS libraries through a native or Docker app, in its own repository": not started).

| Step | Theme |
|---|---|
| Decide | Its scope: a small program on a NAS that exposes folders to the server read-only, faster and safer than SMB for media (the server's own network share warnings point at the need) |
| 0.1.0 | A Docker image and a native program for Linux NAS systems; read-only; pairs with a server by code, like clients do; no internet |
| Later | Thumbnails and probes done on the NAS, so the server's first scan of a big NAS library is quicker |

---

## WittyDiagnoser app

Roadmap: "A small Windows and macOS app that makes a diagnostic file without a server": not started. **Settings → WittyDiagnoser** does this with a server.

| Step | Theme |
|---|---|
| 0.1.0 | A tiny program (Go, no cgo) that collects what the server's diagnoser does when the server won't start: system, ports, the data folder's health, the last crash files, logs sanitised by WittyLog's rules; writes one zip; never sends anything |
| Later | Offer the repair tools (`wittymediaserver doctor`, repair from 0.6.0) from the same window |

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
