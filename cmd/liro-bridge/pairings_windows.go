package main

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/api"
	"github.com/veljaos/liro-bridge/internal/i18n"
	"github.com/veljaos/liro-bridge/internal/platform"
)

// pairingsFileName is where the paired applications' metadata lives,
// beside config.json in the agent's own per-user directory. Per user
// and not per machine, so two people signed in over RDP have two sets
// (SPEC §14.1).
const pairingsFileName = "pairings.json"

// openPairings opens the agent's pairing store: the metadata file, and
// the DPAPI-backed secret store the device secrets live in (SPEC §6.4).
//
// One per process. Not one per window: the settings window revokes a
// pairing and the protocol authenticates against it, and F7 §2.4 says
// revoking is immediate — two stores over one file would each hold
// their own idea of what is paired, and a revoked application would go
// on working until whichever of them served it happened to be
// restarted.
func openPairings() (*api.Pairings, error) {
	dir := filepath.Dir(platform.DefaultConfigFile())
	secrets, err := platform.NewSecretStore(dir)
	if err != nil {
		return nil, fmt.Errorf("opening the secret store: %w", err)
	}
	return api.OpenPairings(filepath.Join(dir, pairingsFileName), secrets, nil)
}

// openPairingsOrNil is openPairings for the callers that must open a
// window whether or not it worked. A nil store renders the settings
// window's list as empty, which is what it truthfully is: nothing can
// be authenticated against a store that would not open.
func openPairingsOrNil() *api.Pairings {
	p, err := openPairings()
	if err != nil {
		slog.Error("pairings: could not open the pairing store", "error", err)
		return nil
	}
	return p
}

// listPairings returns what the settings window should show, or nothing
// when there is no store to ask.
func listPairings(p *api.Pairings) []api.Pairing {
	if p == nil {
		return nil
	}
	return p.List()
}

// jsPairings is one row per paired application, as the settings page
// renders it (F7 §2.4).
//
// Every value here was bound at pairing time and cannot be changed by a
// later request — SPEC §6.6's reason, unchanged: otherwise an
// application pairs as "Test" and later presents itself as "Liro".
func jsPairings(c *i18n.Catalogue, list []api.Pairing) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, p := range list {
		out = append(out, map[string]any{
			"appId":    p.AppID,
			"name":     p.Name,
			"origin":   p.Origin,
			"whenText": pairingWhenText(c, p),
		})
	}
	return out
}

// pairingTimeFormat is the same shape the visible stamp uses: the same
// digits in the same order in every locale, so a date read in one
// language says the same thing in another (D-125).
const pairingTimeFormat = "02.01.2006. 15:04"

// pairingWhenText is when an application was paired and when it last
// asked for anything — the two facts F7 §2.4 names, in one line.
//
// An application that has never asked says so rather than showing a
// zero time, which would read as 1 January year one and mean nothing.
func pairingWhenText(c *i18n.Catalogue, p api.Pairing) string {
	paired := p.PairedAt.Local().Format(pairingTimeFormat)
	if p.LastUsedAt.IsZero() {
		return fmt.Sprintf(c.T("settings.pairings_when_never"), paired)
	}
	return fmt.Sprintf(c.T("settings.pairings_when"), paired, p.LastUsedAt.Local().Format(pairingTimeFormat))
}

// revokePairing takes an application's pairing away, immediately (F7
// §2.4): the device secret is deleted, and the next request that
// presents it authenticates against nothing.
//
// A request naming an application that is not there is not an error
// worth showing. The person asked for it to be gone and it is gone; a
// list one click old is the only way to reach that case.
func revokePairing(c *i18n.Catalogue, p *api.Pairings, appID string) (text string, ok bool) {
	if p == nil || appID == "" {
		return "", false
	}
	if err := p.Revoke(appID); err != nil && !errors.Is(err, api.ErrNoSuchPairing) {
		slog.Warn("pairings: could not revoke a pairing", "appId", appID, "error", err)
	}
	return c.T("settings.pairings_revoked"), true
}
