#!/usr/bin/env python3
"""The project's Bash guard (D-422).

Refuses a Bash command that runs `go test` or `go build` unless every part of
it that does begins with `timeout` and a bound, and the bounds together stay
below the Bash tool's own limit. Twice (D-410, D-421) the tool moved an
over-long build into the background on its own, breaking the one-process
rule by nobody's decision; a note did not stop it, so this does.

A refusal is printed on stderr and says why, so that it is not mistaken for
a broken build: when it fires, the command was never run.
"""

import json
import re
import shlex
import sys

DEFAULT_TOOL_LIMIT_MS = 120_000  # the Bash tool's default when no timeout is given
MARGIN_S = 10  # room for timeout to fire and its output to come back

GO_RE = re.compile(r"\bgo\s+(test|build)\b")  # cheap prefilter on the whole text
PREFIX = {"timeout", "env", "nice", "sudo", "command", "exec"}
SPLIT_RE = re.compile(r"&&|\|\||;|\||\n")
UNIT = {"": 1, "s": 1, "m": 60, "h": 3600, "d": 86400}


def seconds(dur):
    m = re.fullmatch(r"(\d+(?:\.\d+)?)([smhd]?)", dur)
    return float(m.group(1)) * UNIT[m.group(2)] if m else None


def segments(cmd):
    """The command's simple commands, each a list of words.

    Quote-aware, so `;` or `&&` inside a commit message does not start a
    command. Newlines separate commands too (a heredoc's body is therefore
    read as commands: refused if it names the commands, which errs safe).
    If the text will not parse, it falls back to a plain split, also safe.
    """
    text = cmd.replace("\n", " ; ")
    try:
        lex = shlex.shlex(text, posix=True, punctuation_chars=True)
        lex.whitespace_split = True
        tokens = list(lex)
    except ValueError:
        return [s.split() for s in SPLIT_RE.split(cmd)]
    out, cur = [], []
    for tok in tokens:
        if tok and all(c in "&|;()" for c in tok):
            out.append(cur)
            cur = []
        else:
            cur.append(tok)
    out.append(cur)
    return out


def runs_go(words):
    """Whether `go test`/`go build` is this segment's command, not text in it.

    Skips a leading timeout (with its options and bound), env, nice, sudo and
    VAR=value words, so a commit message or a grep that names the commands is
    not refused.
    """
    i = 0
    while i < len(words):
        w = words[i]
        if w in PREFIX or w.startswith("-") or "=" in w or seconds(w) is not None:
            i += 1
            continue
        break
    return i + 1 < len(words) and words[i] == "go" and words[i + 1] in ("test", "build")


def bound(words):
    """The seconds a segment's leading `timeout` allows, or None if it has none."""
    if not words or words[0] != "timeout":
        return None
    total, i = 0.0, 1
    while i < len(words) and words[i].startswith("-"):
        w = words[i]
        if w in ("-k", "--kill-after", "-s", "--signal"):
            if w in ("-k", "--kill-after") and i + 1 < len(words):
                total += seconds(words[i + 1]) or 0
            i += 2
        elif w.startswith("--kill-after="):
            total += seconds(w.split("=", 1)[1]) or 0
            i += 1
        else:
            i += 1
    if i >= len(words):
        return None
    s = seconds(words[i])
    return None if s is None or s == 0 else total + s


def refuse(why):
    sys.stderr.write(
        "REFUSED by the project's Bash guard (.claude/hooks/bash-guard.py, D-422). "
        "This is not a build or test failure: the command was not run.\n"
        f"Why: {why}\n"
        "Rule: every `go test` / `go build` begins with `timeout <bound>`, the bounds "
        "together below the Bash tool's limit, never in the background — the tool "
        "otherwise backgrounds an over-long build on its own (D-410, D-421). "
        "Build first as its own bounded step (session 23 §E).\n"
    )
    sys.exit(2)


def main():
    data = json.load(sys.stdin)
    if data.get("tool_name") != "Bash":
        return
    inp = data.get("tool_input") or {}
    cmd = inp.get("command") or ""
    if not GO_RE.search(cmd):
        return
    segs = [s for s in segments(cmd) if runs_go(s)]
    if not segs:
        return
    if inp.get("run_in_background"):
        refuse("it runs `go test`/`go build` with run_in_background set.")
    limit_s = (inp.get("timeout") or DEFAULT_TOOL_LIMIT_MS) / 1000
    total = 0.0
    for seg in segs:
        b = bound(seg)
        if b is None:
            refuse(f"this part does not begin with `timeout <bound>`: {' '.join(seg)!r}")
        total += b
    if total > limit_s - MARGIN_S:
        refuse(
            f"its timeout bounds add up to {total:g} s, and the Bash tool's limit for "
            f"this call is {limit_s:g} s; they must stay at least {MARGIN_S} s below it."
        )


if __name__ == "__main__":
    main()
