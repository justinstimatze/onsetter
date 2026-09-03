package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/justinstimatze/onsetter/internal/session"
	"mvdan.cc/sh/v3/syntax"
)

// recordBashTouches marks every path a Bash call looks like it wrote to as
// touched, for untouched:'s benefit — no ask-matching, no discover.Asks, no
// injection. It is why this branch is cheap enough to wire by default: the
// cost is one shell parse of typically-short command text, not a CLAUDE.md
// tree walk.
func recordBashTouches(p payload) {
	if p.ToolInput.Command == "" {
		return
	}
	store := session.Open(p.SessionID)
	cwd := p.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	for _, rel := range extractWrittenPaths(p.ToolInput.Command) {
		abs := rel
		if !filepath.IsAbs(abs) && cwd != "" {
			abs = filepath.Join(cwd, abs)
		}
		store.Touch(abs)
	}
}

// extractWrittenPaths parses a shell command line with a real POSIX parser
// and returns every path it looks like the command writes to: an output
// redirect (>, >>), tee's target(s), an in-place sed (-i) or sd's rewritten
// file, or cp/mv's destination. Best-effort and conservative on purpose: a
// command this can't parse, or a shape not in that list, contributes
// nothing — the same blind spot `untouched:` already has today for anything
// written by Bash. A path built from a variable or command substitution
// also contributes nothing rather than a guess, since a wrong guess here
// would wrongly suppress a real untouched: ask, which is worse than the
// miss it would be replacing.
//
// A real parser instead of regex, specifically so that a fd-redirect
// (2>&1, >&2) is excluded by its different AST shape rather than by
// pattern-matching digits, and so a word with an expansion in it —
// Word.Lit() returns "" the moment one is present — is recognized as
// unresolvable instead of silently matched as literal text.
func extractWrittenPaths(command string) []string {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return nil
	}
	var out []string
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.Redirect:
			switch n.Op {
			case syntax.RdrOut, syntax.AppOut, syntax.RdrAll, syntax.AppAll:
				if lit, ok := wordLit(n.Word); ok && lit != "" {
					out = append(out, lit)
				}
			}
		case *syntax.CallExpr:
			out = append(out, writtenByCall(n)...)
		}
		return true
	})
	return out
}

// wordLit resolves a word to its literal text when every part of it is
// determined at parse time: a bare literal, a single-quoted string (single
// quotes suppress all expansion), or a double-quoted string whose own parts
// are themselves plain literals. (*syntax.Word).Lit() only handles the
// first shape — this extends it to the other two, which are just as fully
// known and at least as common in a real command line (`sed -i 's/x/y/'
// file` quotes its pattern). Anything else — a parameter expansion, a
// command substitution, an arithmetic expression, process substitution, an
// extended glob, anywhere in the word — makes the whole word unresolvable,
// the same conservative call Lit() already makes: never guess at what an
// expansion might produce. The bool return distinguishes a genuinely empty
// literal ("") from unresolvable, which a bare string return cannot.
func wordLit(w *syntax.Word) (string, bool) {
	if w == nil {
		return "", false
	}
	var b strings.Builder
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			b.WriteString(p.Value)
		case *syntax.SglQuoted:
			b.WriteString(p.Value)
		case *syntax.DblQuoted:
			for _, inner := range p.Parts {
				lit, ok := inner.(*syntax.Lit)
				if !ok {
					return "", false
				}
				b.WriteString(lit.Value)
			}
		default:
			return "", false
		}
	}
	return b.String(), true
}

// writtenByCall applies per-command positional logic to a call's literal
// arguments.
//
// tee's targets are order-independent, so an unresolved or flag-shaped
// argument is simply skipped — the same reasoning `cp`/`mv`'s destination
// gets below.
//
// cp/mv's destination is always the last argument by raw position,
// regardless of how many earlier arguments there are or whether they
// resolve — an unresolved source (`cp "$SRC" dest.txt`, the common shape)
// still leaves the destination's own position well-defined, so only that
// one argument's literal has to resolve.
//
// sed and sd are different: knowing *which* trailing arguments are files
// depends on first knowing how many leading arguments are flags versus the
// expression/pattern/replacement, which is a fact about *all* of the
// earlier arguments together, not about one fixed position. For those, an
// unresolved argument anywhere in that count aborts the whole call rather
// than guessing where the boundary actually falls.
func writtenByCall(n *syntax.CallExpr) []string {
	if len(n.Args) < 2 {
		return nil
	}
	name, ok := wordLit(n.Args[0])
	if !ok {
		return nil
	}
	rest := n.Args[1:]

	switch name {
	case "tee":
		var out []string
		for _, a := range rest {
			lit, ok := wordLit(a)
			if !ok || lit == "-a" || strings.HasPrefix(lit, "-") {
				continue
			}
			out = append(out, lit)
		}
		return out
	case "cp", "mv":
		lit, ok := wordLit(rest[len(rest)-1])
		if !ok || strings.HasPrefix(lit, "-") {
			return nil
		}
		return []string{lit}
	case "sed":
		args, ok := literalArgs(rest)
		if !ok {
			return nil
		}
		inPlace := false
		var nonFlags []string
		for _, a := range args {
			switch {
			case a == "-i" || strings.HasPrefix(a, "-i"):
				inPlace = true
			case strings.HasPrefix(a, "-"):
			default:
				nonFlags = append(nonFlags, a)
			}
		}
		// nonFlags[0] is the expression; the rest are the files sed rewrites.
		if !inPlace || len(nonFlags) < 2 {
			return nil
		}
		return nonFlags[1:]
	case "sd":
		args, ok := literalArgs(rest)
		if !ok {
			return nil
		}
		var positional []string
		for _, a := range args {
			if !strings.HasPrefix(a, "-") {
				positional = append(positional, a)
			}
		}
		// sd rewrites in place by default: PATTERN REPLACEMENT FILE.
		if len(positional) != 3 {
			return nil
		}
		return []string{positional[2]}
	}
	return nil
}

// literalArgs resolves every word to its literal text, or reports false the
// moment one doesn't resolve — never a partial result with a gap in it. sed
// and sd both need this: knowing where the flag/expression/files boundary
// falls depends on every earlier argument resolving, not just one position.
func literalArgs(words []*syntax.Word) ([]string, bool) {
	out := make([]string, len(words))
	for i, w := range words {
		lit, ok := wordLit(w)
		if !ok {
			return nil, false
		}
		out[i] = lit
	}
	return out, true
}
