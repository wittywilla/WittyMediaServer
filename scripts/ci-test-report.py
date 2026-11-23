#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-or-later
# Copyright (C) 2026 WittyWilla and contributors
"""Turns the output of `go test -json` into a short failure report for CI.

Each failing test becomes an annotation on the run, which anyone can read
without signing in, where the logs themselves need an account.

    ci-test-report.py go-test.json [--no-skip TEXT]

With --no-skip, a test that skipped saying TEXT fails the report: the job with
the module catalogue uses it so that a test needing modules can't pass by
skipping there.
"""
import json
import sys

MAX_ANNOTATIONS = 10  # the most GitHub shows for one step
MAX_MESSAGE = 3000


def escape(s, prop=False):
    s = s.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
    return s.replace(':', '%3A').replace(',', '%2C') if prop else s


def main(path, no_skip=None):
    output = {}  # (package, test) -> text
    failed = []
    skipped = []
    passed = tests = 0
    with open(path, encoding='utf-8', errors='replace') as f:
        for line in f:
            try:
                ev = json.loads(line)
            except ValueError:  # the go command's own message, outside the JSON
                output.setdefault((line.split(':')[0][:80], ''), []).append(line)
                continue
            # A build's messages name "pkg [pkg.test]"; they belong to pkg.
            pkg = (ev.get('Package') or ev.get('ImportPath') or '').split(' [')[0]
            key = (pkg, ev.get('Test') or '')
            action = ev.get('Action')
            if action in ('output', 'build-output'):
                output.setdefault(key, []).append(ev.get('Output', ''))
            elif action == 'fail':
                failed.append(key)
            elif ev.get('Test') and action == 'pass':
                passed += 1
            elif ev.get('Test') and action == 'skip':
                skipped.append(key)
            if ev.get('Test') and action == 'run':
                tests += 1

    with_tests = {p for p, t in failed if t}
    # A failing subtest's parent fails too; the deepest name says the most.
    names = [k for k in failed if k[1] and not any(o[0] == k[0] and o[1].startswith(k[1] + '/') for o in failed)]
    # A package that failed with no failing test didn't build, or died.
    names += [k for k in failed if not k[1] and k[0] not in with_tests]

    for pkg, test in names:
        text = ''.join(output.get((pkg, test), []))
        print('--- FAIL %s %s\n%s' % (pkg, test, text))
    for pkg, test in names[:MAX_ANNOTATIONS]:
        text = ''.join(output.get((pkg, test), []))[-MAX_MESSAGE:]
        title = '%s %s' % (pkg.rsplit('/', 1)[-1], test) if test else pkg
        print('::error title=%s::%s' % (escape(title, True), escape(text)))
    if len(names) > MAX_ANNOTATIONS:
        print('::error title=More failures::%d more failed; the log has them all.' % (len(names) - MAX_ANNOTATIONS))
    print('%d of %d tests passed; %d failed.' % (passed, tests, len(names)))

    bad = [k for k in skipped if no_skip and no_skip in ''.join(output.get(k, []))]
    for pkg, test in bad:
        print('::error title=%s::skipped: %s' % (escape('%s %s' % (pkg.rsplit('/', 1)[-1], test), True), escape(no_skip)))
    if bad:
        print('%d tests skipped saying "%s", which this job must not.' % (len(bad), no_skip))
    return 1 if bad else 0


if __name__ == '__main__':
    args = sys.argv[1:]
    no_skip = None
    if '--no-skip' in args:
        i = args.index('--no-skip')
        no_skip = args[i + 1]
        del args[i:i + 2]
    sys.exit(main(args[0], no_skip))
