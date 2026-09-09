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

// Mode narrows an ask to the creation of a file, to changes of an existing
// one, or to a Read of one. Mint-only is the single most valuable narrowing
// found so far: one ask fired on every file in its corpus gated on content
// alone, and on 2.8% of them once it also required the target not to exist
// yet.
//
// Any/Mint/Edit all mean "a pending Write or Edit" — Read means the
// opposite, a Read tool call, never a pending write. No ask matches both
// kinds: Match rejects a Read-shaped call for any of the first three, and
// rejects a Write/Edit-shaped call for Read.
type Mode string

const (
	ModeAny  Mode = "any"
	ModeMint Mode = "mint"
	ModeEdit Mode = "edit"
	ModeRead Mode = "read"
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
	Evokes    []string // fuzzy trigger phrases; fires on any one, not all
	Revisit   bool     // retired: every matched ask always widens now; kept so old blocks still parse
	Always    bool     // skip the session key entirely — fires every match, no memory
	Block     bool     // added:/removed: only — deny the edit instead of only informing about it
	Name      string   // stable handle other asks can cue by; not part of ID()
	Cues      []string // names of other asks to fire alongside this one
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
	// IsRead reports whether this call is a Read, never a pending write. New
	// is always "" on a real Read — there is no incoming content — and
	// callers outside onsetter's own hook are free to leave this false,
	// which is its zero value and matches every existing caller's behavior.
	IsRead bool
	// Evokes answers whether the edit's content evokes a phrase from an ask's
	// evokes: list. Nil means no fuzzy stage ran for this call — every
	// evokes:-gated ask rejects rather than blocking on it, the same fail-soft
	// shape requires: uses for a missing binary. Match never calls out to an
	// embedding model or a cache itself; that machinery, and the threshold
	// decision, live entirely on the caller's side of this function value.
	Evokes func(phrase string) bool
}

// ID is stable across edits elsewhere in the file and changes when the ask
// itself changes. Line numbers are not usable as an identity: inserting a
// paragraph above an ask would make every ask below it fire again.
//
// Name is deliberately left out, for the same reason Requires already is:
// it is how other asks address this one, not part of the question being
// asked, so renaming an ask (to fix a collision, say) should not re-arm
// every already-answered session instance of it. Cues is included: adding a
// cues: line to an ask that already fired this session, with an unchanged
// quote, has to re-arm it — otherwise a freshly wired cue never gets a
// chance to walk during that session. Always is included: adding always:
// true to an ask mid-session should take effect on its very next match, not
// wait for a new session id.
//
// Revisit is left out — a change from every prior release. It used to widen
// the session key on its own; now every matched ask always fires and counts
// occurrences (session.Key does the widening unconditionally), so Revisit
// no longer changes what Match or the hook do with an ask at all. It is
// still parsed and stored on Ask, purely so an existing revisit: true block
// keeps parsing instead of erroring; onsetter lint flags it as redundant.
//
// Block is included the same way Always is: it changes what response a
// matched ask produces, and an author flipping it on mid-session should not
// have to wait for a fresh session id before the new behavior takes effect.
func (r *Ask) ID() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%t\x00%t",
		r.In, strings.Join(r.NotIn, "\x01"), reSrc(r.When), reSrc(r.Added),
		reSrc(r.Removed)+"\x02"+reSrc(r.Has)+"\x03"+strings.Join(r.Untouched, "\x01"),
		reSrc(r.Not), r.On, strings.Join(r.Evokes, "\x01"), r.Body,
		strings.Join(r.Cues, "\x01"), r.Always, r.Block)
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// Gated reports whether the ask narrows on content at all. An ask with no
// content gate and no path narrowing is a banner.
func (r *Ask) Gated() bool {
	return len(r.When)+len(r.Added)+len(r.Removed)+len(r.Has)+len(r.Untouched)+len(r.Evokes) > 0
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
//
// `revisit`, `always`, `block`, `name` and `cues` are last and out of step
// with that ordering on purpose: Match never looks at any of the five.
// `revisit`, `always` and `block` are metadata the hook dispatcher reads
// afterward — the first two to decide what (if anything) the session key for
// a firing includes, `block` to decide whether a matched firing also denies
// the edit. `name` is only ever read by another ask's `cues:`, and `cues:`
// itself is walked by Cascade, not by Match — none of the five is a gate a
// pending edit can pass or fail on its own.
var Headers = []string{"requires", "in", "not-in", "on", "not", "has", "untouched", "added", "removed", "when", "evokes", "revisit", "always", "block", "name", "cues"}

// Result is the outcome of matching one ask against one edit. When it fired,
// Matched is the text the content gate hit, so the injection can quote it
// back: a question about a string the author can see costs one sentence to
// dismiss, and a question about nothing in particular costs an investigation.
//
// When it did not fire, Gate names the header that turned it away. There are
// a dozen headers now, and "would not fire" said nothing about which one, so
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

// missingRequires returns the first requires: binary that does not resolve
// on $PATH, and whether every one of them did. Shared by Match, which needs
// to name the culprit in its Result, and Cascade, which only needs to know
// whether a cued candidate is meaningful to fire on this machine at all —
// a cued ask still shouldn't inject if it names a tool that isn't installed,
// even though every other gate is bypassed for it.
func (r *Ask) missingRequires() (string, bool) {
	for _, bin := range r.Requires {
		if _, err := exec.LookPath(bin); err != nil {
			return bin, false
		}
	}
	return "", true
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
	// misleading glob or regex mismatch. LookPath only stats a path — nothing
	// here is ever executed.
	//
	// This means a `requires:`-carrying ask pays a LookPath on every edit in
	// the session, not just edits its `in:` would otherwise have reached —
	// unlike every other gate, this one isn't scoped by path first. Left this
	// way on purpose: LookPath is a handful of stats, and this hook's own cost
	// is dominated by process spawn (see "No cache" in README's Design
	// section), so the ordering was chosen for a legible rejection reason, not
	// against a cost that was never the bottleneck.
	if missing, ok := r.missingRequires(); !ok {
		return no("requires", missing, "not found on $PATH")
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
		if e.IsRead {
			return no("on", "mint", "this call is a Read, not a Write or Edit")
		}
		if exists {
			return no("on", "mint", "the file already exists")
		}
	case ModeEdit:
		if e.IsRead {
			return no("on", "edit", "this call is a Read, not a Write or Edit")
		}
		if !exists {
			return no("on", "edit", "the file does not exist yet")
		}
	case ModeAny:
		if e.IsRead {
			return no("on", "any", "this call is a Read; only on: read asks match one")
		}
	case ModeRead:
		if !e.IsRead {
			return no("on", "read", "this call is a Write or Edit, not a Read")
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
		if len(e.Old)+len(e.New) > maxDiffBytes {
			// Myers diff cost isn't bounded by this package — an edit whose
			// old+new text this large would rather this ask stay silent than
			// gate the hook's own "fail open" invariant on a diff nobody
			// asked for. Measured before this cap existed: 4,000 old+new
			// lines cost 1.1GB RSS; this cap sits an order of magnitude
			// below that, comfortably clear of it. Degrades the same way
			// evokes: does when Ollama is unreachable — "this ask does not
			// fire," never an error.
			gate := "added"
			if len(r.Added) == 0 {
				gate = "removed"
			}
			return no(gate, "", "edit too large to diff safely")
		}
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
	// `evokes:` is fuzzy and every other gate here is exact, so it runs last —
	// only an edit every crisp glob and regex already let through pays for it.
	// It is an OR across the list (fires if the edit evokes any one phrase),
	// the opposite of when:'s AND, because these are independent conceptual
	// cues rather than conditions that must all hold at once. A nil e.Evokes
	// means no fuzzy stage ran for this call at all — every phrase rejects,
	// the same shape requires: uses when the binary is missing.
	if len(r.Evokes) > 0 {
		hit := ""
		for _, phrase := range r.Evokes {
			if e.Evokes != nil && e.Evokes(phrase) {
				hit = phrase
				break
			}
		}
		if hit == "" {
			return no("evokes", r.Evokes[0], "the edit does not evoke any of these")
		}
		if matched == "" {
			matched = hit
		}
	}
	return Result{OK: true, Matched: clip(matched)}
}

// ByName indexes asks by their name: header, skipping any that left it
// blank. A cues: value can only ever resolve within whatever slice the
// caller passes in — the hook's own per-edit discover.Asks(path) for a real
// firing, every ask in the repo for lint and status — so scope is entirely
// a property of what's fed in here, not of anything ByName does itself.
func ByName(asks []*Ask) map[string]*Ask {
	m := make(map[string]*Ask, len(asks))
	for _, a := range asks {
		if a.Name != "" {
			m[a.Name] = a
		}
	}
	return m
}

// CueProblem is one thing ValidateCues found wrong with a name: or cues:
// declaration.
type CueProblem struct {
	Ask   *Ask   // the ask carrying the problem
	Kind  string // "duplicate-name", "dangling-cue", or "cue-out-of-scope"
	Value string // the name: or cues: value in question
}

// ValidateCues checks every name: and cues: declaration in asks against
// each other. Two ways a author gets this wrong that Cascade itself has no
// way to notice, because it just silently finds nothing to fire either way:
//
// duplicate-name: two asks declaring the same name: — the second one wins
// silently in ByName, so whichever cue meant the first one now reaches the
// second instead.
//
// dangling-cue: a cues: value matching no name: anywhere in asks.
//
// cue-out-of-scope: a cues: value that does resolve, but to an ask whose
// Dir is not an ancestor of (or equal to) the citing ask's own Dir. A cue
// can only ever be reached at hook time from within the citing ask's own
// CLAUDE.md ancestor chain (discover.Asks walks strictly upward), so a
// target declared in a directory nested below the citer resolves here —
// asks is the whole repo — while silently never resolving for most of the
// files the citer's own in: actually reaches. This is a heuristic, not a
// proof: it flags the shape of the mistake (target nested below citer) by
// comparing directories, not by simulating every path in: could match.
func ValidateCues(asks []*Ask) []CueProblem {
	var problems []CueProblem

	seen := map[string]*Ask{}
	for _, a := range asks {
		if a.Name == "" {
			continue
		}
		if _, dup := seen[a.Name]; dup {
			problems = append(problems, CueProblem{Ask: a, Kind: "duplicate-name", Value: a.Name})
		}
		seen[a.Name] = a
	}

	byName := ByName(asks)
	for _, a := range asks {
		for _, cue := range a.Cues {
			target, ok := byName[cue]
			if !ok {
				problems = append(problems, CueProblem{Ask: a, Kind: "dangling-cue", Value: cue})
				continue
			}
			rel, err := filepath.Rel(target.Dir, a.Dir)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				problems = append(problems, CueProblem{Ask: a, Kind: "cue-out-of-scope", Value: cue})
			}
		}
	}
	return problems
}

// CascadeHit is one ask reached for a single edit, either because its own
// gate matched (Matched holds the quoted text, Via is nil) or because a
// matched ask's cues: named it (Matched is always empty, Via names the
// citer).
type CascadeHit struct {
	Ask     *Ask
	Matched string
	Via     *Ask
}

// Cascade walks cues: breadth-first from an already-computed set of direct
// matches, resolving names against asks — the same slice Match was already
// run over, so a cue can only ever reach an ask that could have been
// discovered for this edit in the first place. Bounded by len(asks): a
// visited-set keyed by Ask.ID() means every ask is walked at most once per
// call, which is what makes a genuine cycle (A cues B, B cues A) and a
// diamond (A and B both cue C) both safe without a separate depth counter.
//
// requires: is the one gate a cued ask still has to clear — a fact about
// the machine, not about this edit, so a cued ask naming an uninstalled
// tool still shouldn't inject. Every other gate is bypassed on purpose:
// that is the entire point of being reached by name instead of by match.
//
// Pure and session-unaware. hook.go layers store.Fired/store.Record
// filtering on top of this result the same way it already does for direct
// hits; list and replay, which have no session concept at all, just render
// it directly.
func Cascade(asks []*Ask, direct []CascadeHit) []CascadeHit {
	byName := ByName(asks)
	visited := make(map[string]bool, len(direct))
	out := make([]CascadeHit, len(direct))
	copy(out, direct)
	for _, h := range direct {
		visited[h.Ask.ID()] = true
	}

	queue := make([]CascadeHit, len(direct))
	copy(queue, direct)
	for len(queue) > 0 {
		citer := queue[0]
		queue = queue[1:]
		for _, cue := range citer.Ask.Cues {
			target, ok := byName[cue]
			if !ok || visited[target.ID()] {
				continue
			}
			visited[target.ID()] = true
			if _, ok := target.missingRequires(); !ok {
				continue
			}
			hit := CascadeHit{Ask: target, Via: citer.Ask}
			out = append(out, hit)
			queue = append(queue, hit)
		}
	}
	return out
}

// multiline prepends (?m) to an added:/removed: pattern, so `^` and `$`
// anchor to each line in the joined added/deleted text diffLines produces,
// not to the start and end of the whole block. Without this, an author
// writing `added: ^\s*[-*]` gets a pattern that only ever matches when the
// intended line happens to be first in the diff — reported live: one ask
// fired only when its match landed on the first added line, a sibling using
// `removed: ^func (Check|Law|Prop|Test)` never fired at all, since a removed
// Go function is essentially never the first deleted line of a hunk.
// Redundant if the author already wrote (?m) themselves — Go's regexp
// accepts a repeated flag group as a no-op, confirmed, not an error.
func multiline(pattern string) string { return "(?m)" + pattern }

// maxDiffBytes bounds len(old)+len(new) before diffLines runs. Measured
// without a cap: 4,000 combined old+new lines cost 1.1GB RSS; 20,000 lines
// got OOM-killed at 8.7GB before its own 120s timeout fired. This cap sits
// an order of magnitude under the first measured danger point, at a size no
// legitimate single edit approaches.
const maxDiffBytes = 100_000

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
			re, err := regexp.Compile(multiline(v))
			if err != nil {
				return nil, fmt.Errorf("added: %w", err)
			}
			r.Added = append(r.Added, re)
		case "removed":
			re, err := regexp.Compile(multiline(v))
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
			case ModeAny, ModeMint, ModeEdit, ModeRead:
				r.On = Mode(strings.ToLower(v))
			default:
				return nil, fmt.Errorf("on: %q is not one of any, mint, edit, read", v)
			}
		case "requires":
			r.Requires = append(r.Requires, v) // repeated requires: is an AND
		case "evokes":
			r.Evokes = append(r.Evokes, v) // repeated evokes: is an OR; not a regex
		case "revisit":
			if strings.ToLower(v) != "true" {
				return nil, fmt.Errorf("revisit: %q is not \"true\" (omit the header for the default)", v)
			}
			r.Revisit = true
		case "always":
			if strings.ToLower(v) != "true" {
				return nil, fmt.Errorf("always: %q is not \"true\" (omit the header for the default)", v)
			}
			r.Always = true
		case "block":
			if strings.ToLower(v) != "true" {
				return nil, fmt.Errorf("block: %q is not \"true\" (omit the header for the default)", v)
			}
			r.Block = true
		case "name":
			r.Name = v
		case "cues":
			r.Cues = append(r.Cues, v) // repeated cues: cues every one, not an AND/OR — it isn't a gate
		default:
			return nil, fmt.Errorf("unknown header %q (want %s)%s", k, strings.Join(Headers, ", "), hint)
		}
	}

	r.Body = strings.TrimSpace(strings.Join(lines[i:], "\n"))
	if r.Body == "" {
		return nil, fmt.Errorf("block has no prose after the headers — an ask with nothing to say cannot help")
	}
	// block: true denies the write in progress, so it has to point at
	// something concrete in that same write — added: or removed:, the two
	// gates that see the edit's own content rather than guessing from a
	// glob or a reminder. A when:-only or has:-only ask has nothing to
	// quote back as the reason for a denial.
	if r.Block && len(r.Added)+len(r.Removed) == 0 {
		return nil, fmt.Errorf("block: true needs an added: or removed: header — a when:-only ask has nothing concrete in hand to justify denying the write")
	}
	return r, nil
}
