#!/usr/bin/env python3
"""Turns the output of `go test -json` into a short failure report for CI.

Each failing test becomes an annotation on the run, which anyone can read
without signing in, where the logs themselves need an account.
"""
import json
import sys

MAX_ANNOTATIONS = 10  # the most GitHub shows for one step
MAX_MESSAGE = 3000


def escape(s, prop=False):
    s = s.replace('%', '%25').replace('\r', '%0D').replace('\n', '%0A')
    return s.replace(':', '%3A').replace(',', '%2C') if prop else s


def main(path):
    output = {}  # (package, test) -> text
    failed = []
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


if __name__ == '__main__':
    main(sys.argv[1])
