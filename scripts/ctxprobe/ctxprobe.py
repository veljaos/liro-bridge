#!/usr/bin/env python3
# ctxprobe: D32 in a GTK host built to resemble internal/ui's — NOT the
# agent's own host (D-350/D-352), so a control must reproduce R0 before a
# variant means anything (C0 did, D-423).
#
#   python3 ctxprobe.py --context default --seq "w w" --label C0   (control)
#   python3 ctxprobe.py --context fresh   --seq "w w" --label F0
#
# Steps, run in order:
#   w  open a window, ask, then terminate_web_process + destroy (internal/ui's Close)
#   o  open a window, ask, keep it
#   c  close every kept window
#   b  one window holding TWO views side by side (B0: per surface or per process)
#   r  a window whose view is related-view of the last kept view, sharing its
#      web process (V0); needs an earlier o
#
# Readouts, which change nothing the defect depends on (K0):
#   --page gradient   the page's background is a gradient over #1d4ed8, so a
#                     frame shows the gradient and a colour fill shows flat blue
#   --view-bg COLOUR  the view's own background (WebKit's default is white),
#                     so a view nothing was drawn into shows that colour
#   --page mixed      plain #1d4ed8, and above the text four things that are not
#                     text, each in a colour the snapshot counts (K1): a box's red
#                     border, an inline SVG's cyan circle, a canvas's yellow
#                     rectangle drawn by the page's own script, a green <img>
#   --geometry        1 s after each load, both sides of the boundary (K2): the
#                     page's own innerWidth/Height, devicePixelRatio, visualViewport,
#                     scroll size, the rects of the canvas and the first text line,
#                     the image's load state; and the view's zoom level, allocated
#                     size and scale factor, and its window surface's scale
#   --xft-dpi VALUE   set GTK's gtk-xft-dpi to VALUE right after GTK starts, before
#                     any view exists (K5: 98304 in a non-dumpable process; K6: -1
#                     in a dumpable one) — every later change of it is logged
#   --snapshot        1 s after each load, webkit_web_view_get_snapshot() of the
#                     visible region to a PNG, with its distinct-colour count and
#                     its share of white (#fff text) pixels in the report
import argparse, ctypes, os, sys, time

# Before GTK initialises, as PrepareWebKitEnvironment does (only if unset).
for k in ("WEBKIT_DISABLE_DMABUF_RENDERER", "__NV_DISABLE_EXPLICIT_SYNC"):
    os.environ.setdefault(k, "1")

ap = argparse.ArgumentParser()
ap.add_argument("--context", choices=["default", "fresh"], required=True)
ap.add_argument("--seq", default="w w")
ap.add_argument("--label", default="run")
ap.add_argument("--dumpable", action="store_true")
ap.add_argument("--page", choices=["plain", "gradient", "mixed"], default="plain")
ap.add_argument("--view-bg", default=None)
ap.add_argument("--snapshot", action="store_true")
ap.add_argument("--geometry", action="store_true")
ap.add_argument("--xft-dpi", type=int, default=None)
a = ap.parse_args()

if not a.dumpable:  # as platform.ForbidCoreDumps: PR_SET_DUMPABLE 0 (B29)
    ctypes.CDLL(None, use_errno=True).prctl(4, 0, 0, 0, 0)

import gi
gi.require_version("Gtk", "4.0"); gi.require_version("WebKit", "6.0"); gi.require_version("Gdk", "4.0"); gi.require_version("JavaScriptCore", "6.0")
from gi.repository import Gtk, Gdk, WebKit, GLib, Gio

stamp = time.strftime("%Y%m%d-%H%M%S", time.gmtime())
rep = open(f"ctxprobe-report-{stamp}.txt", "w")
def say(s):
    line = time.strftime("%H:%M:%S") + f".{int(time.time()*1000)%1000:03d}  {s}"
    print(line, flush=True); rep.write(line + "\n"); rep.flush()

say(f"ctxprobe pid {os.getpid()} label {a.label} context {a.context} seq {a.seq!r} dumpable {a.dumpable} "
    f"page {a.page} view-bg {a.view_bg or '(default)'} snapshot {a.snapshot} xft-dpi {a.xft_dpi}")
for k in ("WEBKIT_DISABLE_DMABUF_RENDERER", "__NV_DISABLE_EXPLICIT_SYNC", "WEBKIT_SKIA_ENABLE_CPU_RENDERING",
          "GSK_RENDERER", "GSK_DEBUG", "GDK_BACKEND", "XDG_SESSION_TYPE"):
    say(f"env {k}={os.environ.get(k, '(unset)')}")
say(f"WebKit {WebKit.get_major_version()}.{WebKit.get_minor_version()}.{WebKit.get_micro_version()}, "
    f"GTK {Gtk.get_major_version()}.{Gtk.get_minor_version()}.{Gtk.get_micro_version()}")

import base64, struct, zlib
def solid_png(w, h, rgb):
    def chunk(t, d):
        return struct.pack(">I", len(d)) + t + d + struct.pack(">I", zlib.crc32(t + d) & 0xffffffff)
    raw = b"".join(b"\x00" + bytes(rgb) * w for _ in range(h))
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw)) + chunk(b"IEND", b""))
MARKERS = ('<div style="display:flex;gap:16px;align-items:center;margin-bottom:12px">'
           '<div style="width:80px;height:40px;border:10px solid #ff0000"></div>'
           '<svg width="80" height="80"><circle cx="40" cy="40" r="36" fill="#00ffff"/></svg>'
           '<canvas id="cv" width="100" height="60"></canvas>'
           '<img width="60" height="60" src="data:image/png;base64,'
           + base64.b64encode(solid_png(60, 60, (0, 255, 0))).decode() + '"></div>'
           '<script>document.getElementById("cv").getContext("2d").fillStyle="#ffff00";'
           'document.getElementById("cv").getContext("2d").fillRect(0,0,100,60)</script>')
COUNTED = {"red border": (255, 0, 0), "cyan SVG": (0, 255, 255), "yellow canvas": (255, 255, 0),
           "green img": (0, 255, 0)}
BG = {"mixed": "background:#1d4ed8;", "plain": "background:#1d4ed8;",
      "gradient": "background-color:#1d4ed8;"
                  "background-image:linear-gradient(135deg,#1d4ed8 0%,#16a34a 50%,#f59e0b 100%);"}[a.page]
PAGE = """<!doctype html><meta charset="utf-8"><style>
html,body{margin:0;height:100%}
body{@@BG@@ color:#fff;font:bold 40px sans-serif;display:flex;flex-direction:column;
align-items:center;justify-content:center}#clock{font-size:64px;margin-top:16px}</style>
@@MARKUP@@<div id="t1">ctxprobe · @@LABEL@@</div><div>window @@N@@ · @@CTX@@</div><div id="clock"></div>
<script>setInterval(()=>{document.getElementById('clock').textContent=new Date().toLocaleTimeString()},250)</script>"""

def serve(req):
    n = req.get_path().strip("/") or "0"
    body = (PAGE.replace("@@BG@@", BG).replace("@@MARKUP@@", MARKERS if a.page == "mixed" else "")
            .replace("@@LABEL@@", a.label).replace("@@N@@", n)
            .replace("@@CTX@@", a.context)).encode()
    req.finish(Gio.MemoryInputStream.new_from_bytes(GLib.Bytes.new(body)), len(body), "text/html")

def wire(ctx):  # liro:// as registerAssetScheme does: local, secure, one handler
    sm = ctx.get_security_manager()
    sm.register_uri_scheme_as_local("liro"); sm.register_uri_scheme_as_secure("liro")
    ctx.register_uri_scheme("liro", serve)

if a.context == "default":
    wire(WebKit.WebContext.get_default())

Gtk.init()
loop = GLib.MainLoop()
gsettings = Gtk.Settings.get_default()
def xft():
    return gsettings.get_property("gtk-xft-dpi")
say(f"GTK after init: gtk-xft-dpi {xft()}")
if a.xft_dpi is not None:
    gsettings.set_property("gtk-xft-dpi", a.xft_dpi)
    say(f"GTK: gtk-xft-dpi set by the probe to {a.xft_dpi}, reads back {xft()}")
gsettings.connect("notify::gtk-xft-dpi", lambda *_: say(f"GTK: gtk-xft-dpi changed to {xft()}"))
steps, kept, n = a.seq.split(), [], [0]

def snapshot(name, view):
    def done(v, res):
        try:
            tex = v.get_snapshot_finish(res)
        except GLib.Error as e:
            say(f"{name}: snapshot failed: {e.message}"); return
        path = f"ctxprobe-{a.label}-{name.replace(' ', '')}-{stamp}.png"
        tex.save_to_png(path)
        d = Gdk.TextureDownloader.new(tex); d.set_format(Gdk.MemoryFormat.R8G8B8A8)
        data, stride = d.download_bytes(); data = data.get_data()
        w, h = tex.get_width(), tex.get_height()
        colours, white = set(), 0
        counts = dict.fromkeys(COUNTED, 0)
        for y in range(0, h, 2):
            row = y * stride
            for x in range(0, w, 2):
                px = data[row + 4*x: row + 4*x + 3]
                colours.add(px)
                if px[0] > 240 and px[1] > 240 and px[2] > 240:
                    white += 1
                for k, c in COUNTED.items():
                    if all(abs(px[j] - c[j]) <= 30 for j in range(3)):
                        counts[k] += 1
        total = ((h + 1) // 2) * ((w + 1) // 2)
        say(f"{name}: snapshot {w}x{h} → {path}: {len(colours)} distinct colours, "
            f"near-white {100*white/total:.2f}% of sampled pixels"
            + ("; " + ", ".join(f"{k} {v} px" for k, v in counts.items()) if a.page == "mixed" else ""))
    GLib.timeout_add(1000, lambda: (view.get_snapshot(WebKit.SnapshotRegion.VISIBLE,
                                                      WebKit.SnapshotOptions.NONE, None, done), False)[1])

GEOMETRY_JS = """(() => {
  const r = (e) => { if (!e) return null; const b = e.getBoundingClientRect();
    return [Math.round(b.x), Math.round(b.y), Math.round(b.width), Math.round(b.height)]; };
  const vv = window.visualViewport, de = document.documentElement, img = document.querySelector("img");
  return JSON.stringify({innerWidth, innerHeight, devicePixelRatio,
    vv: vv ? [vv.scale, vv.width, vv.height, vv.offsetLeft, vv.offsetTop] : null,
    client: [de.clientWidth, de.clientHeight], scroll: [de.scrollWidth, de.scrollHeight, scrollX, scrollY],
    canvas: r(document.getElementById("cv")), text: r(document.getElementById("t1")), body: r(document.body),
    img: img ? [img.complete, img.naturalWidth, img.naturalHeight, img.src.slice(0, 40)] : null});
})()"""

def geometry(name, view):
    def done(v, res):
        try:
            page = v.evaluate_javascript_finish(res).to_string()
        except GLib.Error as e:
            page = f"(failed: {e.message})"
        surf = v.get_root().get_surface() if v.get_root() else None
        say(f"{name}: page {page}")
        say(f"{name}: view zoom {v.get_zoom_level()} allocated {v.get_width()}x{v.get_height()} "
            f"scale-factor {v.get_scale_factor()} surface-scale {surf.get_scale() if surf else None} gtk-xft-dpi {xft()}")
    GLib.timeout_add(1000, lambda: (view.evaluate_javascript(GEOMETRY_JS, -1, None, None, None, done), False)[1])

def make_view(related=None):
    if related is not None:
        view = WebKit.WebView(related_view=related)
    elif a.context == "fresh":
        ctx = WebKit.WebContext(); wire(ctx)
        view = WebKit.WebView(web_context=ctx)
    else:
        view = WebKit.WebView()
    say(f"view made: zoom {view.get_zoom_level()} gtk-xft-dpi {xft()}")
    if a.view_bg:
        c = Gdk.RGBA(); c.parse(a.view_bg); view.set_background_color(c)
    view.set_size_request(560, 420)
    return view

def watch(name, view, t0, loaded):
    def changed(v, ev):
        if ev == WebKit.LoadEvent.FINISHED:
            say(f"{name}: load finished {time.monotonic()-t0:.2f} s after the step began")
            if a.snapshot:
                snapshot(name, v)
            if a.geometry:
                geometry(name, v)
            loaded()
    view.connect("load-changed", changed)
    view.connect("load-failed", lambda v, e, u, err: say(f"{name}: load failed {u}: {err.message}"))

def open_window(keep, related=None):
    n[0] += 1; i = n[0]; t0 = time.monotonic()
    view = make_view(related)
    say(f"window {i}: view created{' related to the last kept view' if related else ''}, "
        f"context {hex(hash(view.get_context()))}")
    win = Gtk.Window(title=f"ctxprobe {a.label} window {i}"); win.set_resizable(False); win.set_child(view)
    watch(f"window {i}", view, t0, lambda: ask(f"window {i}", [view], win, keep))
    win.present(); view.load_uri(f"liro://app/{i}")

def open_box():
    n[0] += 1; i = n[0]; t0 = time.monotonic()
    left, right = make_view(), make_view()
    say(f"window {i}: two views in one window, contexts {hex(hash(left.get_context()))} "
        f"{hex(hash(right.get_context()))}")
    box = Gtk.Box(orientation=Gtk.Orientation.HORIZONTAL, spacing=8)
    box.append(left); box.append(right)
    win = Gtk.Window(title=f"ctxprobe {a.label} window {i} (two views)"); win.set_resizable(False)
    win.set_child(box)
    pending = [2]
    def one_loaded():
        pending[0] -= 1
        if pending[0] == 0:
            ask(f"window {i} (left = {i}L, right = {i}R)", [left, right], win, False)
    watch(f"window {i}L", left, t0, one_loaded); watch(f"window {i}R", right, t0, one_loaded)
    win.present(); left.load_uri(f"liro://app/{i}L"); right.load_uri(f"liro://app/{i}R")

def close(name, views, win):
    for v in views:
        v.terminate_web_process()
    win.destroy(); say(f"{name}: closed")

def ask(name, views, win, keep):
    say(f"{name}: what do you see? (full / background only / white / other, then Enter)")
    def got(ch, cond):  # read on GTK's loop, never blocking it
        ans = sys.stdin.readline().strip()
        say(f"{name}: person says: {ans}")
        if keep:
            kept.append((name, views, win))
        else:
            close(name, views, win)
        GLib.idle_add(next_step)
        return False
    GLib.io_add_watch(GLib.IOChannel.unix_new(sys.stdin.fileno()), GLib.PRIORITY_DEFAULT, GLib.IO_IN, got)

def next_step():
    if not steps:
        say("sequence done"); loop.quit(); return False
    s = steps.pop(0)
    if s in ("w", "o"):
        open_window(keep=(s == "o"))
    elif s == "r":
        if not kept:
            say("step r needs an earlier o; skipped"); GLib.idle_add(next_step)
        else:
            open_window(keep=True, related=kept[-1][1][0])
    elif s == "b":
        open_box()
    elif s == "c":
        for name, views, win in kept:
            close(name + " (kept)", views, win)
        kept.clear(); GLib.idle_add(next_step)
    return False

GLib.idle_add(next_step)
loop.run()
say(f"report {rep.name}")
