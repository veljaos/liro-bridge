#!/usr/bin/env python3
"""check.sh's body: run exactly CI's steps for the jobs that run on the
Ubuntu host, read from ci.yml itself, so that an entry can cite "check.sh"
instead of "local green" (A33, D-438).

What it runs is not a list kept here. Every step is read out of the
workflow at run time and run as written: a `run:` step's script under
bash the way GitHub runs it, with the step's own env; a
golangci-lint-action step as `golangci-lint run` at the version the step
names. Anything this file cannot run exactly as CI would is **refused
before anything runs**, with the reason, rather than approximated — an
unknown action, an expression, a `sudo` that is not an apt install, a key
this file does not understand, a Go or golangci-lint that is not the
version CI uses. A refusal is exit 2; a step that failed is exit 1.

The apt steps are not run (they need root, and nothing here prompts for a
password). Each package they install is instead checked as installed,
from the step's own list, so a package CI adds is a refusal here until it
is installed, not a silent difference.

The environment is CI's where it matters and is said where it is not: the
caller's PATH and HOME (the Go caches are the caller's, warm), CI=true
(internal/chooser turns a missing dbus-daemon from a skip into a failure
under CI), LANG=C.UTF-8 as on the runner, a fresh RUNNER_TEMP per job,
GOTOOLCHAIN=local so Go never downloads another toolchain. No display
variables: CI's host has no display, and neither does a check.

Each step is bounded by `timeout` (its timeout-minutes, else 45 minutes)
and logs to its own file. The summary names the commit, whether the tree
was clean, and every step of both jobs as passed, failed, failed as a
probe (continue-on-error, as CI treats it), skipped with its reason, or
not run.
"""

import argparse
import datetime
import os
import re
import shlex
import shutil
import subprocess
import sys
import time

try:
    import yaml
except ImportError:
    print("check.sh: refused: python3's yaml module is missing (Ubuntu: python3-yaml)", file=sys.stderr)
    sys.exit(2)

# The jobs that run on the Ubuntu host with no container. The others are
# named in the summary as not run, so a claim cannot be read as covering
# them.
JOBS = ("ci", "linux-gui")
RUNNERS = ("ubuntu-latest", "ubuntu-24.04")

# Actions this file knows how to stand in for, by exact version: a new
# major version is a change in what the step does, so it is refused until
# this file is read against it.
SKIPPED_ACTIONS = {
    "actions/checkout@v4": "the tree being checked is the checkout",
    "actions/cache@v4": "a cache only; the caller's Go caches are used",
}
SETUP_GO = "actions/setup-go@v5"
LINT = "golangci/golangci-lint-action@v9"

STEP_KEYS = {"name", "run", "uses", "with", "env", "continue-on-error", "timeout-minutes"}
JOB_KEYS = {"runs-on", "steps"}
DEFAULT_BOUND_MIN = 45


class Refusal(Exception):
    pass


def step_label(job, i, step):
    return f"{job}[{i}] {step.get('name') or step.get('uses')}"


def apt_packages(job, i, step, script):
    """The packages an apt-only step installs, or None if the step is not
    one. A step that mentions sudo and is anything else is refused."""
    joined = re.sub(r"\\\n", " ", script)
    lines = [l.strip() for l in joined.splitlines() if l.strip() and not l.strip().startswith("#")]
    if not any("sudo" in l for l in lines):
        return None
    pkgs = []
    for l in lines:
        words = shlex.split(l)
        if words == ["sudo", "apt-get", "update"]:
            continue
        if words[:4] == ["sudo", "apt-get", "install", "-y"]:
            pkgs += [w for w in words[4:] if not w.startswith("-")]
            continue
        raise Refusal(f"{step_label(job, i, step)}: uses sudo for something other than apt-get update/install: {l!r}")
    if not pkgs:
        raise Refusal(f"{step_label(job, i, step)}: an apt step that installs nothing")
    return pkgs


def plan(workflow, only):
    """Every step of JOBS, classified, or a Refusal. Nothing is run."""
    with open(workflow) as f:
        w = yaml.safe_load(f)
    out = []
    for job in JOBS:
        if job not in w.get("jobs", {}):
            raise Refusal(f"job {job!r} is not in {workflow}")
        j = w["jobs"][job]
        extra = set(j) - JOB_KEYS
        if extra:
            raise Refusal(f"job {job}: keys this file does not run as CI would: {sorted(extra)}")
        if j["runs-on"] not in RUNNERS:
            raise Refusal(f"job {job}: runs-on {j['runs-on']!r}, not an Ubuntu host this file stands in for")
        for i, s in enumerate(j["steps"]):
            label = step_label(job, i, s)
            extra = set(s) - STEP_KEYS
            if extra:
                raise Refusal(f"{label}: keys this file does not run as CI would: {sorted(extra)}")
            env = {k: str(v) for k, v in (s.get("env") or {}).items()}
            for k, v in env.items():
                if "${{" in v:
                    raise Refusal(f"{label}: env {k} is an expression, which only GitHub can evaluate")
            bound = int(s.get("timeout-minutes", DEFAULT_BOUND_MIN))
            probe = bool(s.get("continue-on-error", False))
            item = {"job": job, "i": i, "name": s.get("name") or s.get("uses"), "env": env,
                    "bound": bound, "probe": probe, "label": label}
            if "run" in s:
                if "uses" in s:
                    raise Refusal(f"{label}: both run and uses")
                script = s["run"]
                if "${{" in script:
                    raise Refusal(f"{label}: its script contains an expression, which only GitHub can evaluate")
                pkgs = apt_packages(job, i, s, script)
                if pkgs is not None:
                    item.update(kind="apt", packages=pkgs)
                else:
                    item.update(kind="run", script=script)
            elif "uses" in s:
                u = s["uses"]
                with_ = s.get("with") or {}
                if u in SKIPPED_ACTIONS:
                    item.update(kind="skip", why=SKIPPED_ACTIONS[u])
                elif u == SETUP_GO:
                    bad = set(with_) - {"go-version-file", "cache"}
                    if bad or with_.get("go-version-file") != "go.mod":
                        raise Refusal(f"{label}: setup-go configured in a way this file does not stand in for: {with_}")
                    item.update(kind="setup-go")
                elif u == LINT:
                    bad = set(with_) - {"version"}
                    if bad:
                        raise Refusal(f"{label}: golangci-lint-action inputs this file does not pass on: {sorted(bad)}")
                    item.update(kind="lint", version=str(with_["version"]).lstrip("v"))
                else:
                    raise Refusal(f"{label}: unknown step type: uses {u!r}")
            else:
                raise Refusal(f"{label}: neither run nor uses")
            out.append(item)
    if only:
        keep = []
        for sel in only:
            job, _, which = sel.partition(":")
            hits = [p for p in out if p["job"] == job and (which == "" or which == str(p["i"]) or which == p["name"])]
            if not hits:
                raise Refusal(f"--only {sel!r} matches no step")
            keep += [h for h in hits if h not in keep]
        for p in out:
            p["selected"] = p in keep
    else:
        for p in out:
            p["selected"] = True
    return out


def go_mod_version(root):
    want = None
    with open(os.path.join(root, "go.mod")) as f:
        for line in f:
            m = re.match(r"^(go|toolchain)\s+(go)?(\S+)", line)
            if m:
                # setup-go prefers toolchain over go when both are present.
                if m.group(1) == "toolchain" or want is None:
                    want = m.group(3)
    if not want:
        raise Refusal("go.mod names no Go version")
    return want


def preflight(steps, root, env):
    """Refuse before running anything if the tools are not CI's."""
    sel = [p for p in steps if p["selected"]]
    if any(p["kind"] in ("run", "lint") for p in sel) or any(p["kind"] == "setup-go" for p in steps):
        want = go_mod_version(root)
        go = shutil.which("go", path=env["PATH"])
        if not go:
            raise Refusal(f"no go on PATH; CI uses {want}")
        have = subprocess.run([go, "env", "GOVERSION"], env=env, capture_output=True, text=True).stdout.strip()
        if have != "go" + want:
            raise Refusal(f"go is {have!r}; CI's setup-go installs go{want} from go.mod")
    for p in sel:
        if p["kind"] == "lint":
            lint = shutil.which("golangci-lint", path=env["PATH"])
            if not lint:
                raise Refusal(f"{p['label']}: golangci-lint is not installed; CI uses v{p['version']}")
            out = subprocess.run([lint, "version"], env=env, capture_output=True, text=True)
            text = out.stdout + out.stderr
            if not re.search(r"(?<![\d.])" + re.escape(p["version"]) + r"(?![\d.])", text):
                raise Refusal(f"{p['label']}: golangci-lint is not v{p['version']}: {text.strip()!r}")
        if p["kind"] == "apt":
            if not shutil.which("dpkg-query"):
                raise Refusal(f"{p['label']}: no dpkg-query; this stands in for CI's Ubuntu host and runs on Ubuntu")
            missing = []
            for pkg in p["packages"]:
                r = subprocess.run(["dpkg-query", "-W", "-f=${Status}", pkg], capture_output=True, text=True)
                if r.stdout.strip() != "install ok installed":
                    missing.append(pkg)
            if missing:
                raise Refusal(f"{p['label']}: not installed: {' '.join(missing)} (install them; this file does not use sudo)")


def git(root, *args):
    return subprocess.run(["git", "-C", root, *args], capture_output=True, text=True).stdout.strip()


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    ap = argparse.ArgumentParser(prog="check.sh", description=__doc__.split("\n\n")[0])
    ap.add_argument("--root", default=os.path.abspath(os.path.join(here, "..", "..")),
                    help="the tree to check (default: this repository)")
    ap.add_argument("--workflow", help="the workflow to read (default: ROOT/.github/workflows/ci.yml)")
    ap.add_argument("--only", action="append", default=[], metavar="JOB[:N|:NAME]",
                    help="run only these steps; the summary names the rest as not selected")
    ap.add_argument("--list", action="store_true", help="print the plan and exit; nothing is run")
    ap.add_argument("--logs", help="directory for the logs (default: ~/.cache/liro-check/<time>-<commit>)")
    a = ap.parse_args()
    root = os.path.abspath(a.root)
    workflow = a.workflow or os.path.join(root, ".github", "workflows", "ci.yml")

    try:
        steps = plan(workflow, a.only)
    except Refusal as e:
        print(f"check.sh: refused, nothing run: {e}", file=sys.stderr)
        return 2

    if a.list:
        for p in steps:
            how = {"run": "run", "lint": f"golangci-lint run, v{p.get('version')}", "skip": f"skip: {p.get('why')}",
                   "setup-go": "check go is go.mod's version", "apt": "check installed: " + " ".join(p.get("packages", []))}[p["kind"]]
            mark = "" if p["selected"] else "  (not selected)"
            extra = (" [probe]" if p["probe"] else "") + (f" env {p['env']}" if p["env"] else "")
            print(f"{p['label']}: {how}{extra}{mark}")
        return 0

    env = {
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "HOME": os.environ["HOME"],
        "CI": "true",
        "LANG": "C.UTF-8",
        "GOTOOLCHAIN": "local",
        "GITHUB_WORKSPACE": root,
    }
    try:
        preflight(steps, root, env)
    except Refusal as e:
        print(f"check.sh: refused, nothing run: {e}", file=sys.stderr)
        return 2

    head = git(root, "rev-parse", "HEAD")
    dirty = git(root, "status", "--porcelain")
    stamp = datetime.datetime.now(datetime.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    logs = a.logs or os.path.join(os.path.expanduser("~"), ".cache", "liro-check", f"{stamp}-{head[:7]}")
    os.makedirs(logs, exist_ok=True)
    print(f"check.sh: {root} at {head}{' — the tree has uncommitted changes' if dirty else ', clean'}")
    print(f"check.sh: workflow {workflow}; logs in {logs}")

    results = []
    failed_job = set()
    temps = {}
    for p in steps:
        job = p["job"]
        if not p["selected"]:
            results.append((p, "not selected", 0))
            continue
        if job in failed_job:
            results.append((p, "not run: an earlier step of the job failed", 0))
            continue
        if p["kind"] in ("skip", "setup-go", "apt"):
            what = {"skip": f"skipped: {p.get('why')}", "setup-go": "go matches go.mod (checked before running)",
                    "apt": "packages installed (checked before running)"}[p["kind"]]
            results.append((p, what, 0))
            continue
        if job not in temps:
            temps[job] = os.path.join(logs, f"runner-temp-{job}")
            os.makedirs(temps[job], exist_ok=True)
        senv = dict(env, RUNNER_TEMP=temps[job], **p["env"])
        log = os.path.join(logs, f"{job}-{p['i']:02d}.log")
        if p["kind"] == "run":
            script = os.path.join(logs, f"{job}-{p['i']:02d}.sh")
            with open(script, "w") as f:
                f.write(p["script"])
            # GitHub's default for a run step on Linux.
            cmd = ["bash", "--noprofile", "--norc", "-eo", "pipefail", script]
        else:
            cmd = ["golangci-lint", "run"]
        cmd = ["timeout", "--kill-after=30", f"{p['bound']}m"] + cmd
        print(f"  {p['label']} …", end="", flush=True)
        t0 = time.monotonic()
        with open(log, "w") as f:
            f.write(f"# {p['label']}\n# env: {p['env']}\n# cmd: {' '.join(cmd)}\n")
            f.flush()
            r = subprocess.run(cmd, cwd=root, env=senv, stdout=f, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)
        dt = time.monotonic() - t0
        if r.returncode == 0:
            status = "passed"
        elif r.returncode in (124, 137):
            status = f"failed: timed out after {p['bound']} min"
        else:
            status = f"failed: exit {r.returncode}"
        if status != "passed" and p["probe"]:
            status += " (a probe: continue-on-error, so the job goes on, as in CI)"
        elif status != "passed":
            failed_job.add(job)
        print(f" {status} ({dt:.0f} s)")
        if status != "passed":
            with open(log) as f:
                tail = f.readlines()[-40:]
            print("".join("      " + l for l in tail), end="")
        results.append((p, status, dt))

    print("\ncheck.sh: summary")
    print(f"  commit {head}{' + uncommitted changes' if dirty else ''}")
    for p, status, dt in results:
        print(f"  {p['label']}: {status}" + (f" ({dt:.0f} s)" if dt else ""))
    w = yaml.safe_load(open(workflow))
    others = [j for j in w.get("jobs", {}) if j not in JOBS]
    print(f"  not run by check.sh, CI's only: {', '.join(others)}")
    bad = [r for r in results if r[1].startswith("failed") and not r[0]["probe"]]
    unsel = [r for r in results if r[1] == "not selected"]
    if bad:
        print(f"check.sh: FAILED — {len(bad)} step(s)")
        return 1
    print("check.sh: every selected step passed" + (f"; {len(unsel)} not selected, so this is not a full check" if unsel else ""))
    return 0


if __name__ == "__main__":
    sys.exit(main())
