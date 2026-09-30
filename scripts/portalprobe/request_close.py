#!/usr/bin/python3
# B27, second read (session 16 §B, D-411): why Request.Close found no object.
#
#   request_close.py <delay-seconds>
#
# A dumpable caller asks the portal for a file chooser with a known
# handle_token, subscribes to Response on the request's path first, asks the
# bus whether an object exists at that path (Introspect) right after the reply
# and again just before Close, calls Close after <delay-seconds>, and waits 3 s
# more for a Response. The control: Introspect on the portal's own object,
# which exists, before anything else. Every call is bounded; the whole run is
# bounded at 30 s. Run under env -i with only the session bus address.
import os
import sys
import time

from gi.repository import Gio, GLib

if len(sys.argv) != 2:
    sys.exit("usage: request_close.py <delay-seconds>")
delay = float(sys.argv[1])

DEST = "org.freedesktop.portal.Desktop"
t0 = time.monotonic()


def log(msg):
    print("%7.3f  %s" % (time.monotonic() - t0, msg), flush=True)


bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
unique = bus.get_unique_name()
token = "b27close"
path = "/org/freedesktop/portal/desktop/request/%s/%s" % (unique[1:].replace(".", "_"), token)
log("pid %d, unique name %s, delay %.1f s" % (os.getpid(), unique, delay))


def introspect(label, obj):
    try:
        xml = bus.call_sync(DEST, obj, "org.freedesktop.DBus.Introspectable", "Introspect",
                            None, GLib.VariantType("(s)"), Gio.DBusCallFlags.NONE, 5000, None).unpack()[0]
        has = "org.freedesktop.portal.Request" in xml
        log("%s: Introspect %s -> %d bytes, Request interface %s" % (label, obj, len(xml), "PRESENT" if has else "absent"))
    except GLib.Error as e:
        log("%s: Introspect %s -> error %s" % (label, obj, e.message))


introspect("control", "/org/freedesktop/portal/desktop")


def on_response(_conn, _sender, obj, _iface, _signal, params):
    code, results = params.unpack()
    log("Response on %s: code %d, keys %s" % (obj, code, sorted(results.keys())))


bus.signal_subscribe(DEST, "org.freedesktop.portal.Request", "Response", path, None,
                     Gio.DBusSignalFlags.NONE, on_response)

try:
    reply = bus.call_sync(DEST, "/org/freedesktop/portal/desktop", "org.freedesktop.portal.FileChooser", "OpenFile",
                          GLib.Variant("(ssa{sv})", ("", "B27 close %.1f" % delay, {"handle_token": GLib.Variant("s", token)})),
                          GLib.VariantType("(o)"), Gio.DBusCallFlags.NONE, 5000, None)
except GLib.Error as e:
    log("refused %s" % e.message)
    sys.exit(0)
handle = reply.unpack()[0]
log("accepted %s (%s the predicted path)" % (handle, "is" if handle == path else "is NOT"))
introspect("right after the reply", handle)

loop = GLib.MainLoop()


def close():
    introspect("just before Close", handle)
    try:
        bus.call_sync(DEST, handle, "org.freedesktop.portal.Request", "Close",
                      None, None, Gio.DBusCallFlags.NONE, 5000, None)
        log("Close returned")
    except GLib.Error as e:
        log("Close failed %s" % e.message)
    introspect("after Close", handle)
    GLib.timeout_add(3000, finish, "done")
    return False


def finish(why):
    log(why)
    loop.quit()
    return False


GLib.timeout_add(int(delay * 1000), close)
GLib.timeout_add_seconds(30, finish, "30 s bound")
loop.run()
