# WebKitGTK: with gtk-xft-dpi at −1, every web view after the first gets zoom NaN

*Text for bugs.webkit.org (component WebKitGTK). To be filed by the owner —
this machine has no account there. Measured in D-424 (Fedora VM, K2–K6);
the source reading and the Ubuntu half in D-425; **the reproducer below run
as written on Fedora, with a variant and a control, in D-430**. An upstream
search (D-430) found no existing report, so this is a new bug. **The
source reading's revision named in D-431** (Ubuntu VM): `webkitgtk-2.54.0`,
unchanged in 2.54.1 and on `main` the day it was read. Ready to file.*

---

**[GTK] When GtkSettings:gtk-xft-dpi is −1, the first WebKitWebView has zoom 1.0 and every later one NaN (0×0 viewport, page drawn at an enormous scale)**

WebKitGTK 2.54.0 (webkitgtk6.0-2.54.0-2.fc44), GTK 4.22.5, Fedora 44,
GNOME 50, Wayland, llvmpipe.

`GtkSettings:gtk-xft-dpi` is −1 ("use the default") when GDK gets no
settings from the platform. With GTK 4.22 on Wayland that happens whenever
the Settings portal cannot answer the process: `gdk_wayland_display_init_settings`
clears the portal proxy and "falls back to defaults", and
`gdk_wayland_display_get_setting` then returns FALSE for every translated
setting, so GtkSettings keeps −1 (`gdk/wayland/gdksettings-wayland.c`,
4.22.5). GTK 4.14 reads GSettings directly instead, outside a sandbox, and has
a value wherever the `org.gnome.desktop.interface` schema is installed.

With −1, in one process:

- the first `WebKitWebView` is created with `webkit_web_view_get_zoom_level()`
  1.0 and renders normally;
- every later view is created with zoom level **NaN** — read immediately
  after construction, before any load and before it has a web process. Its
  page then reports `innerWidth` 0, `innerHeight` 0, `devicePixelRatio` NaN
  and every element rect 0×0, while the widget is allocated 560×420 at scale
  factor 1. On screen the page is drawn at an enormous scale: the window
  shows one corner of the page's largest box (for a plain page, its
  background colour and nothing else). Loading finishes normally.

The value alone decides it, both ways:

| run | process | gtk-xft-dpi | view 2 zoom at creation | view 2 on screen |
|---|---|---|---|---|
| K3 | portal refused | −1 (GDK's) | NaN | background only |
| K4 | portal answered | 98304 (GDK's) | 1.0 | correct |
| K5 | portal refused | 98304, set by the program after `gtk_init` | 1.0 | correct |
| K6 | portal answered | −1, set by the program after `gtk_init` | NaN | magnified (red border fills the view) |

`WEBKIT_DISABLE_DMABUF_RENDERER=0` and `=1`, `WEBKIT_SKIA_ENABLE_CPU_RENDERING=1`,
a fresh `WebKitWebContext` per view and `GSK_RENDERER=cairo` were each tried
and changed nothing (all before the cause was found).

**Where it appears to come from** (read, not traced; at tag
`webkitgtk-2.54.0`, commit `5220e80b97a253c60ed899361654142ab5021998`, line
numbers there; the four functions are the same at `webkitgtk-2.54.1` and on
`main` at `3fc0c58adaefe62b3b1b538320da79a47072997c`, 2026-10-05).
`WebCore::fontDPI()` (`Source/WebCore/platform/gtk/PlatformScreenGtk.cpp:89`)
returns `SystemSettings::xftDPI() / 1024.0` whenever SystemSettings holds a
value, with no check for −1, and `SystemSettingsManagerProxy::xftDPI()`
(`Source/WebKit/UIProcess/gtk/SystemSettingsManagerProxyGtk.cpp:128`) passes
GTK's integer on unchanged under GTK 4. `refreshInternalScaling()`
(`Source/WebKit/UIProcess/API/gtk/WebKitWebViewBase.cpp:437`) multiplies the
page zoom by `fontDPI() / 96 / pageScaleFactor` when that ratio is more than
2 % from 1, and is called when the page is created and again from a
SystemSettings observer when `xftDPI` changes (`:2537`, `:2544`);
`webkit_web_view_get_zoom_level()`
(`Source/WebKit/UIProcess/API/glib/WebKitWebView.cpp:4211`) returns
`pageZoomFactor / pageScaleFactor` — so with −1 the scale is −1/98304 of
normal. Why the first view escapes it (perhaps it is created before
SystemSettings holds the value, and `fontDPI()` falls back to the primary
screen's DPI, 96 without screen data) and why the result is NaN rather
than a negative number, we did not establish.

**Seen elsewhere, probably the same root.** An application on KDE Wayland
reports WebKitGTK 2.52.6's **GTK 3** build giving `devicePixelRatio`
−0.0208 and a negative `innerWidth` — every view, negative rather than NaN —
attributed to `fontDPI()` taking GDK's −1 (FastLED/cli#226,
zackees/kernal-api#154); not reported here as far as we found.

**2.52.6 behaves the same.** On Ubuntu 24.04 (WebKitGTK 2.52.6, GTK 4.14.5) with
`gtk-xft-dpi` set to −1 after `gtk_init`: view 1 zoom 1.0, page 560×420; view 2
zoom NaN, `innerWidth`/`innerHeight` 0, `devicePixelRatio` NaN; with GDK's own
98304 both views 1.0. (Run with the sandbox disabled, which the probe on that
system needs; it says nothing about the sandbox.) Ubuntu's users do not see it only
because GTK 4.14 reads GSettings itself and so has a value there (with the
schema missing it has −1 too).

Expected: −1 treated as "unknown", so every view gets the scale
`fontDPI()` already uses when no value is held (the primary screen's DPI,
or 96); or at least the same scale for every view of a process.

**Reproducer** (PyGObject; run as written on the system above — results below):

```python
import gi
gi.require_version("Gtk", "4.0"); gi.require_version("WebKit", "6.0")
from gi.repository import Gtk, WebKit, GLib

Gtk.init()
Gtk.Settings.get_default().set_property("gtk-xft-dpi", -1)  # what GTK 4.22 has when the portal cannot answer
loop = GLib.MainLoop()
views = []

def open_view(i):
    win = Gtk.Window(title=f"view {i}", default_width=560, default_height=420)
    view = WebKit.WebView()
    print(f"view {i}: zoom at creation {view.get_zoom_level()}")
    view.load_html("<body style='background:#1d4ed8;color:#fff;font:24px sans-serif'>Hello</body>", None)
    win.set_child(view); win.present(); views.append(win)
    return False

GLib.timeout_add(0, open_view, 1)
GLib.timeout_add(3000, open_view, 2)
GLib.timeout_add(8000, loop.quit)
loop.run()
```

Three runs, each a fresh process with only `WAYLAND_DISPLAY`,
`XDG_RUNTIME_DIR`, `DBUS_SESSION_BUS_ADDRESS` and `HOME` in its environment;
the windows read by eye:

| run | what | printed | window 1 | window 2 |
|---|---|---|---|---|
| A | the script above, as written | `view 1: zoom at creation 1.0`, `view 2: zoom at creation nan` | blue, "Hello" | blue, no text |
| B | the `set_property` line replaced by a print of `gtk-xft-dpi`, run with `GDK_DEBUG=default-settings` | `gtk-xft-dpi after init -1`, then 1.0 and nan | blue, "Hello" | blue, no text |
| C (control) | B's script with nothing forced (the portal answers this process) | `gtk-xft-dpi after init 98304`, then 1.0 and 1.0 | blue, "Hello" | blue, "Hello" |

So the defect needs nothing but GTK's own "no settings" path: B reproduces it
with no property set by the program, and C shows the same script paints
both views when GTK has a value. B is the shorter reproducer — the script
without the `set_property` line, under `GDK_DEBUG=default-settings`.

The probe that made the measurements, with the geometry and snapshot
readouts, is attached (`ctxprobe.py`; K6 is
`ctxprobe.py --context default --seq "w w" --dumpable --xft-dpi -1 --geometry`).

How we met it: our application makes its process non-dumpable
(`PR_SET_DUMPABLE 0`) to keep PINs out of core dumps and away from
same-user ptrace; xdg-desktop-portal cannot open `/proc/PID/root` for such a
process and refused each call from it that we measured (Settings `ReadAll`,
FileChooser `OpenFile`, Documents), so GTK 4.22 has no settings. We now
set `gtk-xft-dpi` from `org.gnome.desktop.interface text-scaling-factor`
ourselves when GTK has −1.
