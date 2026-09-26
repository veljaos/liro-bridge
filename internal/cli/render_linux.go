//go:build linux

package cli

// explainsEmptyList is whether `certs` says why nothing is offered, under the
// count, in the window's own words (NothingUsableKey). On Linux, because the
// count alone is SPEC §11.11's forbidden "no certificates found" (open item
// A20). Whether Windows should say it too is open, not decided by omission
// here: F12 leaves Windows' output as it was.
const explainsEmptyList = true
