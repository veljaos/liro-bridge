# A GTK 4 password entry whose text is set from code, then cleared — no key
# event, no human input forged (D-094); what GTK does with the text is the
# question.
import sys, gi
gi.require_version('Gtk', '4.0')
from gi.repository import Gtk, GLib
secret = open(sys.argv[1]).read()
app = Gtk.Application(application_id='invalid.liro.a11yprobe')
def activate(a):
    w = Gtk.ApplicationWindow(application=a, title='a11y probe'); e = Gtk.PasswordEntry(); w.set_child(e); w.present()
    GLib.timeout_add(2000, lambda: (e.set_text(secret), False)[1])
    GLib.timeout_add(3000, lambda: (e.set_text(''), False)[1])
    GLib.timeout_add(4500, lambda: (a.quit(), False)[1])
    print('emitter: pid', __import__('os').getpid())
app.connect('activate', activate); app.run([])
