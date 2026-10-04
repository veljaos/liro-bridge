#!/usr/bin/env python3
# ctxprobe: does a fresh WebKitWebContext per view make the second view paint?
# Session 24, from webviewjs/webview#53's "fresh WebContext works". A GTK host
# built to resemble internal/ui's, NOT the agent's own host (D-350/D-352), so
# its control must reproduce R0 before its variant means anything.
#
#   python3 ctxprobe.py --context default --seq "w w" --label C0   (control)
#   python3 ctxprobe.py --context fresh   --seq "w w" --label F0   (variant)
#
# w: open a window, ask what is seen, then terminate_web_process + destroy
#    (internal/ui's Close). o: open and keep. c: close every kept one.
import argparse, ctypes, os, sys, time

# Before GTK initialises, as PrepareWebKitEnvironment does (only if unset).
for k in ("WEBKIT_DISABLE_DMABUF_RENDERER", "__NV_DISABLE_EXPLICIT_SYNC"):
    os.environ.setdefault(k, "1")

ap = argparse.ArgumentParser()
ap.add_argument("--context", choices=["default", "fresh"], required=True)
ap.add_argument("--seq", default="w w")
ap.add_argument("--label", default="run")
ap.add_argument("--dumpable", action="store_true")
a = ap.parse_args()

if not a.dumpable:  # as platform.ForbidCoreDumps: PR_SET_DUMPABLE 0 (B29)
    ctypes.CDLL(None, use_errno=True).prctl(4, 0, 0, 0, 0)

import gi
gi.require_version("Gtk", "4.0"); gi.require_version("WebKit", "6.0")
from gi.repository import Gtk, WebKit, GLib, Gio

rep = open(time.strftime("ctxprobe-report-%Y%m%d-%H%M%S.txt", time.gmtime()), "w")
def say(s):
    line = time.strftime("%H:%M:%S") + f".{int(time.time()*1000)%1000:03d}  {s}"
    print(line, flush=True); rep.write(line + "\n"); rep.flush()

say(f"ctxprobe pid {os.getpid()} label {a.label} context {a.context} seq {a.seq!r} dumpable {a.dumpable}")
for k in ("WEBKIT_DISABLE_DMABUF_RENDERER", "__NV_DISABLE_EXPLICIT_SYNC", "WEBKIT_SKIA_ENABLE_CPU_RENDERING", "GSK_RENDERER", "GDK_BACKEND",
          "XDG_SESSION_TYPE"):
    say(f"env {k}={os.environ.get(k, '(unset)')}")
say(f"WebKit {WebKit.get_major_version()}.{WebKit.get_minor_version()}.{WebKit.get_micro_version()}, "
    f"GTK {Gtk.get_major_version()}.{Gtk.get_minor_version()}.{Gtk.get_micro_version()}")

PAGE = """<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;height:100%}
body{background:#1d4ed8;color:#fff;font:bold 40px sans-serif;display:flex;flex-direction:column;
align-items:center;justify-content:center}#clock{font-size:64px;margin-top:16px}</style>
<div>ctxprobe · LABEL</div><div>window N · CTX</div><div id="clock"></div>
<script>setInterval(()=>{document.getElementById('clock').textContent=new Date().toLocaleTimeString()},250)</script>"""

def serve(req):
    n = req.get_path().strip("/") or "0"
    body = PAGE.replace("LABEL", a.label).replace("N", n).replace("CTX", a.context).encode()
    req.finish(Gio.MemoryInputStream.new_from_bytes(GLib.Bytes.new(body)), len(body), "text/html")

def wire(ctx):  # liro:// as registerAssetScheme does: local, secure, one handler
    sm = ctx.get_security_manager()
    sm.register_uri_scheme_as_local("liro"); sm.register_uri_scheme_as_secure("liro")
    ctx.register_uri_scheme("liro", serve)

if a.context == "default":
    wire(WebKit.WebContext.get_default())

Gtk.init()
loop = GLib.MainLoop()
steps, kept, n = a.seq.split(), [], [0]

def open_window(keep):
    n[0] += 1; i = n[0]; t0 = time.monotonic()
    if a.context == "fresh":
        ctx = WebKit.WebContext(); wire(ctx)
        view = WebKit.WebView(web_context=ctx)
    else:
        view = WebKit.WebView()
    say(f"window {i}: view created, context {hex(hash(view.get_context()))}")
    def changed(v, ev):
        if ev == WebKit.LoadEvent.FINISHED:
            say(f"window {i}: load finished {time.monotonic()-t0:.2f} s after the step began")
            ask(i, v, win, keep)
    view.connect("load-changed", changed)
    view.connect("load-failed", lambda v, e, u, err: say(f"window {i}: load failed {u}: {err.message}"))
    view.set_size_request(560, 420)
    win = Gtk.Window(title=f"ctxprobe {a.label} window {i}"); win.set_resizable(False); win.set_child(view)
    win.present(); view.load_uri(f"liro://app/{i}")

def ask(i, view, win, keep):
    say(f"window {i}: what do you see? (full / background only / white / other, then Enter)")
    def got(ch, cond):  # read on GTK's loop, never blocking it
        ans = sys.stdin.readline().strip()
        say(f"window {i}: person says: {ans}")
        if keep:
            kept.append((i, view, win))
        else:
            view.terminate_web_process(); win.destroy(); say(f"window {i}: closed")
        GLib.idle_add(next_step)
        return False
    GLib.io_add_watch(GLib.IOChannel.unix_new(sys.stdin.fileno()), GLib.PRIORITY_DEFAULT, GLib.IO_IN, got)

def next_step():
    if not steps:
        say("sequence done"); loop.quit(); return False
    s = steps.pop(0)
    if s in ("w", "o"):
        open_window(keep=(s == "o"))
    elif s == "c":
        for i, v, w in kept:
            v.terminate_web_process(); w.destroy(); say(f"window {i}: closed (kept)")
        kept.clear(); GLib.idle_add(next_step)
    return False

GLib.idle_add(next_step)
loop.run()
say(f"report {rep.name}")
