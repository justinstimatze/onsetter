// Package secfile creates directories and writes files at a mode that
// actually holds.
//
// os.MkdirAll and os.WriteFile only apply the mode they're given when they
// create the path — an already-existing directory or file keeps whatever
// mode it had before, no matter what the call site now asks for. That makes
// tightening a permission literal in place silently do nothing for every
// install that already ran once, which is the one case the tightening
// exists for. EnsureDir and WriteFile close that gap by narrowing the
// path's mode toward the target after the create-or-reuse call succeeds, so
// the mode is correct on the next write regardless of the path's history.
//
// Narrowing only, never widening, is load-bearing, not a style choice: a
// path a caller deliberately made less permissive than the target — for
// whatever reason of their own — must stay at least that restrictive.
// Overwriting its mode outright would silently grant back access the
// caller removed on purpose, turning a permission error into success
// underneath them. Intersecting the current mode with the target can only
// remove bits, so a path already missing the owner-write bit, say, stays
// missing it, and the write this package wraps still fails naturally on
// it — exactly as it would have without this package in the way.
package secfile

import "os"

// EnsureDir makes dir (and any missing parents) if needed, then narrows
// dir's mode toward mode — including when dir already existed looser.
func EnsureDir(dir string, mode os.FileMode) error {
	if err := os.MkdirAll(dir, mode); err != nil {
		return err
	}
	return Narrow(dir, mode)
}

// WriteFile writes data to path, then narrows path's mode toward mode —
// including when path already existed looser.
func WriteFile(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return Narrow(path, mode)
}

// Narrow sets path's mode to the intersection of its current mode and
// want, so the result is never more permissive than either one — a path
// already at want (the common case: it was just created at that mode, or a
// prior narrow already landed) costs one extra Stat and no Chmod. Exported
// for a caller that opens a file itself (os.OpenFile with O_APPEND, say,
// where WriteFile's whole-file-replace shape doesn't fit) but still wants
// the same never-widen guarantee applied to what it opened.
func Narrow(path string, want os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	got := info.Mode().Perm()
	if got&^want == 0 {
		return nil
	}
	return os.Chmod(path, got&want)
}
