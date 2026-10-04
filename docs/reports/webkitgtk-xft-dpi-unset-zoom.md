# WebKitGTK: with gtk-xft-dpi at −1, every web view after the first gets zoom NaN

*Text for bugs.webkit.org (component WebKitGTK). To be filed by the owner —
this machine has no account there. Measured in D-424 (Fedora VM, K2–K6);
the source reading and the Ubuntu half in D-425. **Before filing**: run the
minimal reproducer below on Fedora once — it was written after the
measurements and has not been run as written; `ctxprobe.py`'s K3/K6 runs are
what was measured.*

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

**Where it appears to come from** (read, not traced). `WebCore::fontDPI()`
(`Source/WebCore/platform/gtk/PlatformScreenGtk.cpp`) returns
`SystemSettings::xftDPI() / 1024.0` whenever SystemSettings holds a value,
with no check for −1, and `SystemSettingsManagerProxy::xftDPI()`
(`UIProcess/gtk/SystemSettingsManagerProxyGtk.cpp`) passes GTK's integer on
unchanged under GTK 4. `refreshInternalScaling()` (`WebKitWebViewBase.cpp`)
multiplies the page zoom by `fontDPI() / 96 / pageScaleFactor`, and
`webkit_web_view_get_zoom_level()` returns `pageZoomFactor / pageScaleFactor`
— so with −1 the scale is −1/98304 of normal. Why the first view escapes it
(perhaps it is created before SystemSettings holds the value, and
`fontDPI()` falls back to 96) and why the result is NaN rather than a
negative number, we did not establish.

**2.52.6 behaves the same.** On Ubuntu 24.04 (WebKitGTK 2.52.6, GTK 4.14.5) with
`gtk-xft-dpi` set to −1 after `gtk_init`: view 1 zoom 1.0, page 560×420; view 2
zoom NaN, `innerWidth`/`innerHeight` 0, `devicePixelRatio` NaN; with GDK's own
98304 both views 1.0. (Run with the sandbox disabled, which the probe on that
system needs; it says nothing about the sandbox.) Ubuntu's users do not see it only
because GTK 4.14 reads GSettings itself and so has a value there (with the
schema missing it has −1 too).

Expected: −1 treated as "unknown" (as `fontDPI()` already does when no
value is held), so every view gets the 96-dpi scale; or at least the same
scale for every view of a process.

**Reproducer** (PyGObject; not yet run as written — see the note above):

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

Expected output: `view 1: zoom at creation 1.0`, `view 2: zoom at creation nan`,
and window 2 showing blue with no text. Without the `set_property` line (on
a desktop whose portal answers) both are 1.0. From the GDK source,
`GDK_DEBUG=default-settings` should give the same −1 without the
`set_property` line — not tried.

The probe that made the measurements, with the geometry and snapshot
readouts, is attached (`ctxprobe.py`; K6 is
`ctxprobe.py --context default --seq "w w" --dumpable --xft-dpi -1 --geometry`).

How we met it: our application makes its process non-dumpable
(`PR_SET_DUMPABLE 0`) to keep PINs out of core dumps and away from
same-user ptrace; xdg-desktop-portal cannot open `/proc/PID/root` for such a
process and refuses every call from it, so GTK 4.22 has no settings. We now
set `gtk-xft-dpi` from `org.gnome.desktop.interface text-scaling-factor`
ourselves when GTK has −1.
