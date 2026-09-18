//go:build windows

package pkcs11

import "testing"

// The login step's own tests. Windows-only because tokenInfo and session are:
// the struct layouts they read are measured for Windows x64 and are wrong
// elsewhere rather than merely unavailable (module_other.go).
//
// **Nothing here can reach C_Login.** Every session below has a nil module, and
// every path taken returns before any m.call. That is deliberate rather than
// incidental: this card has three attempts, all three of its user-PIN flags
// are clear, and D-268 already spent one of the MUP card's establishing
// something that needed establishing. Nothing in this file needs establishing
// badly enough to risk a second.

// fakeToken returns a tokenInfo shaped like the Pošta card measured in D-273:
// a login required, no protected authentication path, 5 to 15 characters.
func fakeToken() tokenInfo {
	return tokenInfo{
		Label:        "Savka Odžić 200100123",
		SerialNumber: "2353120973204924",
		Flags:        ckfLoginRequired | ckfUserPINInitialized | ckfTokenInitialized,
		MinPINLen:    5,
		MaxPINLen:    15,
	}
}

// protectedToken is the same token with CKF_PROTECTED_AUTHENTICATION_PATH set
// — the branch no module measured so far takes, and which is checked per token
// every time because a reader with a pinpad would answer differently.
func protectedToken() tokenInfo {
	ti := fakeToken()
	ti.Flags |= ckfProtectedAuthenticationPath
	return ti
}

// TestLoginRefusesWithNoPINEntry covers the case a token that needs a PIN
// meets when nothing was wired up to collect one. It must be its own sentinel:
// a caller has to be able to tell it from a card refusing a PIN, because the
// two need completely different things said to a person.
func TestLoginRefusesWithNoPINEntry(t *testing.T) {
	s := &session{} // no module: nothing here may reach one
	if err := s.login(fakeToken(), nil, PINRequest{}); err != ErrNoPINEntry {
		t.Errorf("login with no entry returned %v, want ErrNoPINEntry", err)
	}
}

// TestLoginWipesTheBufferWhenTheEntryFails is the integration half of the
// wipe, and the reason it can run at all without a card.
//
// The entry writes a PIN into the buffer and then reports a failure — which is
// what cancelling looks like from inside login. Every path out of login runs
// the same deferred wipe, so what this measures on the error path is the
// statement that also runs on the success path.
//
// **It cannot reach C_Login.** The session has a nil module, and login returns
// the entry's error before any m.call. That is deliberate: this card has three
// attempts, all three flags are clear, and no test in this package may spend
// one (D-268 spent one of the MUP card's establishing something that needed
// establishing; nothing here needs establishing).
func TestLoginWipesTheBufferWhenTheEntryFails(t *testing.T) {
	var captured []byte
	entry := func(dst []byte, _ PINRequest) (int, error) {
		captured = dst // the same backing array login wipes
		copy(dst, "12345678")
		return 0, ErrPINCancelled
	}

	s := &session{}
	if err := s.login(fakeToken(), entry, PINRequest{}); err != ErrPINCancelled {
		t.Fatalf("login returned %v, want ErrPINCancelled", err)
	}
	if captured == nil {
		t.Fatal("the entry was never called")
	}
	if len(captured) != 15 {
		t.Errorf("the entry was handed a %d-byte buffer, want the token's own ulMaxPinLen of 15", len(captured))
	}
	for i, b := range captured {
		if b != 0 {
			t.Errorf("byte %d of the buffer is 0x%02X after login returned, want 0", i, b)
		}
	}
}

// TestLoginRefusesALengthTheTokenCannotAccept is SPEC §6.5.1's seventh clause,
// and D-268 is why it is worth a test rather than a line of code.
//
// That entry cost one of three attempts on a MUP token: C_Login with a NULL
// PIN, which the module passed to the card as an empty one, which the card
// rejected as wrong. The token had declared minPin=4 the whole time and the
// module range-checked nothing. Here the check is this layer's, it happens
// before C_Login, and — measured by the nil module — nothing reaches the card.
func TestLoginRefusesALengthTheTokenCannotAccept(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
	}{
		{"empty, which is what cost D-268 an attempt", 0},
		{"one short of the minimum", 4},
		{"one past the maximum", 16},
		{"a negative length from a broken entry", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var captured []byte
			entry := func(dst []byte, _ PINRequest) (int, error) {
				captured = dst
				for i := range dst {
					dst[i] = 'x'
				}
				return tc.n, nil
			}
			s := &session{} // nil module: if this reached C_Login it would panic, not sign
			err := s.login(fakeToken(), entry, PINRequest{})
			var lengthErr *PINLengthError
			if !asPINLengthError(err, &lengthErr) {
				t.Fatalf("login returned %v, want a *PINLengthError", err)
			}
			if lengthErr.Got != tc.n || lengthErr.Min != 5 || lengthErr.Max != 15 {
				t.Errorf("the error says got=%d min=%d max=%d", lengthErr.Got, lengthErr.Min, lengthErr.Max)
			}
			for i, b := range captured {
				if b != 0 {
					t.Errorf("byte %d is 0x%02X after a refused length, want 0", i, b)
				}
			}
		})
	}
}

func asPINLengthError(err error, out **PINLengthError) bool {
	e, ok := err.(*PINLengthError)
	if ok {
		*out = e
	}
	return ok
}

// TestLoginAsksForAPINExactlyOnce is SPEC §6.5.1's fifth clause: nothing
// retries a PIN automatically, ever, for any reason.
//
// Three wrong PINs block a card, and for a national identity card unblocking
// means a visit to a police station. A loop here would be the single most
// expensive defect this package could have, so it is measured rather than read
// for.
func TestLoginAsksForAPINExactlyOnce(t *testing.T) {
	calls := 0
	entry := func(dst []byte, _ PINRequest) (int, error) {
		calls++
		return 0, ErrPINCancelled
	}
	s := &session{}
	_ = s.login(fakeToken(), entry, PINRequest{})
	if calls != 1 {
		t.Errorf("the PIN was asked for %d times; SPEC §6.5.1 clause 5 allows exactly one", calls)
	}
}

// TestLoginTellsTheScreenWhatItNeedsToSayWhoIsAsking covers clause 6's data:
// the screen cannot say which card it is asking about unless login hands it
// the token's own label and the limits it is enforcing.
func TestLoginTellsTheScreenWhatItNeedsToSayWhoIsAsking(t *testing.T) {
	var got PINRequest
	entry := func(dst []byte, req PINRequest) (int, error) {
		got = req
		return 0, ErrPINCancelled
	}
	s := &session{}
	_ = s.login(fakeToken(), entry, PINRequest{
		TokenLabel:       "Savka Odžić 200100123",
		CertificateLabel: "Savka Odžić 200100123",
		ModulePath:       `C:\Windows\System32\aetpkss1.dll`,
	})
	if got.TokenLabel == "" || got.CertificateLabel == "" || got.ModulePath == "" {
		t.Errorf("the request reached the screen as %+v, with nothing to name the card by", got)
	}
	if got.MinLength != 5 {
		t.Errorf("the screen was told the minimum is %d, want the token's own 5", got.MinLength)
	}
}

// TestLoginRefusesAnAbsurdMaximum keeps a token that reports nonsense from
// deciding how much this layer allocates.
func TestLoginRefusesAnAbsurdMaximum(t *testing.T) {
	for _, max := range []uint32{0, MaxPINLength + 1, 1 << 20} {
		ti := fakeToken()
		ti.MaxPINLen = max
		ti.MinPINLen = 0
		called := false
		entry := func(dst []byte, _ PINRequest) (int, error) {
			called = true
			return 0, nil
		}
		s := &session{}
		if err := s.login(ti, entry, PINRequest{}); err == nil {
			t.Errorf("a token declaring ulMaxPinLen=%d was accepted", max)
		}
		if called {
			t.Errorf("a token declaring ulMaxPinLen=%d still reached the PIN screen", max)
		}
	}
}

// TestTheProtectedPathBranchAsksNobody is clause 1 from the side that can be
// measured without a module: where a token advertises a protected
// authentication path, the PIN screen is never reached at all, because the
// module or the reader collects it.
//
// What this cannot measure is the C_Login(NULL) that follows, which needs a
// module. Measured, no module this project has met takes this branch — neither
// NetSeT build on a MUP card (D-268) nor SafeSign on a Pošta card (D-273) — so
// there is nothing on this machine to measure it against. It is checked per
// token all the same, because the flag is a property of a token through a
// module rather than of an issuer.
func TestTheProtectedPathBranchAsksNobody(t *testing.T) {
	called := false
	entry := func(dst []byte, _ PINRequest) (int, error) {
		called = true
		return 0, nil
	}
	// A nil module would panic if the branch reached C_Login, which is what
	// makes "it did not ask" and "it went the other way" distinguishable here.
	defer func() {
		if recover() == nil {
			t.Error("the protected-path branch returned without calling C_Login")
		}
		if called {
			t.Error("the protected-path branch asked the PIN screen, which clause 1 forbids")
		}
	}()
	s := &session{}
	_ = s.login(protectedToken(), entry, PINRequest{})
}
