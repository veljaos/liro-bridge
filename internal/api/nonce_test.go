package api

import (
	"fmt"
	"testing"
	"time"
)

// fakeClock is a clock a test moves by hand, so nothing here waits five
// real minutes to observe a five-minute rule.
type fakeClock struct{ t time.Time }

func newFakeClock() *fakeClock {
	return &fakeClock{t: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func TestNonceIsFreshOnceAndReusedThereafter(t *testing.T) {
	clock := newFakeClock()
	c := NewNonceCache(clock.Now)

	if fresh, full := c.Use("app", "n1"); !fresh || full {
		t.Fatalf("first use: fresh=%v full=%v, want true/false", fresh, full)
	}
	if fresh, full := c.Use("app", "n1"); fresh || full {
		t.Fatalf("replay: fresh=%v full=%v, want false/false", fresh, full)
	}
}

func TestNonceIsForgottenAfterItsTTL(t *testing.T) {
	clock := newFakeClock()
	c := NewNonceCache(clock.Now)

	c.Use("app", "n1")
	clock.Advance(NonceTTL - time.Second)
	if fresh, _ := c.Use("app", "n1"); fresh {
		t.Fatal("a nonce was accepted again one second inside its own TTL")
	}
	clock.Advance(2 * time.Second)
	if fresh, _ := c.Use("app", "n1"); !fresh {
		t.Fatal("a nonce was still remembered after its TTL had passed")
	}
}

// The cache expires entries as it goes rather than growing for the life
// of the process. Nothing sweeps on a schedule; every use sweeps.
func TestNonceCacheSweepsAsItGoes(t *testing.T) {
	clock := newFakeClock()
	c := NewNonceCache(clock.Now)

	for i := range 500 {
		c.Use("app", fmt.Sprintf("n%d", i))
	}
	if got := c.Len(); got != 500 {
		t.Fatalf("cache holds %d entries, want 500", got)
	}

	clock.Advance(NonceTTL + time.Second)
	// One more use is all it takes: the sweep runs under the same lock,
	// on the same call, and everything that has aged out goes with it.
	c.Use("app", "later")
	if got := c.Len(); got != 1 {
		t.Fatalf("cache holds %d entries after the TTL passed, want 1 (the new one)", got)
	}
}

// Two applications independently choosing the same random nonce is
// otherwise a refused request for one of them, for a reason neither
// could ever diagnose. Scoping cannot weaken replay protection: a
// replay carries the original signature, which verifies only under the
// original application's secret, so it only ever reaches its own scope.
func TestNoncesAreScopedPerApplication(t *testing.T) {
	c := NewNonceCache(newFakeClock().Now)

	if fresh, _ := c.Use("app-a", "shared"); !fresh {
		t.Fatal("first application's nonce was not fresh")
	}
	if fresh, _ := c.Use("app-b", "shared"); !fresh {
		t.Fatal("a second application was refused for a nonce the first one used")
	}
	if fresh, _ := c.Use("app-a", "shared"); fresh {
		t.Fatal("the first application's own replay was accepted")
	}
}

func TestNonceCacheReportsWhenItIsFull(t *testing.T) {
	clock := newFakeClock()
	const limit = 32
	c := newNonceCacheWithLimit(clock.Now, limit)

	for i := range limit {
		if fresh, full := c.Use("app", fmt.Sprintf("n%d", i)); !fresh || full {
			t.Fatalf("entry %d: fresh=%v full=%v", i, fresh, full)
		}
	}
	fresh, full := c.Use("app", "one-too-many")
	if fresh || !full {
		t.Fatalf("past the cap: fresh=%v full=%v, want false/true", fresh, full)
	}
	// Reporting full rather than evicting is the point: evicting the
	// oldest entry would silently re-open the replay window the cache
	// exists to close.
	if fresh, _ := c.Use("app", "n0"); fresh {
		t.Fatal("an entry was evicted to make room; the replay window is open again")
	}
}

func TestForgetDropsOneApplicationsNonces(t *testing.T) {
	c := NewNonceCache(newFakeClock().Now)
	c.Use("app-a", "n")
	c.Use("app-b", "n")

	c.Forget("app-a")
	if got := c.Len(); got != 1 {
		t.Fatalf("cache holds %d entries after forgetting one application, want 1", got)
	}
	if fresh, _ := c.Use("app-b", "n"); fresh {
		t.Fatal("the other application's nonce was dropped too")
	}
}
