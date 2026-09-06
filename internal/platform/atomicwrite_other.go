//go:build !windows

package platform

// isSharingViolation reports whether err — a failed rename onto path —
// is the operating system saying another program holds the destination
// open.
//
// On POSIX systems there is no such condition. rename(2) replaces a file
// other processes have open and they go on reading the inode they
// already hold, so the refusal WriteFileAtomic's Windows sibling
// documents simply does not arise; the half-written window J-8 measured
// is closed by the same rename either way. A permission failure here is
// a permission failure, and belongs on OUTPUT_WRITE_FAILED with every
// other reason a write did not happen.
func isSharingViolation(string, error) bool { return false }
