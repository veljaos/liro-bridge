# GtkFileDialog.open_multiple against a portal that refuses the caller.
# Two runs differing only in prctl(PR_SET_DUMPABLE, 0), which makes
# xdg-desktop-portal refuse the process ("Unable to open /proc/PID/root").
# The dialog is opened by the program at start, no input forged (D-094); after
# 25 s without a callback the request is cancelled, so every run ends.
# Usage: python3 chooser_control.py dumpable|nodump   (D-408)
import ctypes, os, sys, time
arm = sys.argv[1]
assert arm in ("dumpable", "nodump")
if arm == "nodump":
    libc = ctypes.CDLL(None, use_errno=True)
    PR_SET_DUMPABLE = 4
    if libc.prctl(PR_SET_DUMPABLE, 0, 0, 0, 0) != 0:
        sys.exit("prctl failed: %d" % ctypes.get_errno())
st = os.lstat("/proc/self/root")
print(f"{time.strftime('%T')} arm={arm} pid={os.getpid()} /proc/self/root uid={st.st_uid}", flush=True)
import gi
gi.require_version("Gtk", "4.0")
from gi.repository import Gtk, Gio, GLib
state = {"cb": False}
cancel = Gio.Cancellable()
def on_done(dialog, res, app):
    state["cb"] = True
    try:
        files = dialog.open_multiple_finish(res)
        print(f"{time.strftime('%T')} callback: {files.get_n_items()} file(s)", flush=True)
    except GLib.Error as e:
        print(f"{time.strftime('%T')} callback: error domain={e.domain} code={e.code} msg={e.message}", flush=True)
    GLib.timeout_add_seconds(1, app.quit)
def give_up(app):
    if not state["cb"]:
        print(f"{time.strftime('%T')} no callback after 25 s; cancelling", flush=True)
        cancel.cancel()
        GLib.timeout_add_seconds(3, lambda: (print(f"{time.strftime('%T')} after cancel: callback={state['cb']}", flush=True), app.quit()))
    return False
def activate(app):
    win = Gtk.ApplicationWindow(application=app, title=f"chooser control: {arm}")
    win.set_default_size(360, 120)
    win.present()
    def start():
        d = Gtk.FileDialog(title=f"control {arm}", modal=True)
        print(f"{time.strftime('%T')} open_multiple called", flush=True)
        d.open_multiple(win, cancel, on_done, app)
        return False
    GLib.timeout_add(800, start)
    GLib.timeout_add_seconds(25, give_up, app)
app = Gtk.Application(application_id=None, flags=Gio.ApplicationFlags.NON_UNIQUE)
app.connect("activate", activate)
app.run([])
print(f"{time.strftime('%T')} exit, callback={state['cb']}", flush=True)
