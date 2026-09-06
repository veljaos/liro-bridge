package platform

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/veljaos/liro-bridge/internal/errs"
)

// ErrDestinationInUse is the reason a rename over an existing
// destination failed: another program has that file open. On Windows
// even a reader holding it open is enough — measured (J-8): os.Rename
// over a destination any other program has open, even only for reading,
// fails with "Access is denied", where os.WriteFile succeeds.
//
// It is a sentinel rather than a bare error because the answer a person
// needs is specific and actionable — close the file and try again — and
// "access denied" is neither.
var ErrDestinationInUse = errors.New("platform: the destination file is open in another program")

// WriteFileAtomic writes data to path by creating a temporary file in
// path's own directory, writing and flushing it, and renaming it over
// path. The destination is never observed in a half-written state, and a
// write that fails partway leaves whatever was already there untouched.
//
// Why this replaced os.WriteFile (J-8). os.WriteFile opens with
// O_CREATE|O_TRUNC and then writes, so between those two moments the
// destination exists at the wrong length. Measured with four pollers
// watching a destination while a 4 MB signed document replaced an
// existing 300 KB one, the sizes another program observed at that path
// were 0 (three times — neither the old file nor the new one), 4194304
// (the new file) and 307200 (the old file). Two consequences: a program
// watching the folder can pick up an empty or partial signed document,
// and a write that fails partway — a full disk, a network drive that
// goes away — destroys a previously good signed file and leaves a
// truncated one in its place.
//
// The cost is accepted and is the point: os.Rename over a destination
// that any other program has open fails, where os.WriteFile succeeded.
// Refusing is better than destroying, and it is what careful tools do.
// That refusal is reported as errs.CodeOutputInUse, whose message says
// what to do about it.
//
// The temporary file is created in filepath.Dir(path) so the rename is
// within one volume and therefore atomic; a temporary directory
// elsewhere would make it a copy, which is exactly the half-written
// window this function exists to close. On any failure the temporary
// file is removed and the destination is left exactly as it was.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".liro-*")
	if err != nil {
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("creating a temporary file beside %s: %w", path, err))
	}
	tmpName := tmp.Name()

	// One cleanup path for every failure below, so no half-written
	// temporary file is ever left behind. Removing a file that has
	// already been renamed away is a no-op that returns an error nobody
	// needs, which is why the success path clears this first.
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(perm); err != nil && !errors.Is(err, os.ErrInvalid) {
		// Windows has no mode bits to speak of and Chmod is a near
		// no-op there; on a platform where it means something, a
		// failure to set the mode is a failure to write the file as
		// asked, not something to ignore.
		cleanup()
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("setting the mode of the temporary file for %s: %w", path, err))
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("writing the temporary file for %s: %w", path, err))
	}
	// Sync before the rename: without it the rename can be durable while
	// the content behind it is not, which on a power cut is a file that
	// exists at the right name and is empty — the very state this
	// function exists to make unobservable.
	if err := tmp.Sync(); err != nil {
		cleanup()
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("flushing the temporary file for %s: %w", path, err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("closing the temporary file for %s: %w", path, err))
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		if destinationHeldOpen(path, err) {
			return errs.WithDetails(errs.CodeOutputInUse,
				fmt.Errorf("%w: %s: %v", ErrDestinationInUse, path, err),
				map[string]any{"path": path})
		}
		return errs.New(errs.CodeOutputWriteFailed, fmt.Errorf("replacing %s: %w", path, err))
	}
	return nil
}

// destinationHeldOpen decides whether a failed rename means "another
// program has the destination open" rather than any other reason a write
// could not happen.
//
// The distinction is safe to draw at this point specifically: the
// temporary file was created, written and closed in that same directory
// moments earlier, so permission to write there is established. What is
// left is the destination itself, and isSharingViolation is where the
// platform decides which of the two it is looking at — including the
// case where the operating system reports the same status for both (see
// its Windows implementation).
func destinationHeldOpen(path string, err error) bool {
	if _, statErr := os.Stat(path); statErr != nil {
		return false // nothing there to be held open
	}
	return isSharingViolation(path, err)
}
