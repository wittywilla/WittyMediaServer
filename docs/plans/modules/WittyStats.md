# WittyStats: update plan

> **Status on 2026-11-04**
>
> **Planned.** Choosing a preset already says so and can be undone within the session (WittyStats 0.1.2).

**Now:** 0.1.2. Web module, API 3. A statistics screen you arrange yourself: the machine, streams, libraries and history, as charts or tables, with exports. Presets; layouts reset to stock; "Not available here" with a reason instead of zeros; non-admins see only what's theirs. Costs nothing until opened.

**Not seen in 0.6.0:** real TV, controller, screen reader, a live game session, sensors, touch drag, duplicates, bandwidth and most watched with real rows.

**Channel:** alpha.

| Release | Theme |
|---|---|
| 0.1.x | The unseen checks above; presets replace layouts with no confirmation today (Undo within the session): add a confirmation or a longer Undo |
| **0.2.0** | Game streaming widgets from WittyPlay 0.3.8's stamps (frame to screen, press to picture, hitches, sound offset, encoder); live TV widgets (viewers per channel, tuner use, signal quality); thumbnails and intro detection get tasks of their own and show here |
| 0.3.0 | Alerts: thresholds on any widget (disk nearly full, a scan failing repeatedly, transcode queue long) shown in the activity panel, never sent anywhere |
| Later | WittyStats on native apps (a web view first); history kept longer with downsampling; export to standard formats (CSV exists; add a Prometheus-style endpoint for people who run their own monitoring, admin only, off by default) |

**Rule kept:** everything comes from the person's own server; estimates labelled as estimates; no telemetry leaves the server.

---

<sub>Future plans, written 2026-11-02. Not built, not tested.</sub>
