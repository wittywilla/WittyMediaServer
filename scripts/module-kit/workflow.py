#!/usr/bin/env python3
"""Checks and runs the workflows module-kit generates, for scripts/cutover-modules.sh's dry run.

  workflow.py lint <main repository's workflows folder> <workflow.yml>...
      The rules a workflow here must keep: see lint().
  workflow.py run <workflow.yml> <job> --state DIR [--checkout DIR] [--ref vX.Y.Z]
                  [--repo owner/name] [--set name=value]... [--set-file name=path]...
                  [--only "step name"]...
      Runs one job's steps here, as a runner would. A job without a checkout gets an
      empty folder, so one that needs this repository's files fails. Artifacts go
      through DIR/artifacts, and what a release would publish lands in DIR/release.

This imitates GitHub's runner and proves what a job's commands do. It can't prove what
GitHub does itself: environments, secrets, tokens and the actions it skips.
Needs PyYAML.
"""
import glob
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile

import yaml

EXPRESSION = re.compile(r"\$\{\{\s*(.*?)\s*\}\}")
PINNED = re.compile(r"^[\w.-]+/[\w./-]+@[0-9a-f]{40}$")


def load(path):
    with open(path, encoding="utf-8") as f:
        return yaml.safe_load(f)


def main_pins(folder):
    pins = {}
    for path in glob.glob(os.path.join(folder, "*.yml")):
        for job in (load(path).get("jobs") or {}).values():
            for step in job.get("steps") or []:
                if "uses" in step and "@" in step["uses"]:
                    name, sha = step["uses"].split("@", 1)
                    pins.setdefault(name, set()).add(sha)
    return pins


def lint(pins, path):
    problems = []
    wf = load(path)
    name = os.path.basename(path)
    top = wf.get("permissions")

    def bad(text):
        problems.append(f"{path}: {text}")

    if name == "ci.yml":
        if top != {"contents": "read"}:
            bad("CI's permissions must be contents: read")
    elif top != {}:
        bad("permissions must default to none")

    for job_name, job in wf["jobs"].items():
        steps = job.get("steps") or []
        text = yaml.safe_dump(job)
        checkout = any(s.get("uses", "").startswith("actions/checkout@") for s in steps)
        runs = "\n".join(s.get("run", "") for s in steps)
        if name != "ci.yml" and "permissions" not in job:
            bad(f"job {job_name} doesn't say which permissions it needs")
        for step in steps:
            uses = step.get("uses")
            if uses:
                if not PINNED.match(uses):
                    bad(f"{uses} isn't pinned to a full commit")
                action, sha = uses.split("@", 1)
                if sha not in pins.get(action, ()):
                    bad(f"{uses} isn't the commit the main repository pins")
            if "${{" in step.get("run", ""):
                bad(f"job {job_name} puts an expression inside a command: it must go through env")
            if step.get("uses", "").startswith("actions/checkout@") and (step.get("with") or {}).get("persist-credentials") is not False:
                bad(f"job {job_name} leaves the token behind after checkout")
        if "secrets." in text:
            if name == "ci.yml":
                bad("CI must not use secrets")
            if job.get("environment") != "release":
                bad(f"job {job_name} uses a secret outside the release environment")
            if checkout or re.search(r"\bnpm\b", runs):
                bad(f"job {job_name} has the key and also this repository's code")
            if re.search(r"wittymodule@v\d", runs) is None:
                bad(f"job {job_name} signs with a tool that isn't at a released version")
            if "umask 077" not in runs or 'rm -f "$key"' not in runs:
                bad(f"job {job_name} doesn't keep the key file private, or doesn't remove it")
            if "is missing" not in runs:
                bad(f"job {job_name} doesn't say so when the key is missing")
            if name == "release.yml" and "-expect " not in runs:
                bad(f"job {job_name} signs a checksum list without saying whose it must be")
            if name == "withdraw.yml" and "-list-key " not in runs:
                bad(f"job {job_name} signs a list without checking the one already there")
        if job_name == "build" and re.search(r"\b(go|npm) test\b", runs):
            bad("the job that packs a release also runs the tests, which could change what is packed")
        if (job.get("permissions") or {}).get("contents") == "write":
            if checkout or re.search(r"\b(go|npm|node)\b", runs):
                bad(f"job {job_name} can write and also runs project code")
    return problems


def expand(text, values, strict):
    def one(m):
        expr = m.group(1)
        if expr.startswith("secrets."):
            return values.get(expr, "")
        if expr in values:
            return values[expr]
        contains = re.fullmatch(r"contains\(github\.ref_name, '(.*)'\)", expr)
        if contains:
            return "true" if contains.group(1) in values["github.ref_name"] else "false"
        if strict:
            sys.exit(f"workflow.py: nothing here knows {expr}")
        return m.group(0)

    return EXPRESSION.sub(one, str(text))


def collect(pattern, cwd):
    found = []
    for line in str(pattern).splitlines():
        line = line.strip()
        if line:
            found += glob.glob(os.path.join(cwd, line))
    return [f for f in found if os.path.isfile(f)]


def run(args):
    path, job_name = args.pop(0), args.pop(0)
    options = {"--state": None, "--checkout": None, "--ref": "v0.0.0", "--repo": "wittywilla/Example"}
    values, only = {}, []
    while args:
        flag = args.pop(0)
        if flag == "--set":
            k, v = args.pop(0).split("=", 1)
            values[k] = v
        elif flag == "--set-file":
            k, v = args.pop(0).split("=", 1)
            with open(v, encoding="utf-8") as f:
                values[k] = f.read().strip()
        elif flag == "--only":
            only.append(args.pop(0))
        else:
            options[flag] = args.pop(0)
    state = options["--state"]
    wf = load(path)
    job = wf["jobs"][job_name]
    values.update({"github.ref_name": options["--ref"], "github.token": "dry-run-token", "github.repository": options["--repo"]})

    temp = tempfile.mkdtemp(prefix="runner-")
    cwd = options["--checkout"] if any(s.get("uses", "").startswith("actions/checkout@") for s in job["steps"]) else tempfile.mkdtemp(prefix="job-")
    if cwd is None:
        sys.exit(f"workflow.py: job {job_name} checks the repository out: give --checkout")
    base = dict(os.environ, GITHUB_REF_NAME=options["--ref"], GITHUB_REF_TYPE="tag", RUNNER_TEMP=temp, GITHUB_REPOSITORY=options["--repo"])

    for step in job["steps"]:
        uses = step.get("uses", "").split("@")[0]
        label = step.get("name") or step.get("run") or uses
        if only and label not in only:
            continue
        given = {k: expand(v, values, False) for k, v in (step.get("with") or {}).items()}
        if uses == "actions/upload-artifact":
            files = collect(given["path"], cwd)
            if not files and given.get("if-no-files-found") == "error":
                sys.exit(f"job {job_name}: artifact {given['name']} has no files")
            where = os.path.join(state, "artifacts", given["name"])
            shutil.rmtree(where, ignore_errors=True)
            os.makedirs(where)
            for f in files:
                shutil.copy(f, where)
            print(f"[{job_name}] artifact {given['name']}: {len(files)} files")
        elif uses == "actions/download-artifact":
            where = os.path.join(cwd, given["path"])
            os.makedirs(where, exist_ok=True)
            for f in glob.glob(os.path.join(state, "artifacts", given["name"], "*")):
                shutil.copy(f, where)
            print(f"[{job_name}] downloaded {given['name']}: {sorted(os.listdir(where))[:6]}")
        elif uses == "softprops/action-gh-release":
            where = os.path.join(state, "release")
            os.makedirs(where, exist_ok=True)
            files = collect(given.get("files", ""), cwd)
            if given.get("files") and not files and given.get("fail_on_unmatched_files") == "true":
                sys.exit(f"job {job_name}: nothing to publish")
            for f in files:
                shutil.copy(f, where)
            with open(os.path.join(state, "release.json"), "w") as out:
                json.dump({"tag": options["--ref"], "prerelease": given.get("prerelease"), "generate_release_notes": given.get("generate_release_notes"), "assets": sorted(os.listdir(where))}, out)
            print(f"[{job_name}] would publish {options['--ref']} (prerelease {given.get('prerelease')}) with {len(files)} files")
        elif uses:
            print(f"[{job_name}] skipped, GitHub's own: {uses}")
        else:
            print(f"[{job_name}] run: {label.splitlines()[0]}", flush=True)
            env = dict(base)
            env.update({k: expand(v, values, True) for k, v in (step.get("env") or {}).items()})
            script = os.path.join(temp, "step.sh")
            with open(script, "w") as f:
                f.write(step["run"])
            done = subprocess.run(["bash", "-e", script], cwd=cwd, env=env)
            if done.returncode:
                sys.exit(f"job {job_name}: step \"{label.splitlines()[0]}\" failed ({done.returncode})")
    shutil.rmtree(temp, ignore_errors=True)


def main():
    args = sys.argv[1:]
    if not args:
        sys.exit(__doc__)
    command = args.pop(0)
    if command == "lint":
        pins = main_pins(args.pop(0))
        problems = [p for path in args for p in lint(pins, path)]
        for p in problems:
            print(p)
        if problems:
            sys.exit(1)
        print(f"workflow rules: {len(args)} files ok")
    elif command == "run":
        run(args)
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
