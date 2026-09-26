//go:build linux

package platform

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"
)

// pcscdSocket is where pcsc-lite's client library looks for pcscd, and
// the environment variable it honours first. Both read out of
// libpcsclite.so.1 itself (D-358), because the question this answers is
// the one a vendor PKCS#11 module will ask a moment later through that
// library, and it has to be asked in the same place.
const (
	pcscdSocketDefault = "/run/pcscd/pcscd.comm"
	pcscdSocketEnv     = "PCSCLITE_CSOCK_NAME"
)

// pcscdConnectTimeout bounds the one connection attempt. A listening
// socket accepts at once — socket activation holds it open for systemd
// even while pcscd itself is not running — so this is only ever spent on
// a machine where something is wrong in a way this check cannot name.
const pcscdConnectTimeout = 2 * time.Second

// CardServiceCheck reports whether a card reader can be reached at all
// on this machine: whether pcscd answers on its socket (F12 §9).
//
// **It is not a reader listing, and nothing may read it as one.** This
// program does not talk PC/SC — SPEC §1.1 allows dynamic linkage for the
// webview and the PKCS#11 loader and nothing else, and the vendor module
// is what reads the card. What it can learn without linking anything is
// the one fact that turns "no certificates" into an instruction: the
// service every module needs is not there. pcscd is socket-activated and
// has historically been left disabled after install (F12 §9).
//
// It connects rather than only looking for the file, because pcsc-lite
// does both: a socket file with nothing listening is SCARD_E_NO_SERVICE
// to a module exactly as a missing one is. Connecting to an activation
// socket starts pcscd, which is what the module would do next anyway;
// the connection is closed without a word.
func CardServiceCheck() func(context.Context) error {
	return checkPCSCD
}

func checkPCSCD(ctx context.Context) error {
	// Set-but-empty is pcsc-lite's empty path, not its default: measured,
	// PCSCLITE_CSOCK_NAME= gives SCARD_E_NO_SERVICE from
	// SCardEstablishContext while pcscd is up (D-358). Treating it as
	// unset would say "running" to a person whose module says otherwise.
	path, set := os.LookupEnv(pcscdSocketEnv)
	if !set {
		path = pcscdSocketDefault
	}
	d := net.Dialer{Timeout: pcscdConnectTimeout}
	conn, err := d.DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf("%w: pcscd does not answer at %s: %v", ErrSmartCardServiceDown, path, err)
	}
	_ = conn.Close()
	return nil
}
