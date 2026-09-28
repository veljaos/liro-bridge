#!/usr/bin/env python3
# proctree: every descendant of one exact pid, by name, with PSS for WebKit
# web processes and bubblewrap/xdg-dbus-proxy chains counted.
#
#   python3 scripts/proctree/proctree.py PID [LABEL]
#
# Written for open-items C2 and D23 (D-397). It reads /proc/*/stat for the
# parent links, not ss -p and not /proc/PID/fd: the agent is not dumpable
# (D-376), so its fd directory is root's and anything that goes through it
# sees nothing (D-396). The children are separate processes and readable.
#
# It is a count, and a count needs a control: take one reading while a
# window is open. A WebKitWebProcess count of 0 there means the instrument
# is blind, not that the leak is fixed.
import os, sys, time
root = int(sys.argv[1]); label = sys.argv[2] if len(sys.argv) > 2 else ""
kids = {}
for d in os.listdir("/proc"):
    if not d.isdigit(): continue
    try:
        with open(f"/proc/{d}/stat") as f: s = f.read()
    except OSError: continue
    name = s[s.index("(")+1:s.rindex(")")]
    ppid = int(s[s.rindex(")")+2:].split()[1])
    kids.setdefault(ppid, []).append((int(d), name))
def walk(p, depth):
    for pid, name in sorted(kids.get(p, [])):
        pss = ""
        if name.startswith("WebKitWebProces"):
            try:
                with open(f"/proc/{pid}/smaps_rollup") as f:
                    pss = next(l for l in f if l.startswith("Pss:")).split()[1] + " kB PSS"
            except (OSError, StopIteration): pss = "PSS unreadable"
        print(f"  {'  '*depth}{pid} {name} {pss}")
        yield name
        yield from walk(pid, depth+1)
alive = os.path.exists(f"/proc/{root}")
print(f"{time.strftime('%H:%M:%S')} {label} tray {root}: {'alive' if alive else 'GONE'}")
names = list(walk(root, 0))
print(f"  => WebKitWebProcess: {sum(n.startswith('WebKitWebProces') for n in names)}, "
      f"xdg-dbus-proxy: {names.count('xdg-dbus-proxy')}, "
      f"all descendants: {len(names)}")
