//go:build linux

package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// The Secret Service branch of SPEC §6.4: the desktop's own keyring —
// GNOME Keyring, KDE's ksecretd — spoken directly over the session bus.
//
// **Pure Go, no libsecret**, which is SPEC §1.1's rule and not a
// preference: "a dependency that can be satisfied in Go is not a reason
// to declare one". libsecret would put a C library and its glib stack
// into every package's Depends: for what is six D-Bus method calls.
// This file compiles at CGO_ENABLED=0, the same as the notifier
// (D-341).
const (
	ssDest  = "org.freedesktop.secrets"
	ssPath  = "/org/freedesktop/secrets"
	ssSvc   = "org.freedesktop.Secret.Service"
	ssColl  = "org.freedesktop.Secret.Collection"
	ssItem  = "org.freedesktop.Secret.Item"
	ssSess  = "org.freedesktop.Secret.Session"
	ssPrmpt = "org.freedesktop.Secret.Prompt"

	// ssTimeout bounds every call to a service this program does not
	// own and cannot restart.
	//
	// Three seconds, the same bound and for the same reason as the
	// notifier's: a keyring daemon that has not answered in three
	// seconds is one the agent stops waiting for. The fallback below it
	// is a working store, so waiting longer buys nothing and costs the
	// agent's startup — and the case this most matters in is an
	// autostarted agent on a desktop that has only just appeared.
	ssTimeout = 3 * time.Second
)

// The attributes every item this program stores carries. The Secret
// Service has no notion of a key: items are found by matching
// attributes, so these are the primary key.
//
// `xdg:schema` is the convention every Secret Service client follows
// for saying which application's item this is, and it is what makes
// this program's items identifiable in Seahorse rather than anonymous.
const (
	ssAttrSchema = "xdg:schema"
	ssAttrApp    = "application"
	ssAttrName   = "name"

	ssSchemaValue = "rs.liro.bridge.DeviceSecret"
	ssAppValue    = "liro-bridge"
)

// secretValue is the Secret Service's (oayays) struct: the session the
// transfer belongs to, algorithm parameters, the bytes, and a content
// type.
type secretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

// secretServiceStore is a SecretStore backed by the desktop's keyring.
//
// # The transfer algorithm is "plain", and that is decided rather than defaulted
//
// The Secret Service offers two ways to move a secret across the bus:
// "plain", which puts the bytes in the message, and a Diffie-Hellman
// negotiation that encrypts them under a per-session AES key. F12 §7
// asks for this to be decided with a reason, and the reason is a
// measurement rather than an argument from convenience (D-347):
//
//   - **Plain does put the secret on the bus in the clear.** Measured
//     on this desktop with a D-Bus monitor: the value appears twice,
//     once in CreateItem and once in GetSecret's reply. Under DH it
//     appears zero times, and a string that was never sent appears zero
//     times — so the instrument could see the absence it reports.
//   - **But an unrelated process running as the same user does not need
//     to eavesdrop.** Measured: a different binary, with a different
//     parent, which never saw the bus traffic, called SearchItems and
//     GetSecret and was handed the secret. gnome-keyring applies no
//     per-application access control to an unlocked collection.
//
// So DH defends against a strictly weaker adversary than the one the
// keyring already admits through the front door: anyone who could
// monitor this program's bus traffic can equally well ask the keyring
// for the item. Encrypting the wire between two parties who will both
// hand the plaintext to the same caller is ceremony, and ceremony that
// looks like protection is worse than none — it is what a reader would
// point at when asking whether this is safe.
//
// What plain would cost, if any of these were true, is written down so
// that the decision can be re-taken rather than inherited: a bus daemon
// that logged message bodies (measured: neither journal contains the
// probe's secret), a desktop that authorised GetSecret per application
// but not monitoring, or a Secret Service reached across anything other
// than a unix socket only this uid can open.
type secretServiceStore struct {
	conn       *dbus.Conn
	collection dbus.ObjectPath
	label      string
}

// ssUnavailable says why the Secret Service branch could not be taken,
// in the words the log line and the settings window will carry.
type ssUnavailable struct{ reason string }

func (e ssUnavailable) Error() string { return e.reason }

// newSecretServiceStore opens the desktop's keyring, or says why it
// could not be used.
//
// **It never raises a prompt.** Measured on GNOME 46 (D-347): nothing
// in the Secret Service interface shows a dialog until Prompt.Prompt()
// is called, and this program never calls it. A locked collection
// answers Unlock with a prompt object, which is dismissed here and
// treated as "locked" — because the case this is written for is an
// agent started at login, asking before the person has unlocked
// anything, and a modal password dialog in front of a desktop that has
// just appeared is not something this program may cause.
func newSecretServiceStore() (*secretServiceStore, error) {
	conn, err := ssSessionBus()
	if err != nil {
		return nil, ssUnavailable{"there is no session bus"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), ssTimeout)
	defer cancel()
	service := conn.Object(ssDest, ssPath)

	// ReadAlias is the first call and doubles as the availability
	// check: if nothing owns org.freedesktop.secrets and nothing can be
	// activated to own it, this fails with ServiceUnknown rather than
	// hanging, and the timeout bounds an activation that goes wrong.
	var collection dbus.ObjectPath
	if err := service.CallWithContext(ctx, ssSvc+".ReadAlias", 0, "default").Store(&collection); err != nil {
		return nil, ssUnavailable{"this desktop has no Secret Service: " + dbusReason(err)}
	}
	if collection == "" || collection == "/" {
		return nil, ssUnavailable{"this desktop's Secret Service has no default collection"}
	}

	coll := conn.Object(ssDest, collection)
	label := ""
	if v, err := coll.GetProperty(ssColl + ".Label"); err == nil {
		label, _ = v.Value().(string)
	}

	locked, err := ssLocked(coll)
	if err != nil {
		return nil, ssUnavailable{"this desktop's Secret Service would not say whether it is locked: " + dbusReason(err)}
	}
	if locked {
		// One attempt at the unlock that needs nobody: a daemon that
		// already holds the password answers Unlock with the collection
		// and a "/" prompt. Anything else is a dialog, and a dialog is
		// refused.
		var unlocked []dbus.ObjectPath
		var prompt dbus.ObjectPath
		if err := service.CallWithContext(ctx, ssSvc+".Unlock", 0,
			[]dbus.ObjectPath{collection}).Store(&unlocked, &prompt); err != nil {
			return nil, ssUnavailable{"the keyring is locked and would not unlock: " + dbusReason(err)}
		}
		if prompt != "" && prompt != "/" {
			// Dismiss rather than leave it: the prompt object is the
			// service's, it was created because this program asked, and
			// an undismissed one is a dialog somebody else's code may
			// later show on this program's behalf.
			_ = conn.Object(ssDest, prompt).CallWithContext(ctx, ssPrmpt+".Dismiss", 0).Err
		}
		if locked, err := ssLocked(coll); err != nil || locked {
			return nil, ssUnavailable{"the keyring is locked, and unlocking it would have asked this person for a password before they asked for anything"}
		}
	}

	return &secretServiceStore{conn: conn, collection: collection, label: label}, nil
}

// ssSessionBus is how this file reaches the session bus, as a variable
// so that a test can point it at a bus that is not there and assert
// that the fallback carries a reason. It is dbus.SessionBus in every
// build; nothing but a test replaces it.
var ssSessionBus = dbus.SessionBus

// ssLocked reads a collection's Locked property.
func ssLocked(coll dbus.BusObject) (bool, error) {
	v, err := coll.GetProperty(ssColl + ".Locked")
	if err != nil {
		return false, err
	}
	b, ok := v.Value().(bool)
	if !ok {
		return false, fmt.Errorf("platform: the Secret Service's Locked property is %T, not a boolean", v.Value())
	}
	return b, nil
}

// dbusReason strips D-Bus's error name down to what is worth putting in
// a log line, keeping the name when there is no message.
func dbusReason(err error) string {
	var de dbus.Error
	if errors.As(err, &de) {
		for _, b := range de.Body {
			if s, ok := b.(string); ok && s != "" {
				return s
			}
		}
		return de.Name
	}
	return err.Error()
}

// Describe implements SecretStore.
func (s *secretServiceStore) Describe() SecretStoreDescription {
	return SecretStoreDescription{Mechanism: MechanismSecretService, Detail: s.label}
}

// attributes is the attribute map identifying one stored secret.
func ssAttributes(name string) map[string]string {
	return map[string]string{
		ssAttrSchema: ssSchemaValue,
		ssAttrApp:    ssAppValue,
		ssAttrName:   name,
	}
}

// session opens a transfer session and returns it with a closer.
//
// One session per operation rather than one held for the life of the
// agent. A held session is state the service may drop — on its own
// restart, on a logout and back — and a stale session path fails a call
// that has nothing wrong with it. These operations happen a handful of
// times per pairing, so the extra round trip costs nothing worth the
// staleness.
func (s *secretServiceStore) session(ctx context.Context) (dbus.ObjectPath, func(), error) {
	var out dbus.Variant
	var sess dbus.ObjectPath
	err := s.conn.Object(ssDest, ssPath).
		CallWithContext(ctx, ssSvc+".OpenSession", 0, "plain", dbus.MakeVariant("")).
		Store(&out, &sess)
	if err != nil {
		return "", func() {}, fmt.Errorf("platform: opening a Secret Service session: %w", err)
	}
	return sess, func() {
		_ = s.conn.Object(ssDest, sess).Call(ssSess+".Close", 0).Err
	}, nil
}

// search returns the item paths matching name, unlocked ones first.
func (s *secretServiceStore) search(ctx context.Context, name string) (unlocked, locked []dbus.ObjectPath, err error) {
	err = s.conn.Object(ssDest, ssPath).
		CallWithContext(ctx, ssSvc+".SearchItems", 0, ssAttributes(name)).
		Store(&unlocked, &locked)
	if err != nil {
		return nil, nil, fmt.Errorf("platform: searching the keyring: %w", err)
	}
	return unlocked, locked, nil
}

// Get implements SecretStore.
func (s *secretServiceStore) Get(name string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ssTimeout)
	defer cancel()

	unlocked, locked, err := s.search(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(unlocked) == 0 {
		if len(locked) > 0 {
			// Deliberately not ErrSecretNotFound. The secret exists and
			// this program is being told it may not have it; answering
			// "no such secret" would make a locked keyring look like an
			// unpaired application, and the remedy for those two is not
			// the same.
			return nil, errors.New("platform: the keyring holding this secret is locked")
		}
		return nil, ErrSecretNotFound
	}

	sess, closeSession, err := s.session(ctx)
	if err != nil {
		return nil, err
	}
	defer closeSession()

	var got secretValue
	if err := s.conn.Object(ssDest, unlocked[0]).
		CallWithContext(ctx, ssItem+".GetSecret", 0, sess).Store(&got); err != nil {
		return nil, fmt.Errorf("platform: reading a secret from the keyring: %w", err)
	}
	if len(got.Parameters) != 0 {
		// The service answered a plain session with algorithm
		// parameters, which means it encrypted a transfer this program
		// asked to be plain. Returning the bytes would return
		// ciphertext as if it were the secret.
		return nil, errors.New("platform: the keyring answered a plain session with encrypted parameters")
	}
	return got.Value, nil
}

// Set implements SecretStore.
func (s *secretServiceStore) Set(name string, value []byte) error {
	if len(value) == 0 {
		return errEmptySecret
	}
	ctx, cancel := context.WithTimeout(context.Background(), ssTimeout)
	defer cancel()

	sess, closeSession, err := s.session(ctx)
	if err != nil {
		return err
	}
	defer closeSession()

	props := map[string]dbus.Variant{
		ssItem + ".Label":      dbus.MakeVariant(ssLabel(name)),
		ssItem + ".Attributes": dbus.MakeVariant(ssAttributes(name)),
	}
	var item, prompt dbus.ObjectPath
	err = s.conn.Object(ssDest, s.collection).
		CallWithContext(ctx, ssColl+".CreateItem", 0, props,
			secretValue{Session: sess, Value: value, ContentType: "application/octet-stream"}, true).
		Store(&item, &prompt)
	if err != nil {
		return fmt.Errorf("platform: writing a secret to the keyring: %w", err)
	}
	if item == "" || item == "/" {
		// A "/" item with a prompt is the service saying it needs a
		// dialog to finish. This program does not raise one, and a Set
		// that silently did nothing would be worse than one that says
		// it did nothing.
		return errors.New("platform: the keyring would not store this secret without asking this person to unlock it")
	}
	return nil
}

// Delete implements SecretStore.
func (s *secretServiceStore) Delete(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), ssTimeout)
	defer cancel()

	unlocked, locked, err := s.search(ctx, name)
	if err != nil {
		return err
	}
	for _, item := range append(append([]dbus.ObjectPath{}, unlocked...), locked...) {
		var prompt dbus.ObjectPath
		if err := s.conn.Object(ssDest, item).
			CallWithContext(ctx, ssItem+".Delete", 0).Store(&prompt); err != nil {
			return fmt.Errorf("platform: deleting a secret from the keyring: %w", err)
		}
	}
	return nil
}

// ssLabel is what a person sees beside this item in their keyring
// manager. It names the program and the pairing, because an item
// labelled "secret" in a list of forty is an item nobody can decide
// about.
func ssLabel(name string) string {
	return "Liro Bridge — " + strings.TrimSpace(name)
}
