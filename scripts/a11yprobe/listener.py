# An ordinary client of the accessibility bus: a plain signal subscription
# (AddMatch), which any application may make — not BecomeMonitor. Reports what
# it received; compares payloads with the test string and never prints them.
import sys, gi
gi.require_version('Gio', '2.0')
from gi.repository import Gio, GLib
secret = open(sys.argv[1]).read()
bus_addr = Gio.bus_get_sync(Gio.BusType.SESSION).call_sync('org.a11y.Bus', '/org/a11y/bus', 'org.a11y.Bus', 'GetAddress', None, GLib.VariantType('(s)'), 0, -1, None).unpack()[0]
conn = Gio.DBusConnection.new_for_address_sync(bus_addr, Gio.DBusConnectionFlags.AUTHENTICATION_CLIENT | Gio.DBusConnectionFlags.MESSAGE_BUS_CONNECTION, None, None)
seen = []
def on_signal(c, sender, path, iface, member, params):
    detail = params[0]
    payload = params[3]
    seen.append((sender, detail, payload == secret))
conn.signal_subscribe(None, 'org.a11y.atspi.Event.Object', 'TextChanged', None, None, Gio.DBusSignalFlags.NONE, on_signal)
loop = GLib.MainLoop()
GLib.timeout_add_seconds(int(sys.argv[2]), loop.quit)
loop.run()
print('listener: unique name', conn.get_unique_name(), '— a plain AddMatch subscription')
for s, d, match in seen:
    print(f'listener: TextChanged {d!r} from {s}; payload is the test string: {match}')
if not seen:
    print('listener: received no TextChanged')
