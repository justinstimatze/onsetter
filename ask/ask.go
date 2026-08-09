// Package ask parses asks out of CLAUDE.md files and decides whether one
// applies to a pending file edit.
//
// An ask is a short epistemic sniff check that lives in the CLAUDE.md sitting
// beside the files it is about. It is a fenced block in an ordinary CLAUDE.md,
// so the prose is still there for anyone reading the file top to bottom; the
// fence only marks which paragraph has a gate on it.
//
//	```ask
//	in: corpus/{locations,chars}/**
//	when: you (nod|realize|decide|turn away)
//
//	The narrator describes the world; the player decides what they do and what
//	it means. Rewrite to observable world-state — unless the match is genuinely
//	sensory, in which case continue.
//	```
//
// Headers first, one blank line, then the prose, all inside the fence. Putting
// the body inside means the block has exactly one delimiter and no heuristic
// about where the prose stops.
package ask

import (
	"bufio"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
)

// Mode narrows an ask to the creation of a file or to changes of an existing
// one. Mint-only is the single most valuable narrowing found so far: one ask
// fired on every file in its corpus gated on content alone, and on 2.8% of
// them once it also required the target not to exist yet.
type Mode string

const (
	ModeAny  Mode = "any"
	ModeMint Mode = "mint"
	ModeEdit Mode = "edit"
)

// Ask is one ```ask block, parsed.
type Ask struct {
	Source    string   // absolute path of the CLAUDE.md it came from
	Line      int      // 1-based line of the opening fence
	Dir       string   // directory of Source; In is relative to this
	In        string   // glob against the path, relative to Dir
	NotIn     []string // globs that exclude a path `in` would have matched
	When      []*regexp.Regexp
	Added     []*regexp.Regexp // more occurrences after the edit than before
	Removed   []*regexp.Regexp // fewer occurrences after the edit than before
	Has       []*regexp.Regexp // matches the file already on disk, not the edit
	Untouched []string         // globs no file written this session may match
	Not       []*regexp.Regexp
	On        Mode
	Requires  []string // binary names that must resolve on $PATH
	Body      string
}

// Edit is the pending write an ask is matched against. Old is empty for a
// Write and for the synthetic edits `list` and `replay` construct from files
// on disk, which is why `added:` degrades to `when:` and `removed:` cannot
// fire in either of those.
type Edit struct {
	Path string
	New  string // `content` on a Write, `new_string` on an Edit
	Old  string // `old_string` on an Edit
	Disk string // the file as it stands, for `has:`; read only if one asks
	// Touched is every path written this session, for `untouched:`. Empty in
	// `list` and `replay`, which have no session to speak of.
	Touched []string
	Exists  bool
}

// ID is stable across edits elsewhere in the file and changes when the ask
// itself changes. Line numbers are not usable as an identity: inserting a
// paragraph above an ask would make every ask below it fire again.
func (r *Ask) ID() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s",
		r.In, strings.Join(r.NotIn, "\x01"), reSrc(r.When), reSrc(r.Added),
		reSrc(r.Removed)+"\x02"+reSrc(r.Has)+"\x03"+strings.Join(r.Untouched, "\x01"),
		reSrc(r.Not), r.On, r.Body)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Gated reports whether the ask narrows on content at all. An ask with no
// content gate and no path narrowing is a banner.
func (r *Ask) Gated() bool {
	return len(r.When)+len(r.Added)+len(r.Removed)+len(r.Has)+len(r.Untouched) > 0
}

func reSrc(res []*regexp.Regexp) string {
	parts := make([]string, len(res))
	for i, re := range res {
		parts[i] = re.String()
	}
	return strings.Join(parts, "\x01")
}

// Where renders the ask's origin for a human, e.g. "corpus/CLAUDE.md:12".
// Relative to base when the source is underneath it, absolute otherwise.
func (r *Ask) Where(base string) string {
	p := r.Source
	if rel, err := filepath.Rel(base, r.Source); err == nil && !strings.HasPrefix(rel, "..") {
		p = rel
	}
	return fmt.Sprintf("%s:%d", p, r.Line)
}

//go:embed headers.md
var reference string

// Reference is the header documentation, printed by `onsetter headers`. It is
// written for whoever is drafting an ask, which in practice is an agent that
// will read it once mid-task and then write the block — so it ships with the
// binary rather than living at a path someone has to already know.
func Reference() string { return reference }

//go:embed skill.md
var skill string

// Skill is the Claude Code skill `onsetter install` writes to
// ~/.claude/skills/onsetter/SKILL.md. It is deliberately thin and defers the
// header table to Reference: two copies of the same reference would drift, and
// the copy inside the binary is the one that cannot disagree with the parser
// shipped beside it.
func Skill() string { return skill }

// Headers are the header keys a block may carry, in the order Match applies
// them. One list, so the parse error, the reference in `onsetter headers` and
// the funnel in `onsetter replay` cannot disagree about what exists.
var Headers = []string{"requires", "in", "not-in", "on", "not", "has", "untouched", "added", "removed", "when"}

// Result is the outcome of matching one ask against one edit. When it fired,
// Matched is the text the content gate hit, so the injection can quote it
// back: a question about a string the author can see costs one sentence to
// dismiss, and a question about nothing in particular costs an investigation.
//
// When it did not fire, Gate names the header that turned it away. There are
// ten headers now, and "would not fire" said nothing about which one, so
// debugging a draft ask meant deleting headers one at a time and rebuilding.
type Result struct {
	OK      bool
	Matched string // the text a content gate matched; empty when nothing gates on content
	Gate    string // the header that rejected, e.g. "when"; empty when OK
	Pattern string // that header's value, e.g. `\bTODO\b`
	Note    string // what the gate saw instead
}

// Why renders a rejection as one clause, e.g.
// "when: \bTODO\b — nothing like it in the incoming text". Empty when it fired.
func (r Result) Why() string {
	switch {
	case r.OK:
		return ""
	case r.Gate == "":
		return "no gate rejected it"
	case r.Pattern == "":
		return fmt.Sprintf("%s — %s", r.Gate, r.Note)
	default:
		return fmt.Sprintf("%s: %s — %s", r.Gate, r.Pattern, r.Note)
	}
}

func no(gate, pattern, note string) Result {
	return Result{Gate: gate, Pattern: pattern, Note: note}
}

// Match reports whether the ask applies to a pending write of content to path,
// and either what its content gate matched or which gate turned it away.
//
// path may be empty — a caller outside onsetter's own Write/Edit hook (an MCP
// tool call with no file path, e.g.) can match content against when:/not:/
// has: alone. An ask that also sets in:/not-in:/untouched: cannot honor those
// without a path, so it rejects with a Result naming that instead of running
// the path-matching code below on an empty string and failing for the wrong
// reason.
func (r *Ask) Match(e Edit) Result {
	path, content, exists := e.Path, e.New, e.Exists
	// `requires:` is a fact about the machine, not the file or the edit, so it
	// runs before anything path- or content-based: a rejection on a machine
	// without the tool should read "turned away at requires:", not a
	// misleading glob or regex mismatch. LookPath only stats $PATH — nothing
	// here is ever executed.
	for _, bin := range r.Requires {
		if _, err := exec.LookPath(bin); err != nil {
			return no("requires", bin, "not found on $PATH")
		}
	}
	if path == "" {
		switch {
		case r.In != "**":
			return no("in", r.In, "this ask matches in: against a path, and this call has none")
		case len(r.NotIn) > 0:
			return no("not-in", r.NotIn[0], "this ask matches not-in: against a path, and this call has none")
		case len(r.Untouched) > 0:
			return no("untouched", r.Untouched[0], "this ask matches untouched: against a path, and this call has none")
		}
	} else {
		rel, err := filepath.Rel(r.Dir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return no("in", r.In, "the file is not under this CLAUDE.md's directory")
		}
		slash := filepath.ToSlash(rel)
		if ok, err := doublestar.Match(r.In, slash); err != nil || !ok {
			return no("in", r.In, "does not match "+slash)
		}
		// `not:` is a content regex, so it cannot exclude a path. Without a
		// separate key, `not: _test\.go` silently matches nothing and an ask scoped
		// to a package fires on its tests too — which is how a migrated ask came
		// to fire on nearly twice the files the script it replaced did.
		for _, ex := range r.NotIn {
			if ok, err := doublestar.Match(ex, slash); err == nil && ok {
				return no("not-in", ex, "matches "+slash)
			}
		}
	}
	switch r.On {
	case ModeMint:
		if exists {
			return no("on", "mint", "the file already exists")
		}
	case ModeEdit:
		if !exists {
			return no("on", "edit", "the file does not exist yet")
		}
	}
	for _, not := range r.Not {
		if m := not.FindString(content); m != "" {
			return no("not", not.String(), fmt.Sprintf("matched %q, which suppresses the ask", clip(m)))
		}
	}
	// `has:` looks at the file as it stands rather than at the edit, which is
	// what expresses "this file already does X and you are adding a second
	// way to do it". No prior tool sees the pending write and the current
	// file at once, so this one has no name to borrow.
	for _, has := range r.Has {
		if !has.MatchString(e.Disk) {
			return no("has", has.String(), "nothing like it in the file as it stands")
		}
	}
	// `untouched:` is the paired-file gate — "you changed the schema and have
	// not been near a migration". Globs resolve against this CLAUDE.md's
	// directory, the same as `in:`.
	for _, g := range r.Untouched {
		for _, tp := range e.Touched {
			trel, err := filepath.Rel(r.Dir, tp)
			if err != nil {
				continue
			}
			tslash := filepath.ToSlash(trel)
			if ok, err := doublestar.Match(g, tslash); err == nil && ok {
				return no("untouched", g, tslash+" was written this session")
			}
		}
	}
	var matched string
	// `added:` and `removed:` match only the lines the edit introduces or
	// takes out, which is Danger's `.added` / `.deleted`. The line kinds come
	// from a real Myers diff rather than an occurrence count, so replacing a
	// span that already contained the pattern does not read as adding it.
	if len(r.Added) > 0 || len(r.Removed) > 0 {
		add, rem := diffLines(e.Old, e.New)
		for _, re := range r.Added {
			m := re.FindString(add)
			if m == "" {
				return no("added", re.String(), "nothing like it in the lines this edit adds")
			}
			if matched == "" {
				matched = m
			}
		}
		for _, re := range r.Removed {
			m := re.FindString(rem)
			if m == "" {
				return no("removed", re.String(), "nothing like it in the lines this edit removes")
			}
			if matched == "" {
				matched = m
			}
		}
	}
	// Every `when:` must match. Two conditions across the whole text is a real
	// shape — "touches a fact_text block AND contains inference language" — and
	// RE2 has no lookahead to express it in one pattern. Repeating the header
	// costs no new vocabulary and reads as the conjunction it is.
	for _, when := range r.When {
		m := when.FindString(content)
		if m == "" {
			return no("when", when.String(), "nothing like it in the incoming text")
		}
		if matched == "" {
			matched = m
		}
	}
	return Result{OK: true, Matched: clip(matched)}
}

// diffLines returns the inserted and deleted lines of old -> new, each joined
// back into one string so an ordinary regex can be run over them. A Write has
// no old text, so everything counts as added, which is what it is.
func diffLines(old, new string) (added, removed string) {
	if old == "" {
		return new, ""
	}
	u := gotextdiff.ToUnified("a", "b", old, myers.ComputeEdits(span.URIFromPath("a"), old, new))
	var a, d strings.Builder
	for _, h := range u.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case gotextdiff.Insert:
				a.WriteString(l.Content)
			case gotextdiff.Delete:
				d.WriteString(l.Content)
			}
		}
	}
	return a.String(), d.String()
}

func clip(s string) string {
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

// ParseFile reads every ask out of a CLAUDE.md, scoping `in:`
// globs to the file's own directory.
func ParseFile(path string) ([]*Ask, error) {
	return ParseFileScoped(path, "")
}

// ParseFileScoped is ParseFile with an explicit scope for `in:` globs. It
// exists for `<root>/.claude/CLAUDE.md`, which Claude Code loads as the
// project's file even though it sits one directory down: scoping its asks to
// `.claude/` would make every glob in it match nothing. An empty scope means
// the file's own directory.
func ParseFileScoped(path, scope string) ([]*Ask, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	asks, err := Parse(f, abs)
	if scope != "" {
		for _, r := range asks {
			r.Dir = scope
		}
	}
	return asks, err
}

var fenceOpen = regexp.MustCompile("^\\s*```+\\s*ask\\s*$")
var fenceClose = regexp.MustCompile("^\\s*```+\\s*$")

// Parse extracts asks from a CLAUDE.md's contents. A malformed
// block is an error naming its line, because an ask that silently fails to
// parse is worse than no ask: the author believes it is watching.
func Parse(r io.Reader, source string) ([]*Ask, error) {
	dir := filepath.Dir(source)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var out []*Ask
	var errs []string
	lineNo := 0
	for sc.Scan() {
		lineNo++
		if !fenceOpen.MatchString(sc.Text()) {
			continue
		}
		start := lineNo
		var block []string
		closed := false
		for sc.Scan() {
			lineNo++
			if fenceClose.MatchString(sc.Text()) {
				closed = true
				break
			}
			block = append(block, sc.Text())
		}
		if !closed {
			errs = append(errs, fmt.Sprintf("%s:%d: ```ask block is never closed", source, start))
			break
		}
		a, err := parseBlock(block, source, dir, start)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: %v", source, start, err))
			continue
		}
		out = append(out, a)
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	if len(errs) > 0 {
		return out, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return out, nil
}

func parseBlock(lines []string, source, dir string, start int) (*Ask, error) {
	r := &Ask{Source: source, Dir: dir, Line: start, In: "**", On: ModeAny}

	i := 0
	for ; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			break
		}
		// The first line is where a headerless block goes wrong: written the
		// obvious way, fence then prose, the prose is read as a header and the
		// block does not parse. The rule is one blank line before the body even
		// when there are no headers, and the error has to say so — otherwise
		// the author gets "not key: value" about a sentence.
		hint := ""
		if i == 0 {
			hint = " (a block with no headers must still start with a blank line)"
		}
		k, v, found := strings.Cut(line, ":")
		if !found {
			return nil, fmt.Errorf("header line %q is not `key: value`%s", line, hint)
		}
		k, v = strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if v == "" {
			return nil, fmt.Errorf("header %q has no value", k)
		}
		switch k {
		case "in":
			r.In = strings.TrimPrefix(filepath.ToSlash(v), "./")
		case "not-in":
			r.NotIn = append(r.NotIn, strings.TrimPrefix(filepath.ToSlash(v), "./"))
		case "when":
			re, err := regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("when: %w", err)
			}
			r.When = append(r.When, re) // repeated when: is an AND
		case "added":
			re, err := regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("added: %w", err)
			}
			r.Added = append(r.Added, re)
		case "removed":
			re, err := regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("removed: %w", err)
			}
			r.Removed = append(r.Removed, re)
		case "has":
			re, err := regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("has: %w", err)
			}
			r.Has = append(r.Has, re)
		case "untouched":
			r.Untouched = append(r.Untouched, strings.TrimPrefix(filepath.ToSlash(v), "./"))
		case "not":
			re, err := regexp.Compile(v)
			if err != nil {
				return nil, fmt.Errorf("not: %w", err)
			}
			r.Not = append(r.Not, re) // repeated not: is an OR of suppressors
		case "on":
			switch Mode(strings.ToLower(v)) {
			case ModeAny, ModeMint, ModeEdit:
				r.On = Mode(strings.ToLower(v))
			default:
				return nil, fmt.Errorf("on: %q is not one of any, mint, edit", v)
			}
		case "requires":
			r.Requires = append(r.Requires, v) // repeated requires: is an AND
		default:
			return nil, fmt.Errorf("unknown header %q (want %s)%s", k, strings.Join(Headers, ", "), hint)
		}
	}

	r.Body = strings.TrimSpace(strings.Join(lines[i:], "\n"))
	if r.Body == "" {
		return nil, fmt.Errorf("block has no prose after the headers — an ask with nothing to say cannot help")
	}
	return r, nil
}
