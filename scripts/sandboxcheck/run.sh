#!/bin/sh
# Does WebKitGTK's sandbox start for the installed agent on this kernel, and
# does it fail for the same binary where no AppArmor profile names it?
# (F12 §3.2's sandbox question; open-items B7/F6; D-388.)
#
# Run as the desktop user, in the graphical session, after each boot of the
# kernel-and-update sitting. Writes ~/sandboxcheck-<kernel>-<time>.txt. Needs
# no root; the loaded AppArmor profiles are root-only, so `sudo aa-status`
# is a separate step for the person.
#
# Two windows appear for a few seconds each, and the a11y reproducer's for
# about five. Nothing is clicked (D-094). The agent already running at login
# is not touched: each run gets its own home and runtime directory.
set -u
here=$(cd "$(dirname "$0")" && pwd)
out="$HOME/sandboxcheck-$(uname -r)-$(date -u +%Y%m%d-%H%M%S).txt"
exec > "$out" 2>&1

echo "== context"
echo "kernel: $(uname -r)"
echo "apparmor_restrict_unprivileged_userns: $(cat /proc/sys/kernel/apparmor_restrict_unprivileged_userns 2>&1)"
echo "this shell: $(readlink /proc/$$/exe)"
dpkg-query -W -f='${Package} ${Version}\n' apparmor bubblewrap libgtk-4-1 libwebkitgtk-6.0-4 gnome-shell liro-bridge 2>&1
ls /etc/apparmor.d/ | grep -E '^liro' | sed 's/^/profile file: /'

# open_window BINARY LABEL — start `open` with its own home and a short
# runtime directory (a Unix socket path must fit in 108 bytes, D-378), wait,
# report whether it is alive and which processes run under it, then end it
# by its own PID.
#
# **Started from this shell, never from Python.** The first version launched
# from python3, and this VM carries an AppArmor profile from an earlier
# session (liro-f12-probe) that gives /usr/bin/python3.12 the userns
# permission: everything Python started inherited it, the "unprofiled"
# control's sandbox started too, and the check could tell nothing apart
# (D-388). /bin/sh is named by no profile.
descendants() {
    for c in $(pgrep -P "$1"); do
        cat "/proc/$c/comm" 2>/dev/null
        descendants "$c"
    done
}
open_window() {
    bin=$1
    w=$(mktemp -d /tmp/sbx-XXXX)
    mkdir -p "$w/run" && chmod 700 "$w/run"
    HOME=$w XDG_CONFIG_HOME=$w/c XDG_DATA_HOME=$w/d XDG_STATE_HOME=$w/s \
        XDG_CACHE_HOME=$w/k XDG_RUNTIME_DIR=$w/run \
        "$bin" open >/dev/null 2>"$w/stderr" &
    pid=$!
    sleep 10
    if kill -0 "$pid" 2>/dev/null; then
        kids=$(descendants "$pid" | sort -u | tr '\n' ' ')
        echo "alive after 10 s: yes (pid $pid, parent $(ps -o comm= -p $$))"
        echo "processes under it: ${kids:-none}"
        case "$kids" in
        *bwrap*WebKitWebProces*|*WebKitWebProces*bwrap*) echo "sandbox started (bwrap and WebKitWebProcess): yes" ;;
        *) echo "sandbox started (bwrap and WebKitWebProcess): no" ;;
        esac
        kill "$pid"
        wait "$pid" 2>/dev/null
    else
        wait "$pid"
        echo "alive after 10 s: no, exit $?"
        echo "sandbox started (bwrap and WebKitWebProcess): no"
    fi
    echo "stderr, bwrap and WebKit lines:"
    grep -i -E 'bwrap|uid map|sandbox|dbus-proxy|failed' "$w/stderr" | head -5 | sed 's/^/    /'
    grep -h -i -E 'bwrap|sandbox|webkit|could not open' "$w/s/liro/logs/bridge.log" 2>/dev/null | head -3 | cut -c1-200 | sed 's/^/    log: /'
    rm -rf "$w"
}

echo
echo "== the installed agent, /usr/bin/liro-bridge (the package's profile names it)"
open_window /usr/bin/liro-bridge

echo
echo "== control: the same binary where no profile names it"
c=$(mktemp -d /tmp/sbx-bin-XXXX)
cp /usr/bin/liro-bridge "$c/liro-bridge-unprofiled"
open_window "$c/liro-bridge-unprofiled"
rm -rf "$c"

echo
echo "== accessibility: does a GtkPasswordEntry send its text (D-384)?"
sh "$here/../a11yprobe/run.sh" 2>&1 | grep -E 'listener:|^GTK'

echo
echo "written: $out"
