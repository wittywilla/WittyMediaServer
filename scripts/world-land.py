#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Makes web/public/world-land.txt, the photo map's outline of the world's land,
from world-atlas's land-110m.json (Natural Earth 1:110m land, public domain;
world-atlas is ISC):

  curl -LO https://cdn.jsdelivr.net/npm/world-atlas@2.0.2/land-110m.json
  python3 scripts/world-land.py land-110m.json > web/public/world-land.txt

The output is one SVG path: Web Mercator, the world 1000 units wide with the
top at 85.05 degrees north, as web/src/pages/photos/mapMath.ts draws it. Every
ring is kept: the first outline lost Africa, Europe and Asia, so this fails if
a ring of that size is missing.
"""
import json
import math
import sys

MERC_LAT = 85.05112878


def merc(lon, lat):
    lat = max(-MERC_LAT, min(MERC_LAT, lat))
    y = 0.5 - math.log(math.tan(math.pi / 4 + math.radians(lat) / 2)) / (2 * math.pi)
    return (lon + 180) / 360 * 1000, y * 1000


def main(path):
    with open(path, encoding='utf-8') as f:
        topo = json.load(f)
    sx, sy = topo['transform']['scale']
    tx, ty = topo['transform']['translate']
    arcs = []
    for arc in topo['arcs']:  # delta-encoded, quantised
        x = y = 0
        out = []
        for dx, dy in arc:
            x += dx
            y += dy
            out.append((x * sx + tx, y * sy + ty))
        arcs.append(out)

    def ring(indexes):
        pts = []
        for i in indexes:
            a = arcs[i] if i >= 0 else arcs[~i][::-1]
            pts.extend(a if not pts else a[1:])  # arcs share their end points
        return pts

    polygons = []
    for g in topo['objects']['land']['geometries']:
        if g['type'] == 'Polygon':
            polygons.append(g['arcs'])
        elif g['type'] == 'MultiPolygon':
            polygons.extend(g['arcs'])
    parts, biggest = [], 0

    def emit(pts, shift):
        out, last = [], None
        for lon, lat in pts:
            x, y = merc(lon, lat)
            p = '%.1f,%.1f' % (x + shift, y)
            if p != last:
                out.append(p)
                last = p
        if len(out) >= 3:
            parts.append('M' + ' '.join(out) + 'Z')

    for poly in polygons:
        for r in poly:
            # The data isn't cut at longitude 180: Asia runs on into Chukotka at
            # -170, and Antarctica goes right round. Make the longitudes run on
            # (190 rather than -170), so no edge crosses the whole world.
            pts = []
            for lon, lat in ring(r):
                if pts:
                    while lon - pts[-1][0] > 180:
                        lon -= 360
                    while lon - pts[-1][0] < -180:
                        lon += 360
                pts.append((lon, lat))
            turn = pts[-1][0] - pts[0][0]
            if abs(turn) > 180:  # round a pole: close the ring along the map's edge there
                pole = -90 if sum(p[1] for p in pts) < 0 else 90
                pts += [(pts[-1][0], pole), (pts[0][0], pole)]
            lons = [p[0] for p in pts]
            biggest = max(biggest, max(lons) - min(lons))
            emit(pts, 0)
            # The part past longitude 180 is drawn again at the other side.
            if max(lons) > 180:
                emit(pts, -1000)
            if min(lons) < -180:
                emit(pts, 1000)
    if biggest < 180:  # Afro-Eurasia runs from Senegal to the Bering Strait
        sys.exit('no ring crosses half the world: Africa and Eurasia are missing')
    sys.stdout.write(''.join(parts))


if __name__ == '__main__':
    main(sys.argv[1])
