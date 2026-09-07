package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/veljaos/liro-bridge/internal/platform"
)

// DeviceSecretLength is the size of a paired application's device
// secret: 32 random bytes (SPEC §6.2, F7 §2.1). It is the HMAC key for
// every request that application makes.
const DeviceSecretLength = 32

// appIDLength is how many random bytes an application identifier is
// made of, before hex encoding. It is an identifier, not a secret —
// it travels in a header on every request and appears in the agent's
// own log — so its only requirements are that it not collide and not
// carry meaning.
const appIDLength = 16

// SecretStore is the subset of platform.SecretStore this package needs:
// somewhere to keep a device secret encrypted at rest (SPEC §6.4).
//
// It is declared here rather than imported so that this package can be
// tested with an in-memory store on any platform, and so that the one
// place that chooses DPAPI stays cmd/liro-bridge.
// platform.NewSecretStore returns a value satisfying it.
type SecretStore interface {
	Get(name string) ([]byte, error)
	Set(name string, value []byte) error
	Delete(name string) error
}

// ErrNoSuchPairing is returned by Pairings.Revoke for an application
// identifier nothing is paired under.
var ErrNoSuchPairing = errors.New("api: no such pairing")

// Pairing is one paired application, as Settings lists it and as the
// audit log names it (F7 §2.4).
//
// It deliberately holds no secret. The device secret lives in the
// SecretStore and is fetched by identifier when a request needs
// verifying, so nothing that merely displays a pairing — the settings
// window, a log line, a JSON response — can carry one by accident.
type Pairing struct {
	// AppID is what the X-Liro-App-Id header carries.
	AppID string `json:"appId"`

	// Name is the display name bound at pairing time, already
	// sanitised (SPEC §6.6). It is the name the consent window shows,
	// always — never one supplied in a later request, or an
	// application could pair as "Test" and present itself as "Liro"
	// (SPEC §6.6, F7 §2.2).
	Name string `json:"name"`

	// Origin is the origin bound at pairing time, already sanitised and
	// shown to the person verbatim.
	Origin string `json:"origin"`

	PairedAt   time.Time `json:"pairedAt"`
	LastUsedAt time.Time `json:"lastUsedAt"`
}

// secretName is the SecretStore key a pairing's device secret lives
// under. The prefix exists so that a future secret of some other kind
// cannot collide with an application identifier.
func secretName(appID string) string { return "pairing." + appID }

// pairingsFile is the on-disk shape of pairings.json.
type pairingsFile struct {
	Version  int       `json:"version"`
	Pairings []Pairing `json:"pairings"`
}

const pairingsFileVersion = 1

// lastUsedGranularity is how much LastUsedAt has to move before it is
// written back to disk. Every authenticated request updates it, and a
// hundred-document batch is a hundred requests; persisting each one
// would mean a hundred rewrites of a file whose only reader is a
// settings window nobody has open. A minute is finer than any use this
// field has ("when was this application last active") and coarse
// enough that a batch costs one write.
const lastUsedGranularity = time.Minute

// Pairings is the agent's record of every paired application: the
// metadata in a plain JSON file, and each application's device secret
// in the SecretStore.
//
// The split is deliberate. The metadata is exactly what Settings shows
// and what the person is entitled to look at; the secret is the one
// thing that must be encrypted at rest, and keeping it somewhere else
// entirely means no code path that reads a pairing can leak one.
//
// What this does not defend against, stated plainly because it would
// otherwise look like an oversight: another process running as the
// same user can read both files and can call DPAPI with the same
// entropy. Nothing on the machine protects against that, and nothing
// is meant to — SPEC §6.5 is explicit that the human at the consent
// window is the only boundary that holds, precisely because the card
// caches its own PIN independently of which process is talking to it.
type Pairings struct {
	mu      sync.Mutex
	path    string
	secrets SecretStore
	items   map[string]*Pairing
	now     func() time.Time
}

// OpenPairings loads the pairing metadata at path, creating nothing
// until something is actually paired. secrets is where device secrets
// are kept. now may be nil, in which case time.Now is used.
func OpenPairings(path string, secrets SecretStore, now func() time.Time) (*Pairings, error) {
	if now == nil {
		now = time.Now
	}
	p := &Pairings{
		path:    path,
		secrets: secrets,
		items:   make(map[string]*Pairing),
		now:     now,
	}
	if err := p.load(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Pairings) load() error {
	b, err := os.ReadFile(p.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("api: reading the pairing list: %w", err)
	}
	var f pairingsFile
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("api: the pairing list is not valid JSON: %w", err)
	}
	if f.Version != pairingsFileVersion {
		return fmt.Errorf("api: the pairing list is version %d, this build understands %d",
			f.Version, pairingsFileVersion)
	}
	for i := range f.Pairings {
		item := f.Pairings[i]
		p.items[item.AppID] = &item
	}
	return nil
}

// saveLocked rewrites pairings.json. Held under p.mu.
func (p *Pairings) saveLocked() error {
	f := pairingsFile{Version: pairingsFileVersion, Pairings: p.listLocked()}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := platform.WriteFileAtomic(p.path, b, 0o600); err != nil {
		return fmt.Errorf("api: writing the pairing list: %w", err)
	}
	return nil
}

// listLocked returns every pairing, newest first. Held under p.mu.
func (p *Pairings) listLocked() []Pairing {
	out := make([]Pairing, 0, len(p.items))
	for _, item := range p.items {
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PairedAt.Equal(out[j].PairedAt) {
			return out[i].PairedAt.After(out[j].PairedAt)
		}
		return out[i].AppID < out[j].AppID
	})
	return out
}

// List returns every pairing, newest first — what the settings window
// renders (F7 §2.4).
func (p *Pairings) List() []Pairing {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.listLocked()
}

// Get returns the pairing for appID.
func (p *Pairings) Get(appID string) (Pairing, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	item, ok := p.items[appID]
	if !ok {
		return Pairing{}, false
	}
	return *item, true
}

// Secret returns the device secret for appID. A pairing whose metadata
// exists but whose secret does not is not a usable pairing: it reports
// false, exactly as an unknown application would, so that a half-
// written state can never be authenticated against.
func (p *Pairings) Secret(appID string) ([]byte, bool) {
	p.mu.Lock()
	_, known := p.items[appID]
	p.mu.Unlock()
	if !known {
		return nil, false
	}
	secret, err := p.secrets.Get(secretName(appID))
	if err != nil || len(secret) == 0 {
		return nil, false
	}
	return secret, true
}

// Add pairs a new application and returns it together with its device
// secret. The secret is returned exactly once, here; it is never
// readable from this type again except through Secret, which exists
// only so a request can be verified.
//
// name and origin are stored as given — the caller (the pairing flow)
// is what sanitises them, before the person ever sees them.
func (p *Pairings) Add(name, origin string) (Pairing, []byte, error) {
	appID, err := randomHex(appIDLength)
	if err != nil {
		return Pairing{}, nil, err
	}
	secret := make([]byte, DeviceSecretLength)
	if _, err := rand.Read(secret); err != nil {
		return Pairing{}, nil, fmt.Errorf("api: generating a device secret: %w", err)
	}

	// The secret goes in first. If storing it fails there is no
	// pairing, which is recoverable; the other order leaves a pairing
	// nothing can ever authenticate against.
	if err := p.secrets.Set(secretName(appID), secret); err != nil {
		return Pairing{}, nil, fmt.Errorf("api: storing the device secret: %w", err)
	}

	now := p.now()
	item := Pairing{
		AppID:      appID,
		Name:       name,
		Origin:     origin,
		PairedAt:   now,
		LastUsedAt: time.Time{},
	}

	p.mu.Lock()
	p.items[appID] = &item
	err = p.saveLocked()
	p.mu.Unlock()

	if err != nil {
		// Nothing is paired, so nothing should hold a secret for it.
		_ = p.secrets.Delete(secretName(appID))
		p.mu.Lock()
		delete(p.items, appID)
		p.mu.Unlock()
		return Pairing{}, nil, err
	}
	return item, secret, nil
}

// Revoke removes a pairing and its device secret. It is immediate (F7
// §2.4): the next request from that application authenticates against
// nothing and is refused.
//
// The secret is deleted first, for the same reason Add stores it first:
// of the two possible half-finished states, the one where a secret
// outlives its pairing cannot authenticate anything, and the one where
// a pairing outlives its secret cannot either — but only the first
// leaves no entry for a person to see and wonder about.
func (p *Pairings) Revoke(appID string) error {
	p.mu.Lock()
	_, ok := p.items[appID]
	p.mu.Unlock()
	if !ok {
		return ErrNoSuchPairing
	}
	if err := p.secrets.Delete(secretName(appID)); err != nil {
		return fmt.Errorf("api: deleting the device secret: %w", err)
	}
	p.mu.Lock()
	delete(p.items, appID)
	err := p.saveLocked()
	p.mu.Unlock()
	return err
}

// Touch records that appID made a request at now. It writes to disk
// only when LastUsedAt moves by more than lastUsedGranularity, so a
// hundred-document batch costs one write rather than a hundred.
func (p *Pairings) Touch(appID string, now time.Time) {
	p.mu.Lock()
	item, ok := p.items[appID]
	if !ok {
		p.mu.Unlock()
		return
	}
	persist := now.Sub(item.LastUsedAt) >= lastUsedGranularity
	item.LastUsedAt = now
	var err error
	if persist {
		err = p.saveLocked()
	}
	p.mu.Unlock()

	if err != nil {
		// Losing "last used" is not worth failing a request over, but
		// it is worth saying: a pairing list that silently stops
		// updating looks like an application that has stopped calling.
		slog.Warn("api: could not record when a paired application was last used", "error", err)
	}
}

// randomHex returns n random bytes, hex-encoded.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("api: reading random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// EncodeDeviceSecret renders a device secret for the one response that
// carries it. Base64, matching how every other binary value in this
// protocol travels (a digest, a document's content) — the signature and
// the digests inside the canonical string are hex because they are
// text in a text format, and this is not.
func EncodeDeviceSecret(secret []byte) string {
	return base64.StdEncoding.EncodeToString(secret)
}

// DecodeDeviceSecret reverses EncodeDeviceSecret. It exists so that a
// test, and a future SDK written in Go, share this package's own idea
// of the encoding rather than each having their own.
func DecodeDeviceSecret(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}
