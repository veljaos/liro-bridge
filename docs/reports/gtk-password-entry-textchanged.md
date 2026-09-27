# GtkPasswordEntry sends its contents in AT-SPI TextChanged events

*Text for the GNOME GitLab issue (GTK). To be filed by the owner — this
machine has no account there (open-items A26, D-385).*

---

**GtkPasswordEntry sends its contents in AT-SPI TextChanged events**

GTK 4.14.5 (Ubuntu 24.04, GNOME 46, Wayland), at-spi2-core 2.52.0; no
screen reader running.

A `GtkPasswordEntry` whose text is set — by typing, or by
`gtk_editable_set_text` — emits `org.a11y.atspi.Event.Object.TextChanged`
with detail `insert`, and on clearing `delete`, whose `any_data` payload is
the full plaintext.

Measured: an ordinary client of the accessibility bus, with only a plain
`AddMatch` subscription and no monitor, received both signals with the
payload byte-equal to a random 20-character string set in the entry. The same
happens with the string typed by hand. With `GTK_A11Y=none` nothing is
emitted.

Reproducer (Python, a GTK 4 window for about 5 s; compares the payload, never
prints it): `emitter.py`, `listener.py` and `run.sh`, attached — from
`scripts/a11yprobe/` in the reporting project.

Expected: a password entry's text change events should not carry the text,
or should carry the obscuring character.
