#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Compresses pictures taken for the docs, and makes the small copies in-app Help shows.

  scripts/docs-images.py <folder of PNGs> <docs/images folder>      WebP copies for GitHub
  scripts/docs-images.py --help-copies                              small copies of what the guides use

Needs cwebp. The first keeps pictures to what a repository can carry, at a quality that still reads
well. The second finds every picture named in docs/*.md, writes a copy
720 pixels wide to web/public/help-images, and its size to web/src/help/imageSizes.json. Those
copies go inside the server's download, so keep to the pictures that teach something.
"""
import glob, json, os, re, struct, subprocess, sys

HERE = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))


def webp(src, dest, width, quality):
    args = ['cwebp', '-quiet', '-mt', '-m', '6', '-q', str(quality)]
    if width:
        args += ['-resize', str(width), '0']
    subprocess.run(args + [src, '-o', dest], check=True)


def size_of(path):
    with open(path, 'rb') as f:
        head = f.read(30)
    if head[12:16] == b'VP8 ':
        w, h = struct.unpack('<HH', head[26:30])
        return w & 0x3fff, h & 0x3fff
    if head[12:16] == b'VP8L':
        b = struct.unpack('<I', head[21:25])[0]
        return (b & 0x3fff) + 1, ((b >> 14) & 0x3fff) + 1
    if head[12:16] == b'VP8X':
        return int.from_bytes(head[24:27], 'little') + 1, int.from_bytes(head[27:30], 'little') + 1
    raise ValueError('not a WebP file: ' + path)


def full(src_dir, out_dir):
    os.makedirs(out_dir, exist_ok=True)
    for png in sorted(glob.glob(os.path.join(src_dir, '*.png'))):
        name = os.path.splitext(os.path.basename(png))[0] + '.webp'
        webp(png, os.path.join(out_dir, name), 0, 80)
        print(name, os.path.getsize(os.path.join(out_dir, name)) // 1024, 'KB')


def help_copies():
    docs = os.path.join(HERE, 'docs')
    out = os.path.join(HERE, 'web', 'public', 'help-images')
    os.makedirs(out, exist_ok=True)
    used = set()
    for md in glob.glob(os.path.join(docs, '*.md')):
        with open(md, encoding='utf-8') as f:
            used |= set(re.findall(r'^\s*!\[[^\]]*\]\(images/([A-Za-z0-9_-]+\.webp)\)', f.read(), re.M))
    for old in os.listdir(out):
        if old not in used:
            os.remove(os.path.join(out, old))
    sizes = {}
    total = 0
    for name in sorted(used):
        dest = os.path.join(out, name)
        webp(os.path.join(docs, 'images', name), dest, 720, 62)
        sizes[name] = list(size_of(dest))
        total += os.path.getsize(dest)
    with open(os.path.join(HERE, 'web', 'src', 'help', 'imageSizes.json'), 'w', encoding='utf-8', newline='\n') as f:
        json.dump(sizes, f, indent=0, sort_keys=True)
        f.write('\n')
    print(len(sizes), 'copies,', total // 1024, 'KB')


if __name__ == '__main__':
    if sys.argv[1:] == ['--help-copies']:
        help_copies()
    elif len(sys.argv) == 3:
        full(sys.argv[1], sys.argv[2])
    else:
        sys.exit(__doc__)
