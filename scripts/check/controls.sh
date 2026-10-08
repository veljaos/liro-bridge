#!/bin/bash
# The controls for check.sh: each shows that check.sh can say no, and says
# no for the right reason. Run on the Ubuntu VM, before check.sh's first
# green is believed (D-304: a check that cannot fail looks like a pass).
#
#   C1  red tree: D-438's parent (59b65b3^), whose CI failed at ci's GTK
#       guard and linux-gui's lint. check.sh must exit 1 with exactly
#       those two steps failed. In a git worktree, removed afterwards.
#   C2  an unknown action added to ci → refused, exit 2, "unknown step type".
#   C3  the lint step's version changed → refused, exit 2, "is not v".
#   C4  an apt step given a package that does not exist → refused, exit 2,
#       "not installed".
#   C5  an expression in a step's env → refused, exit 2, "expression".
#   C6  a sudo that is not apt → refused, exit 2, "sudo for something other".
#   C7  a Go file git ignores (dist/…), in a worktree of HEAD → refused,
#       exit 2, "Go files git ignores" (D-447). The worktree is removed.
#
# With arguments (C2 C5 …) only those run; with none, all seven.
# C2–C6 change a copy of ci.yml, never the file; C1 takes about as long as
# the red tree's first failing steps (minutes, not CI's quarter hour).
# Each control's edit is checked to be in its copy before its result is
# read (session 25 §E).
set -u
cd "$(dirname "$0")/../.." || exit 2
root=$(pwd)
work=$(mktemp -d)
pass=0
fail=0

expect() { # name, want-exit, want-text, logfile, exit
	if [ "$5" -eq "$2" ] && grep -qF -- "$3" "$4"; then
		echo "$1: as expected (exit $5, \"$3\")"
		pass=$((pass + 1))
	else
		echo "$1: NOT as expected — wanted exit $2 and \"$3\"; got exit $5:"
		sed 's/^/    /' "$4"
		fail=$((fail + 1))
	fi
}

edited() { # name, file, text that the edit must have put there
	if ! grep -qF -- "$3" "$2"; then
		echo "$1: the edit is not in the copy; the control is void"
		fail=$((fail + 1))
		return 1
	fi
}

mutate() { # out, python statements over w (the loaded workflow)
	python3 - "$root/.github/workflows/ci.yml" "$1" "$2" <<'EOF'
import sys, yaml
w = yaml.safe_load(open(sys.argv[1]))
exec(sys.argv[3])
yaml.safe_dump(w, open(sys.argv[2], "w"), sort_keys=False, allow_unicode=True)
EOF
}

want() { local c; for c in "${selected[@]}"; do [ "$c" = "$1" ] && return 0; done; return 1; }
selected=("$@")

control() { # name, mutation, marker, want-exit, want-text
	[ "${#selected[@]}" -eq 0 ] || want "$1" || return 0
	local yml="$work/$1.yml" log="$work/$1.log"
	mutate "$yml" "$2" || { echo "$1: could not make the copy"; fail=$((fail + 1)); return; }
	edited "$1" "$yml" "$3" || return
	./check.sh --workflow "$yml" >"$log" 2>&1
	expect "$1" "$4" "$5" "$log" $?
}

control C2 'w["jobs"]["ci"]["steps"].insert(2, {"name": "control C2", "uses": "example/not-an-action@v1"})' \
	"example/not-an-action@v1" 2 "unknown step type"
control C3 '[s.__setitem__("with", {"version": "v0.0.1"}) for s in w["jobs"]["linux-gui"]["steps"] if str(s.get("uses","")).startswith("golangci/")]' \
	"v0.0.1" 2 "is not v0.0.1"
control C4 '[s.__setitem__("run", s["run"].replace("libp11-kit-dev", "libp11-kit-dev liro-control-no-such-package")) for s in w["jobs"]["ci"]["steps"] if "apt-get install" in s.get("run","")]' \
	"liro-control-no-such-package" 2 "not installed: liro-control-no-such-package"
control C5 'w["jobs"]["ci"]["steps"][4].setdefault("env", {})["CONTROL"] = "${{ github.sha }}"' \
	'${{ github.sha }}' 2 "expression"
control C6 'w["jobs"]["ci"]["steps"].insert(4, {"name": "control C6", "run": "sudo true"})' \
	"sudo true" 2 "sudo for something other"

# C7: an ignored Go file, planted in a worktree of HEAD, never in this tree.
if [ "${#selected[@]}" -eq 0 ] || want C7; then
	wt7="$work/c7"
	if git worktree add --detach "$wt7" HEAD >/dev/null 2>&1; then
		mkdir -p "$wt7/dist/liro-control-c7"
		printf 'package main\n\nfunc main() {}\n' >"$wt7/dist/liro-control-c7/main.go"
		if git -C "$wt7" check-ignore -q dist/liro-control-c7/main.go; then
			./check.sh --root "$wt7" >"$work/C7.log" 2>&1
			expect C7 2 "Go files git ignores are in the tree, and ./... would sweep them where CI's checkout has none: dist/liro-control-c7/main.go" "$work/C7.log" $?
		else
			echo "C7: the planted file is not ignored in the worktree; the control is void"
			fail=$((fail + 1))
		fi
		git worktree remove --force "$wt7"
	else
		echo "C7: could not make the worktree"
		fail=$((fail + 1))
	fi
fi

# C1: the red tree, in a worktree of its own.
if [ "${#selected[@]}" -gt 0 ] && ! want C1; then
	echo "controls: $pass as expected, $fail not (selected: ${selected[*]}); work in $work"
	[ "$fail" -eq 0 ]
	exit
fi
red=$(git rev-parse 59b65b3^)
wt="$work/red"
git worktree add --detach "$wt" "$red" >/dev/null 2>&1 || { echo "C1: could not make the worktree"; exit 2; }
./check.sh --root "$wt" --logs "$work/c1-logs" >"$work/C1.log" 2>&1
c1=$?
if [ "$c1" -eq 1 ] &&
	grep -qF "ci[2] only the window packages and the agent need GTK on linux: failed" "$work/C1.log" &&
	grep -qF "linux-gui[5] golangci-lint (GOOS=linux): failed" "$work/C1.log" &&
	[ "$(grep -c ': failed' "$work/C1.log")" -ge 2 ]; then
	echo "C1: as expected (exit 1; ci's GTK guard and linux-gui's lint failed, as CI's run did)"
	grep -E '^  [a-z-]+\[[0-9]+\].*: failed' "$work/C1.log" | sed 's/^/    /'
	pass=$((pass + 1))
else
	echo "C1: NOT as expected (exit $c1):"
	sed 's/^/    /' "$work/C1.log"
	fail=$((fail + 1))
fi
git worktree remove --force "$wt"

echo "controls: $pass as expected, $fail not; work in $work"
[ "$fail" -eq 0 ]
