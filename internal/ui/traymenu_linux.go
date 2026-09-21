//go:build linux

package ui

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

// The menu behind the tray icon, as com.canonical.dbusmenu.
//
// A StatusNotifierItem does not draw its own menu: it names an object
// that describes one, and the panel draws it. That object speaks
// DBusMenu, which is a general tree protocol — submenus, icons,
// checkmarks, radio groups, separators — of which this program needs
// the smallest possible part: five items, in a row, each of which does
// something when it is clicked.
//
// So what is implemented here is exactly that, and the parts that are
// not implemented are answered honestly rather than left out: a host
// that asks for a property this menu does not have gets an empty
// variant, and one that asks about an item that does not exist gets an
// empty group rather than an error, which is what the reference
// implementations do.

const (
	dbusMenuIface = "com.canonical.dbusmenu"
	dbusMenuPath  = dbus.ObjectPath("/MenuBar")

	// dbusMenuRootID is the id of the root item, which is not drawn.
	dbusMenuRootID int32 = 0
)

// trayMenuItem is one row.
type trayMenuItem struct {
	id     int32
	label  func() string
	action func()
}

// trayMenu is the DBusMenu object. It is exported once and its labels
// are read afresh every time a host asks, because the interface
// language is a setting and the menu that opened Settings has to be in
// the language chosen there (TrayOptions.Labels says the same on the
// other platform).
type trayMenu struct {
	mu       sync.Mutex
	items    []trayMenuItem
	revision uint32
}

func newTrayMenu(opts TrayOptions) *trayMenu {
	labels := func() TrayLabels {
		if opts.Labels == nil {
			return TrayLabels{}
		}
		return opts.Labels()
	}
	m := &trayMenu{revision: 1}
	add := func(text func() string, action func()) {
		m.items = append(m.items, trayMenuItem{
			id:     int32(len(m.items) + 1),
			label:  text,
			action: action,
		})
	}
	add(func() string { return labels().Open }, opts.OnOpen)
	add(func() string { return labels().Settings }, opts.OnSettings)
	add(func() string { return labels().Certificates }, opts.OnCertificates)
	add(func() string { return labels().AuditLog }, opts.OnAuditLog)
	add(func() string { return labels().Quit }, opts.OnQuit)
	return m
}

// properties returns one item's DBusMenu properties, filtered to what a
// host asked for. An empty filter means all of them.
func (m *trayMenu) properties(it trayMenuItem, want []string) map[string]dbus.Variant {
	all := map[string]dbus.Variant{
		"label":            dbus.MakeVariant(it.label()),
		"enabled":          dbus.MakeVariant(true),
		"visible":          dbus.MakeVariant(true),
		"type":             dbus.MakeVariant("standard"),
		"children-display": dbus.MakeVariant(""),
	}
	if len(want) == 0 {
		return all
	}
	out := make(map[string]dbus.Variant, len(want))
	for _, k := range want {
		if v, ok := all[k]; ok {
			out[k] = v
		}
	}
	return out
}

// layoutItem is DBusMenu's (ia{sv}av): an id, some properties, and
// children as variants because the structure is recursive and D-Bus
// has no recursive types.
type layoutItem struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

// GetLayout describes the menu. parentID 0 is the root; recursionDepth
// is ignored because this menu is one level deep and a host asking for
// less than all of it would still get a root with five children, which
// is all there is.
func (m *trayMenu) GetLayout(parentID int32, recursionDepth int32, propertyNames []string) (uint32, layoutItem, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if parentID != dbusMenuRootID {
		for _, it := range m.items {
			if it.id == parentID {
				return m.revision, layoutItem{ID: it.id, Props: m.properties(it, propertyNames)}, nil
			}
		}
		// An id this menu does not have. Empty rather than an error:
		// a host that asks about a row that has gone is not doing
		// anything wrong, and there is nothing to report.
		return m.revision, layoutItem{ID: parentID, Props: map[string]dbus.Variant{}}, nil
	}

	children := make([]dbus.Variant, 0, len(m.items))
	for _, it := range m.items {
		children = append(children, dbus.MakeVariant(layoutItem{
			ID:    it.id,
			Props: m.properties(it, propertyNames),
		}))
	}
	return m.revision, layoutItem{
		ID:       dbusMenuRootID,
		Props:    map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")},
		Children: children,
	}, nil
}

// groupProperty is DBusMenu's (ia{sv}).
type groupProperty struct {
	ID    int32
	Props map[string]dbus.Variant
}

// GetGroupProperties answers for several items at once, which is how a
// panel usually refreshes labels.
func (m *trayMenu) GetGroupProperties(ids []int32, propertyNames []string) ([]groupProperty, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	wanted := func(id int32) bool {
		if len(ids) == 0 {
			return true
		}
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	out := make([]groupProperty, 0, len(m.items))
	for _, it := range m.items {
		if wanted(it.id) {
			out = append(out, groupProperty{ID: it.id, Props: m.properties(it, propertyNames)})
		}
	}
	return out, nil
}

// GetProperty answers for one property of one item.
func (m *trayMenu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, it := range m.items {
		if it.id == id {
			if v, ok := m.properties(it, []string{name})[name]; ok {
				return v, nil
			}
		}
	}
	return dbus.MakeVariant(""), nil
}

// Event is a click, and the only one this menu acts on is "clicked".
//
// **The handler runs on its own goroutine**, deliberately. A menu item
// opens a window and waits for it, and this call is a method the panel
// is blocking on: answering it and then doing the work is the
// difference between a menu that closes when you click it and a panel
// that stops redrawing until the settings window is dismissed.
func (m *trayMenu) Event(id int32, eventID string, data dbus.Variant, timestamp uint32) *dbus.Error {
	if eventID != "clicked" {
		return nil
	}
	m.mu.Lock()
	var action func()
	for _, it := range m.items {
		if it.id == id {
			action = it.action
			break
		}
	}
	m.mu.Unlock()

	if action != nil {
		go action()
	}
	return nil
}

// EventGroup is Event for several at once. The reply is the list of ids
// that were not found, which for this menu is the ones it does not have.
func (m *trayMenu) EventGroup(events []struct {
	ID        int32
	EventID   string
	Data      dbus.Variant
	Timestamp uint32
}) ([]int32, *dbus.Error) {
	var notFound []int32
	for _, e := range events {
		known := false
		m.mu.Lock()
		for _, it := range m.items {
			if it.id == e.ID {
				known = true
				break
			}
		}
		m.mu.Unlock()
		if !known {
			notFound = append(notFound, e.ID)
			continue
		}
		_ = m.Event(e.ID, e.EventID, e.Data, e.Timestamp)
	}
	return notFound, nil
}

// AboutToShow says whether the menu needs redrawing before it opens.
// False: the labels are read when a host asks for them, so nothing
// changes between being asked and being shown.
func (m *trayMenu) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

// AboutToShowGroup is the same answer for several ids: none needed
// updating, and none was unknown that matters.
func (m *trayMenu) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}
