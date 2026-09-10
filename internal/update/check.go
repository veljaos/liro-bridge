package update

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Where a release publishes itself. Both are plain release-asset
// downloads on github.com — not api.github.com — so the request needs
// no token, is not rate-limited per user, and carries no query string.
//
// SPEC §6.8 allows exactly four kinds of outbound request and this is
// the fourth. What one of these GETs sends is: the method, the path,
// the User-Agent below, and Accept. No identifier of the machine, no
// identifier of the user, and not this agent's version — see
// checkUserAgent.
const (
	// LatestManifestURL is the signed description of the newest release.
	LatestManifestURL = "https://github.com/veljaos/liro-bridge/releases/latest/download/release.json"

	// LatestSignatureURL is its detached signature.
	LatestSignatureURL = LatestManifestURL + ".sig"

	// ReleasesPageURL is where a person goes when the agent will not or
	// cannot install for them.
	ReleasesPageURL = "https://github.com/veljaos/liro-bridge/releases/latest"

	// downloadBase is where a tagged release's own assets live. An
	// artefact's URL is built from this, the tag and the artefact's
	// name — never from anything inside the manifest, so a manifest
	// cannot point a download at another host even after it has
	// verified.
	downloadBase = "https://github.com/veljaos/liro-bridge/releases/download/"
)

// checkUserAgent is deliberately a fixed string with no version in it.
//
// SPEC §6.8 forbids telemetry and F10 §5 says the check "must not send
// anything about the user or the machine — no identifier, no version
// telemetry beyond what a plain release fetch unavoidably reveals". A
// User-Agent carrying the agent's own version would be exactly that
// telemetry: it would let whoever serves the file count installations
// per version. Go's default User-Agent (Go-http-client/2.0) would say
// less again, but it says nothing about who is asking either, and a
// server operator seeing this string can at least tell an agent from a
// scraper. Nothing downstream branches on it.
const checkUserAgent = "liro-bridge"

// checkTimeout bounds one check. A person is not waiting on it — the
// daily check runs in the background and the manual one reports on a
// status line — so this is generous enough for a slow connection and
// short enough that a hung server is not a goroutine that never ends.
const checkTimeout = 20 * time.Second

// maxDownloadBytes bounds an artefact download. The MSI this project
// produces is a few megabytes; 200 MB is far above anything it will
// publish and far below what would matter on a disk.
const maxDownloadBytes = 200 << 20

// Interval is F10 §5's "once per day". It is a property of the
// schedule, not of the network: a check that fails still counts as
// having happened, so a machine that is offline every day does not
// retry every minute (SPEC §6.8: offline is normal, not an error).
const Interval = 24 * time.Hour

// Result is what a check found. Exactly one of Available, UpToDate or
// Err is meaningful, and Err being set is never a reason to show
// anybody an error unless they asked for the check themselves.
type Result struct {
	// Available is set when a verified release is newer than this
	// build. It is the only state that leads anywhere.
	Available bool

	// Manifest is the verified release. Zero unless Available.
	Manifest Manifest

	// Current is this build's own version as it was parsed, or the zero
	// Version when this build is not a released one.
	Current Version

	// Comparable is false for a build whose version is not a semantic
	// version — `dev`, which is what an unstamped build reports. Such a
	// build is never told an update is available, because there is no
	// order between "dev" and any release and offering one would offer
	// a developer an update over their own work.
	Comparable bool
}

// Checker performs the check. Its zero value is not usable; use
// NewChecker.
type Checker struct {
	client       *http.Client
	trusted      []ed25519.PublicKey
	manifest     string
	sig          string
	downloadBase string
}

// NewChecker returns a Checker over the given HTTP client. A nil
// client gets one with checkTimeout and no cookie jar — a cookie jar
// is how a plain download starts carrying state about who asked.
func NewChecker(client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{Timeout: checkTimeout}
	}
	return &Checker{
		client:       client,
		trusted:      TrustedKeys(),
		manifest:     LatestManifestURL,
		sig:          LatestSignatureURL,
		downloadBase: downloadBase,
	}
}

// withURLs points a Checker at other addresses. Unexported: a caller in
// this project never chooses where a release comes from — that is the
// point of the constants above — and a test does, through the
// package-internal helper this exists for.
func (c *Checker) withURLs(manifest, sig string) *Checker {
	c.manifest = manifest
	c.sig = sig
	return c
}

// Check fetches the newest release's manifest and its signature,
// verifies the signature against the embedded keys, and compares the
// result against currentVersion.
//
// The verification happens before the manifest is parsed and long
// before anything is downloaded (SPEC §15.2). An unverifiable manifest
// is not "a release with a problem" — it is not a release at all as far
// as this agent is concerned, and Check returns an error rather than a
// Result.
func (c *Checker) Check(ctx context.Context, currentVersion string) (Result, error) {
	manifestBytes, err := c.fetch(ctx, c.manifest, maxManifestBytes)
	if err != nil {
		return Result{}, fmt.Errorf("fetching the release manifest: %w", err)
	}
	sigBytes, err := c.fetch(ctx, c.sig, maxSignatureBytes)
	if err != nil {
		return Result{}, fmt.Errorf("fetching the release signature: %w", err)
	}

	m, err := VerifyManifest(manifestBytes, sigBytes, c.trusted)
	if err != nil {
		return Result{}, err
	}

	current, parseErr := ParseVersion(currentVersion)
	if parseErr != nil {
		// Not an error to the caller: an unstamped build is a normal
		// thing to be, and what it needs to be told is that it cannot
		// be compared, not that something went wrong.
		return Result{Manifest: m, Comparable: false}, nil
	}

	released, err := ParseVersion(m.Version)
	if err != nil {
		// ParseManifest already checked this, so reaching here means
		// the two disagree — a bug rather than a bad release.
		return Result{}, fmt.Errorf("%w: %v", ErrManifestInvalid, err)
	}

	return Result{
		Available:  released.Compare(current) > 0,
		Manifest:   m,
		Current:    current,
		Comparable: true,
	}, nil
}

// ErrUnexpectedStatus is what a fetch returns for anything but 200. It
// is distinguishable because "GitHub answered 404" (a repository with
// no release yet) and "the network is not there" call for different
// words on a status line, even though neither is a failure of this
// machine.
var ErrUnexpectedStatus = errors.New("update: unexpected HTTP status")

func (c *Checker) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", checkUserAgent)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s answered %d", ErrUnexpectedStatus, url, resp.StatusCode)
	}
	// limit+1 so a body exactly at the limit is read whole and one
	// byte over is detectable rather than silently truncated into
	// something that might still parse.
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s answered with more than %d bytes", url, limit)
	}
	return b, nil
}

// DownloadURL is where one artefact of a release lives. Built from the
// release tag and the artefact's own name rather than from anything in
// the manifest, so a manifest cannot point a download somewhere else.
func (c *Checker) DownloadURL(m Manifest, a Artefact) string {
	return fmt.Sprintf("%sv%s/%s", c.downloadBase, m.Version, a.Name)
}

// Download fetches one artefact and checks it against the digest and
// size the verified manifest carries. It returns the bytes only if
// both match.
//
// This is the second half of "verifies the signature before doing
// anything with what it downloaded": the signature covers the
// manifest, the manifest covers the digest, and the digest covers the
// file. Nothing here trusts the download itself.
func (c *Checker) Download(ctx context.Context, m Manifest, a Artefact) ([]byte, error) {
	if a.Size > maxDownloadBytes {
		return nil, fmt.Errorf("update: %s is %d bytes, which is more than this agent will download", a.Name, a.Size)
	}
	b, err := c.fetch(ctx, c.DownloadURL(m, a), a.Size)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != a.Size {
		return nil, fmt.Errorf("update: %s is %d bytes, the release says %d", a.Name, len(b), a.Size)
	}
	sum := sha256.Sum256(b)
	if got := hex.EncodeToString(sum[:]); got != a.SHA256 {
		return nil, fmt.Errorf("update: %s hashes to %s, the release says %s", a.Name, got, a.SHA256)
	}
	return b, nil
}
