#!/bin/sh
# Does a GTK 4 password entry tell the accessibility bus what it holds?
# (D-384, D-385.) An ordinary client of the bus — a plain signal
# subscription, not a monitor — listens for AT-SPI TextChanged while a
# GtkPasswordEntry's text is set from code to a random string and cleared.
# Reports whether any payload equals the string; never prints it.
#
#   scripts/a11yprobe/run.sh            # the default environment
#   GTK_A11Y=none scripts/a11yprobe/run.sh
#
# Needs python3-gi with GTK 4. A small window appears for about five seconds.
set -eu
here=$(dirname "$0")
secret=$(mktemp)
trap 'rm -f "$secret"' EXIT
python3 -c "import secrets;print(''.join(secrets.choice('abcdefghjkmnpqrstuvwx23456789') for _ in range(20)),end='')" > "$secret"
python3 "$here/listener.py" "$secret" 9 &
listener=$!
sleep 1.5
python3 "$here/emitter.py" "$secret" 2>/dev/null
wait "$listener"
echo "GTK $(python3 -c 'import gi;gi.require_version("Gtk","4.0");from gi.repository import Gtk;print(Gtk.get_major_version(),Gtk.get_minor_version(),Gtk.get_micro_version(),sep=".")')"
