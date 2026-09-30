#!/usr/bin/python3
# B27's control (session 16 §B, D-411): the portal's FileChooser.OpenFile,
# called by a process that is dumpable or not, and nothing else.
#
#   openfile_control.py nondumpable   clears PR_SET_DUMPABLE first, as D-376 does
#   openfile_control.py dumpable      does not
#
# Prints "accepted <request path>" or "refused <error>", then closes any
# request it got, so no dialog stays on screen. Every call has a 5 s bound.
# Run under env -i with only the session bus address (session 15 §E).
import ctypes
import os
import sys

from gi.repository import Gio, GLib

mode = sys.argv[1] if len(sys.argv) == 2 else ""
if mode not in ("dumpable", "nondumpable"):
    sys.exit("usage: openfile_control.py dumpable|nondumpable")

if mode == "nondumpable":
    PR_SET_DUMPABLE = 4
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(PR_SET_DUMPABLE, 0, 0, 0, 0) != 0:
        sys.exit("prctl failed: errno %d" % ctypes.get_errno())

print("pid %d, %s" % (os.getpid(), mode), flush=True)
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
print("unique name %s" % bus.get_unique_name(), flush=True)
try:
    reply = bus.call_sync(
        "org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop",
        "org.freedesktop.portal.FileChooser", "OpenFile",
        GLib.Variant("(ssa{sv})", ("", "B27 " + mode, {})),
        GLib.VariantType("(o)"), Gio.DBusCallFlags.NONE, 5000, None)
except GLib.Error as e:
    print("refused %s" % e.message, flush=True)
    sys.exit(0)

request = reply.unpack()[0]
print("accepted %s" % request, flush=True)
try:
    bus.call_sync(
        "org.freedesktop.portal.Desktop", request,
        "org.freedesktop.portal.Request", "Close",
        None, None, Gio.DBusCallFlags.NONE, 5000, None)
    print("closed", flush=True)
except GLib.Error as e:
    print("close failed %s" % e.message, flush=True)
