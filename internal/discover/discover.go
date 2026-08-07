// Package discover finds the CLAUDE.md files whose asks could govern a path.
package discover

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/justinstimatze/onsetter/internal/ask"
)

// Names are the files searched in each directory, in the order Claude Code
// itself reads them. `.claude/CLAUDE.md` is here because Claude Code loads it
// as the directory's file, and because some repos deliberately keep their AI
// conventions there — a repo that gitignores `.claude/*` keeps its guidance
// out of the tree it publishes. Asks from it are scoped to the parent directory,
// not to `.claude/`, or every glob in the file would match nothing.
var Names = []string{"CLAUDE.md", "CLAUDE.local.md", ".claude/CLAUDE.md"}

// Roots returns the CLAUDE.md files governing path, outermost first. It walks
// up from the file's directory and stops after the directory holding .git, or
// at $HOME, whichever comes first.
//
// Stopping at the repo root is what makes `in:` default to "everything below
// this file" instead of "everything on the disk". An ask's blast radius is the
// directory it lives in, which is the only scope an author can reason about
// without holding the whole tree in their head.
func Roots(path string) []string {
	dir := filepath.Dir(path)
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	home, _ := os.UserHomeDir()

	var found []string
	for {
		for _, n := range Names {
			p := filepath.Join(dir, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				found = append(found, p)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		if dir == home || dir == "/" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Outermost first, so a repo-wide ask is read before a directory-local one.
	for i, j := 0, len(found)-1; i < j; i, j = i+1, j-1 {
		found[i], found[j] = found[j], found[i]
	}
	return found
}

// Asks collects every ask governing path. Parse errors are returned
// alongside whatever parsed, never instead of it: one bad block must not take
// the rest of the file's asks down with it.
func Asks(path string) ([]*ask.Ask, []error) {
	var out []*ask.Ask
	var errs []error
	for _, src := range Roots(path) {
		rs, err := ParseSource(src)
		if err != nil {
			errs = append(errs, err)
		}
		out = append(out, rs...)
	}
	return out, errs
}

// ParseSource parses one CLAUDE.md with its globs scoped correctly. Every
// caller goes through here, so the `.claude/` exception is known in one place
// rather than remembered in three.
func ParseSource(src string) ([]*ask.Ask, error) {
	return ask.ParseFileScoped(src, scopeOf(src))
}

// scopeOf returns the directory a file's `in:` globs are relative to: the
// project directory, which for `.claude/CLAUDE.md` is one level up. Empty
// means "the file's own directory".
func scopeOf(src string) string {
	dir := filepath.Dir(src)
	if filepath.Base(dir) == ".claude" {
		return filepath.Dir(dir)
	}
	return ""
}

// Sources lists every CLAUDE.md under root that contains at least one ask, or
// that tried to and failed. Used by lint.
//
// A file whose only block is malformed has to be in this list. Dropping it —
// which this did — made `onsetter lint` print "0 ask(s) in 0 file(s)" and exit
// 0 over an unclosed regex, which is the one failure lint exists to catch: the
// author believes the block is watching and nothing says otherwise.
func Sources(root string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree is not a lint failure
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", ".venv", "target":
				return filepath.SkipDir
			}
			return nil
		}
		// Match on the base name, not on Names: `.claude/CLAUDE.md` is one
		// entry there but arrives here as a plain `CLAUDE.md` inside a
		// `.claude` directory, and it needs scopeOf to place its globs.
		switch d.Name() {
		case "CLAUDE.md", "CLAUDE.local.md":
			if rs, perr := ParseSource(p); len(rs) > 0 || perr != nil {
				out = append(out, p)
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}
