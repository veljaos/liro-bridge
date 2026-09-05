//go:build !windows

package platform

// NewLeader on a platform with no Explorer integration always says this
// process is the leader: there is nothing else competing for the claim,
// because nothing else starts a second copy (SPEC §11.11 scopes the
// shell integration, like every other window facility, to Windows).
func NewLeader(string) Leader { return noopLeader{} }

type noopLeader struct{}

func (noopLeader) Acquire() (bool, error) { return true, nil }
func (noopLeader) Release()               {}
