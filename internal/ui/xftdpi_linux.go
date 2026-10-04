//go:build linux

package ui

// GTK's missing font DPI, supplied from the desktop's own setting — D32's
// workaround (D-424, D-425). **It is a workaround, and called one.**
//
// The agent is not dumpable (D-376), so on GTK 4.22 the Settings portal
// refuses it ("Unable to open /proc/PID/root"), GDK falls back to no
// settings at all (gdksettings-wayland.c: "falling back to defaults") and
// GtkSettings keeps gtk-xft-dpi at its default, -1. WebKitGTK 2.54.0 then
// gives a process's first web view zoom 1.0 and every later one NaN: a 0×0
// viewport and the page drawn at an enormous scale, which on our white page
// is a white window (D32). The DPI value alone decides it, both ways (D-424,
// K5 and K6), so D-376's protection stays and GTK is given the value it is
// missing. Why WebKit's first view copes with -1 and the second does not is
// not known; an update can change or remove it, and this file should then
// go with it.
//
// GTK 4.14 (Ubuntu 24.04) never asks the portal outside a sandbox and reads
// the same GSettings key itself, so there GTK has the value and nothing here
// acts (read in its source and measured, D-425).
//
// The value is the desktop's text-scaling-factor, read directly through
// GSettings — dconf's own file, no portal and no /proc check — and followed
// while the process runs, so a person who changes their text scaling is not
// left with the old value (the owner). A desktop without the schema keeps -1
// and a line saying so: a number this program invented would be wrong on
// someone's machine.

import (
	"log/slog"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const (
	desktopInterfaceSchema = "org.gnome.desktop.interface"
	textScalingKey         = "text-scaling-factor"
	gtkXftDPIProperty      = "gtk-xft-dpi"

	// xftDPIUnset is GtkSettings' default for gtk-xft-dpi: no value from
	// the platform.
	xftDPIUnset = -1
)

// desktopInterface is the GSettings object whose "changed" handler keeps
// gtk-xft-dpi following the desktop. Held for the life of the process:
// were it collected, its handler would go with it and the value would
// silently stop following.
var desktopInterface *gio.Settings

// gtkLacksXftDPI reports whether GTK's gtk-xft-dpi is the one value this
// workaround replaces. Only -1: a value GTK got from anywhere — a portal
// that answered, XSETTINGS on X11, GTK 4.14's own GSettings read — is left
// exactly as it is.
func gtkLacksXftDPI(v int) bool {
	return v == xftDPIUnset
}

// xftDPIFromTextScaling turns a text-scaling-factor into gtk-xft-dpi as
// GDK 4.22.5 does for the value the portal would have given it
// (gdksettings-wayland.c, apply_portal_setting and update_xft_settings):
// the factor kept as 16.16 fixed point, then 96 dpi times it in Xft's
// 1/1024ths, truncated to an int. GDK 4.14's GSettings path multiplies
// without the fixed point, so the two differ by one unit for a factor
// such as 1.1; this follows the GTK that has the defect.
func xftDPIFromTextScaling(factor float64) int32 {
	fixed := int32(factor * 65536.0)
	return int32(96.0 * float64(fixed) / 65536.0 * 1024)
}

// desktopHasTextScaling reports whether the default schema source has
// text-scaling-factor in org.gnome.desktop.interface. Asked first because
// g_settings_new aborts the process on a schema that is not installed.
func desktopHasTextScaling(source *gio.SettingsSchemaSource) bool {
	if source == nil {
		return false
	}
	schema := source.Lookup(desktopInterfaceSchema, true)
	return schema != nil && schema.HasKey(textScalingKey)
}

// supplyMissingXftDPI gives GTK a gtk-xft-dpi when it has none. Called on
// the UI thread after gtk_init succeeds and before any web view exists;
// the GSettings object is made here so that its "changed" signal is
// delivered on this thread's loop.
func supplyMissingXftDPI() {
	settings := gtk.SettingsGetDefault()
	if settings == nil {
		return
	}
	had, ok := settings.ObjectProperty(gtkXftDPIProperty).(int)
	if !ok {
		slog.Warn("ui: GTK's gtk-xft-dpi could not be read, so it is left as it is")
		return
	}
	if !gtkLacksXftDPI(had) {
		slog.Info("ui: GTK has gtk-xft-dpi from the desktop, left as it is", "gtk-xft-dpi", had)
		return
	}
	if !desktopHasTextScaling(gio.SettingsSchemaSourceGetDefault()) {
		slog.Warn("ui: GTK has no gtk-xft-dpi and this desktop has no text-scaling-factor to take it from, "+
			"so it stays -1; with WebKitGTK 2.54 every web window after the first may be drawn blank (D32)",
			"schema", desktopInterfaceSchema, "key", textScalingKey)
		return
	}

	desktopInterface = gio.NewSettings(desktopInterfaceSchema)
	apply := func(msg string) {
		factor := desktopInterface.Double(textScalingKey)
		dpi := xftDPIFromTextScaling(factor)
		settings.SetObjectProperty(gtkXftDPIProperty, dpi) // int32: a Go int would be a gint64 GValue
		slog.Info(msg, "text-scaling-factor", factor, "gtk-xft-dpi", dpi)
	}
	// Connected before the first read: GSettings emits "changed" only for
	// a key read while a handler was connected.
	desktopInterface.ConnectChanged(func(key string) {
		if key == textScalingKey {
			apply("ui: the desktop's text scaling changed, so gtk-xft-dpi follows it (D32's workaround)")
		}
	})
	apply("ui: GTK had no gtk-xft-dpi, so it is taken from the desktop's text-scaling-factor " +
		"(D32's workaround, D-424)")
}
