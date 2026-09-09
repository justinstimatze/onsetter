package ask

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// match is the two-value form most of these tests want. Match returns a
// Result so that a rejection can say which gate rejected; in a table test
// asserting only fired-or-not, the rest of the struct is noise.
func match(r *Ask, e Edit) (string, bool) {
	res := r.Match(e)
	return res.Matched, res.OK
}

func parseOne(t *testing.T, src string) *Ask {
	t.Helper()
	rs, err := Parse(strings.NewReader(src), "/repo/corpus/CLAUDE.md")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("got %d asks, want 1", len(rs))
	}
	return rs[0]
}

const basic = "prose above\n\n" +
	"```ask\n" +
	"in: chars/**\n" +
	"when: you (nod|realize)\n" +
	"\n" +
	"The narrator describes the world.\n" +
	"The player decides what it means.\n" +
	"```\n" +
	"prose below\n"

func TestParseHeadersAndBody(t *testing.T) {
	r := parseOne(t, basic)
	if r.In != "chars/**" {
		t.Errorf("In = %q", r.In)
	}
	if len(r.When) != 1 || r.When[0].String() != "you (nod|realize)" {
		t.Errorf("When = %v", r.When)
	}
	if r.On != ModeAny {
		t.Errorf("On = %q, want the default", r.On)
	}
	if r.Line != 3 {
		t.Errorf("Line = %d, want 3", r.Line)
	}
	want := "The narrator describes the world.\nThe player decides what it means."
	if r.Body != want {
		t.Errorf("Body = %q", r.Body)
	}
	if r.Dir != filepath.FromSlash("/repo/corpus") {
		t.Errorf("Dir = %q", r.Dir)
	}
}

func TestDefaultsToEverythingBelow(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	if r.In != "**" {
		t.Errorf("In = %q, want ** (everything below this CLAUDE.md)", r.In)
	}
}

// `ask` is the only fence keyword. A block opened with anything else is
// ordinary markdown and is skipped in silence, which is what makes the fence a
// deliberate opt-in rather than a heuristic over prose.
func TestOnlyAskOpensABlock(t *testing.T) {
	for _, kw := range []string{"miniprompt", "prompt", "asks", "Ask"} {
		rs, err := Parse(strings.NewReader("```"+kw+"\nwhen: x\n\nAsk.\n```\n"), "/repo/CLAUDE.md")
		if err != nil || len(rs) != 0 {
			t.Errorf("```%s opened a block: %d asks, err %v", kw, len(rs), err)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unknown header": "```ask\nwherever: x\n\nAsk.\n```\n",
		"bad regex":      "```ask\nwhen: (unclosed\n\nAsk.\n```\n",
		"bad mode":       "```ask\non: sometimes\n\nAsk.\n```\n",
		"no body":        "```ask\nwhen: x\n```\n",
		"empty value":    "```ask\nwhen:\n\nAsk.\n```\n",
		"not key value":  "```ask\njust some prose\n\nAsk.\n```\n",
		"never closed":   "```ask\nwhen: x\n\nAsk.\n",
	} {
		if _, err := Parse(strings.NewReader(src), "/repo/CLAUDE.md"); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

// One malformed block must not take the rest of the file's asks down with it.
func TestBadBlockDoesNotEatGoodOnes(t *testing.T) {
	src := "```ask\nnope: x\n\nAsk.\n```\n\n```ask\nwhen: ok\n\nAsk.\n```\n"
	rs, err := Parse(strings.NewReader(src), "/repo/CLAUDE.md")
	if err == nil {
		t.Fatal("want an error for the bad block")
	}
	if len(rs) != 1 {
		t.Fatalf("got %d surviving asks, want 1", len(rs))
	}
}

func TestMatchScope(t *testing.T) {
	r := parseOne(t, basic)
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/repo/corpus/chars/betty.md", true},
		{"/repo/corpus/chars/nested/deep.md", true},
		{"/repo/corpus/locations/diner.md", false}, // outside in:
		{"/repo/other/chars/betty.md", false},      // outside the CLAUDE.md's subtree
		{"/repo/corpus/CLAUDE.md", false},
	} {
		if _, ok := match(r, Edit{Path: filepath.FromSlash(tc.path), New: "you nod slowly", Disk: "you nod slowly", Exists: true}); ok != tc.want {
			t.Errorf("%s: ok = %v, want %v", tc.path, ok, tc.want)
		}
	}
}

func TestMatchQuotesBack(t *testing.T) {
	r := parseOne(t, basic)
	m, ok := match(r, Edit{Path: filepath.FromSlash("/repo/corpus/chars/betty.md"), New: "and you realize she is lying", Disk: "and you realize she is lying", Exists: true})
	if !ok {
		t.Fatal("want a fire")
	}
	if m != "you realize" {
		t.Errorf("matched = %q, want the text the gate hit", m)
	}
}

func TestNoContentGateFiresOnAnyContent(t *testing.T) {
	r := parseOne(t, "```ask\nin: gen/**\n\nGenerated. Do not hand-edit.\n```\n")
	m, ok := match(r, Edit{Path: filepath.FromSlash("/repo/corpus/gen/x.go"), New: "anything", Disk: "anything", Exists: true})
	if !ok || m != "" {
		t.Errorf("ok = %v, matched = %q", ok, m)
	}
}

func TestNotSuppresses(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nnot: TODO\\(alice\\)\n\nAsk.\n```\n")
	if _, ok := match(r, Edit{Path: filepath.FromSlash("/repo/corpus/a.md"), New: "TODO(alice) fix", Disk: "TODO(alice) fix", Exists: true}); ok {
		t.Error("not: should have suppressed this")
	}
	if _, ok := match(r, Edit{Path: filepath.FromSlash("/repo/corpus/a.md"), New: "TODO fix", Disk: "TODO fix", Exists: true}); !ok {
		t.Error("want a fire when not: does not match")
	}
}

func TestModes(t *testing.T) {
	mint := parseOne(t, "```ask\non: mint\n\nAsk.\n```\n")
	edit := parseOne(t, "```ask\non: edit\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.md")
	if _, ok := match(mint, Edit{Path: p, New: "x", Disk: "x", Exists: true}); ok {
		t.Error("mint fired on an existing file")
	}
	if _, ok := match(mint, Edit{Path: p, New: "x", Disk: "x", Exists: false}); !ok {
		t.Error("mint did not fire on a new file")
	}
	if _, ok := match(edit, Edit{Path: p, New: "x", Disk: "x", Exists: false}); ok {
		t.Error("edit fired on a new file")
	}
	if _, ok := match(edit, Edit{Path: p, New: "x", Disk: "x", Exists: true}); !ok {
		t.Error("edit did not fire on an existing file")
	}
}

// on: read is its own axis, not a fourth existence-state: any/mint/edit are
// all "a pending write," and read is the opposite, a Read call. No ask
// matches both kinds.
func TestOnReadMatchesOnlyAReadCall(t *testing.T) {
	read := parseOne(t, "```ask\non: read\n\nAsk.\n```\n")
	any := parseOne(t, "```ask\n\nAsk.\n```\n")
	mint := parseOne(t, "```ask\non: mint\n\nAsk.\n```\n")
	edit := parseOne(t, "```ask\non: edit\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.md")

	writeEdit := Edit{Path: p, New: "x", Disk: "x", Exists: true}
	readEdit := Edit{Path: p, New: "", Disk: "x", Exists: true, IsRead: true}

	if _, ok := match(read, writeEdit); ok {
		t.Error("on: read fired on a Write/Edit-shaped call")
	}
	if _, ok := match(read, readEdit); !ok {
		t.Error("on: read did not fire on a Read-shaped call")
	}
	for name, r := range map[string]*Ask{"any": any, "mint": mint, "edit": edit} {
		if _, ok := match(r, readEdit); ok {
			t.Errorf("on: %s fired on a Read-shaped call", name)
		}
	}
}

// has: reads the file as it stands regardless of what triggered the call,
// so it works against a Read the same way it does against a pending write.
func TestOnReadWithHasStillReadsDisk(t *testing.T) {
	r := parseOne(t, "```ask\non: read\nhas: sync\\.Mutex\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.go")
	if _, ok := match(r, Edit{Path: p, Disk: "no locks here", Exists: true, IsRead: true}); ok {
		t.Error("has: fired against disk content that doesn't match")
	}
	if _, ok := match(r, Edit{Path: p, Disk: "var m sync.Mutex", Exists: true, IsRead: true}); !ok {
		t.Error("has: did not fire against matching disk content on a Read")
	}
}

// Identity must survive an unrelated edit to the same CLAUDE.md, or inserting
// a paragraph above an ask re-fires every ask below it for the session.
func TestIDIgnoresPosition(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: x\n\nAsk.\n```\n")
	b := parseOne(t, "a heading\n\nsome prose\n\n```ask\nwhen: x\n\nAsk.\n```\n")
	if a.ID() != b.ID() {
		t.Error("ID changed when only the surrounding text moved")
	}
	c := parseOne(t, "```ask\nwhen: y\n\nAsk.\n```\n")
	if a.ID() == c.ID() {
		t.Error("ID survived a change to the gate")
	}
}

func TestBracesAndDoublestar(t *testing.T) {
	r := parseOne(t, "```ask\nin: {locations,chars}/**/*.yaml\n\nAsk.\n```\n")
	for _, tc := range []struct {
		path string
		want bool
	}{
		{"/repo/corpus/chars/a/b.yaml", true},
		{"/repo/corpus/locations/x.yaml", true},
		{"/repo/corpus/props/x.yaml", false},
		{"/repo/corpus/chars/a/b.json", false},
	} {
		if _, ok := match(r, Edit{Path: filepath.FromSlash(tc.path), New: "x", Disk: "x", Exists: true}); ok != tc.want {
			t.Errorf("%s: ok = %v, want %v", tc.path, ok, tc.want)
		}
	}
}

// Repeated `when:` is an AND. Two conditions across the whole text is a real
// shape — "touches a fact_text block AND contains inference language" — and
// RE2 has no lookahead to express it in one pattern.
func TestRepeatedWhenIsConjunction(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: fact_text\nwhen: you realize\n\nAsk.\n```\n")
	if len(r.When) != 2 {
		t.Fatalf("got %d when clauses, want 2", len(r.When))
	}
	p := filepath.FromSlash("/repo/corpus/a.json")
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"both":        {`"fact_text": "you realize she lied"`, true},
		"first only":  {`"fact_text": "the door is ajar"`, false},
		"second only": {`"desc": "you realize she lied"`, false},
		"neither":     {`"desc": "the door is ajar"`, false},
	} {
		m, ok := match(r, Edit{Path: p, New: tc.content, Disk: tc.content, Exists: true})
		if ok != tc.want {
			t.Errorf("%s: ok = %v, want %v", name, ok, tc.want)
		}
		if ok && m != "fact_text" {
			t.Errorf("%s: quoted %q, want the first clause's match", name, m)
		}
	}
}

// Repeated `not:` is an OR of suppressors — any one of them silences the ask.
func TestRepeatedNotIsDisjunction(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nnot: TODO\\(alice\\)\nnot: generated by\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.md")
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"clean":       {"TODO fix this", true},
		"suppressor1": {"TODO(alice) fix this", false},
		"suppressor2": {"generated by portkit; TODO fix", false},
	} {
		if _, ok := match(r, Edit{Path: p, New: tc.content, Disk: tc.content, Exists: true}); ok != tc.want {
			t.Errorf("%s: ok = %v, want %v", name, ok, tc.want)
		}
	}
}

// `added:` matches only the lines the edit introduces. The case that matters
// is an edit whose replaced span already contained the pattern: the count went
// nowhere, the line was never touched, and the ask must stay quiet.
func TestAddedSeesOnlyIntroducedLines(t *testing.T) {
	r := parseOne(t, "```ask\nadded: \\bpanic\\(\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.go")

	if _, ok := match(r, Edit{Path: p, Old: "x := 1\n", New: "x := 1\npanic(\"new\")\n", Exists: true}); !ok {
		t.Error("a newly added panic did not fire")
	}
	if _, ok := match(r, Edit{
		Path:   p,
		Old:    "panic(\"old\")\nx := 1\n",
		New:    "panic(\"old\")\nx := 2\n",
		Exists: true,
	}); ok {
		t.Error("a panic that was already there fired as if added")
	}
	// A Write has no old text, so everything in it is added.
	if _, ok := match(r, Edit{Path: p, New: "panic(\"x\")\n", Exists: false}); !ok {
		t.Error("a Write containing the pattern did not fire")
	}
}

// `removed:` is the class nothing else here can see: an edit that takes
// something out leaves no trace in the text being written.
func TestRemovedSeesOnlyDeletedLines(t *testing.T) {
	r := parseOne(t, "```ask\nremoved: if err != nil\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.go")

	m, ok := match(r, Edit{
		Path:   p,
		Old:    "v, err := f()\nif err != nil {\n\treturn err\n}\n",
		New:    "v, _ := f()\n",
		Exists: true,
	})
	if !ok {
		t.Fatal("deleting the error check did not fire")
	}
	if !strings.Contains(m, "if err != nil") {
		t.Errorf("quoted %q, want the deleted text", m)
	}
	if _, ok := match(r, Edit{Path: p, Old: "x := 1\n", New: "if err != nil {\n", Exists: true}); ok {
		t.Error("adding the pattern fired a removed: gate")
	}
}

// added:/removed: run against the diff's inserted/deleted lines joined into
// one string, so a bare `^` in the pattern used to anchor to the start of
// that whole block rather than the start of each line — reported live by a
// real user: an ask matching only when its line happened to land first in
// the diff, and a sibling ask that could never fire at all, since the line
// it wanted was essentially never the first one. multiline() fixes this by
// compiling with (?m), so `^` means what "matches a line" already implied.
func TestAddedAnchorsPerLineNotPerBlock(t *testing.T) {
	r := parseOne(t, "```ask\nadded: ^\\s*[-*] a real entry\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/BOUNDARY.md")

	// The matching line is second in the diff, not first — this is exactly
	// the shape that silently never fired before (?m) was added.
	old := "# Boundary\n\n"
	new := "# Boundary\n\n- an unrelated first entry\n- a real entry\n"
	if _, ok := match(r, Edit{Path: p, Old: old, New: new, Exists: true}); !ok {
		t.Error("added: with ^ did not fire on a match that wasn't the first added line")
	}
}

func TestRemovedAnchorsPerLineNotPerBlock(t *testing.T) {
	r := parseOne(t, "```ask\nremoved: ^func (Check|Law)\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/foo.go")

	// The deleted function signature is second in the diff, not first.
	old := "func Helper() {}\n\nfunc Check(x int) bool { return x > 0 }\n"
	new := "func Helper() {}\n"
	if _, ok := match(r, Edit{Path: p, Old: old, New: new, Exists: true}); !ok {
		t.Error("removed: with ^ did not fire on a deleted line that wasn't first")
	}
}

// maxDiffBytes exists so an edit large enough to make the Myers diff
// expensive skips diffLines entirely rather than paying its cost — this
// pins the cap's behavior at both sides without needing an edit anywhere
// near the size that made the cap necessary in the first place.
func TestAddedSkipsTheDiffPastTheSizeCap(t *testing.T) {
	r := parseOne(t, "```ask\nadded: panic\\(\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.go")

	// Comfortably under the cap: still diffs normally and fires.
	small := strings.Repeat("x\n", 10_000) // 20,000 bytes
	if _, ok := match(r, Edit{Path: p, Old: small, New: small + "panic(\"x\")\n", Exists: true}); !ok {
		t.Error("an edit under maxDiffBytes did not fire")
	}

	// Comfortably over the cap: would fire if diffed, must not once the
	// added: pattern only exists in a New the cap refuses to diff.
	big := strings.Repeat("x\n", 60_000) // 120,000 bytes, over maxDiffBytes
	if _, ok := match(r, Edit{Path: p, Old: big, New: big + "panic(\"x\")\n", Exists: true}); ok {
		t.Error("an edit over maxDiffBytes fired added: instead of skipping the diff")
	}
}

// `has:` gates on the file as it stands, not on the edit — "this file already
// does X and you are adding a second way to do it".
func TestHasGatesOnTheFileNotTheEdit(t *testing.T) {
	r := parseOne(t, "```ask\nhas: sync\\.Mutex\nwhen: sync\\.RWMutex\n\nAsk.\n```\n")
	p := filepath.FromSlash("/repo/corpus/a.go")

	if _, ok := match(r, Edit{Path: p, New: "var m sync.RWMutex", Disk: "var a sync.Mutex", Exists: true}); !ok {
		t.Error("a second lock type in a file that already has one did not fire")
	}
	if _, ok := match(r, Edit{Path: p, New: "var m sync.RWMutex", Disk: "no locks here", Exists: true}); ok {
		t.Error("fired on a file that does not have the first lock")
	}
}

// The reason is the whole point of the Result: with nine headers, "would not
// fire" leaves the author deleting one at a time to find the one that meant it.
func TestRejectionNamesTheGateThatRejected(t *testing.T) {
	p := filepath.FromSlash("/repo/corpus/chars/betty.md")
	cases := []struct {
		name string
		src  string
		e    Edit
		gate string
		note string
	}{
		{
			"in", "```ask\nin: locations/**\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true},
			"in", "does not match chars/betty.md",
		},
		{
			"not-in", "```ask\nin: chars/**\nnot-in: **/betty.md\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true},
			"not-in", "matches chars/betty.md",
		},
		{
			"on", "```ask\non: mint\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true},
			"on", "the file already exists",
		},
		{
			"not", "```ask\nnot: WIP\n\nAsk.\n```\n",
			Edit{Path: p, New: "WIP do not review", Exists: true},
			"not", `matched "WIP", which suppresses the ask`,
		},
		{
			"has", "```ask\nhas: sync\\.Mutex\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Disk: "no locks here", Exists: true},
			"has", "nothing like it in the file as it stands",
		},
		{
			"untouched", "```ask\nuntouched: migrations/**\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true,
				Touched: []string{filepath.FromSlash("/repo/corpus/migrations/003.sql")}},
			"untouched", "migrations/003.sql was written this session",
		},
		{
			"added", "```ask\nadded: panic\\(\n\nAsk.\n```\n",
			Edit{Path: p, Old: "x := 1\n", New: "x := 2\n", Exists: true},
			"added", "nothing like it in the lines this edit adds",
		},
		{
			"removed", "```ask\nremoved: panic\\(\n\nAsk.\n```\n",
			Edit{Path: p, Old: "x := 1\n", New: "x := 2\n", Exists: true},
			"removed", "nothing like it in the lines this edit removes",
		},
		{
			"when", "```ask\nwhen: \\bTODO\\b\n\nAsk.\n```\n",
			Edit{Path: p, New: "nothing to see", Exists: true},
			"when", "nothing like it in the incoming text",
		},
		{
			"requires", "```ask\nrequires: definitely-not-a-real-binary-onsetter-test\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true},
			"requires", "not found on $PATH",
		},
		{
			"evokes nil predicate", "```ask\nevokes: committing without asking\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true}, // Evokes left nil
			"evokes", "the edit does not evoke any of these",
		},
		{
			"evokes predicate says no", "```ask\nevokes: committing without asking\n\nAsk.\n```\n",
			Edit{Path: p, New: "x", Exists: true, Evokes: func(string) bool { return false }},
			"evokes", "the edit does not evoke any of these",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := parseOne(t, tc.src).Match(tc.e)
			if res.OK {
				t.Fatalf("fired, want a rejection at %s:", tc.gate)
			}
			if res.Gate != tc.gate {
				t.Errorf("rejected at %q, want %q (%s)", res.Gate, tc.gate, res.Why())
			}
			if res.Note != tc.note {
				t.Errorf("note %q, want %q", res.Note, tc.note)
			}
			if !strings.Contains(res.Why(), tc.gate) {
				t.Errorf("Why() = %q, does not name the gate", res.Why())
			}
		})
	}
}

// A caller outside onsetter's own hook (winze-agent's capture-guard is the
// motivating one) has no file path to give Match — the text it wants matched
// lives in an MCP tool argument, not a Write/Edit. when:/not:/has: don't need
// a path and should still run; in:/not-in:/untouched: do, and must say so
// instead of failing for a path-scoping reason that does not apply.
func TestMatchWithoutAPath(t *testing.T) {
	pathless := Edit{New: "TODO fix this", Disk: "TODO fix this", Exists: true}

	t.Run("when-only ask fires without a path", func(t *testing.T) {
		r := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
		if _, ok := match(r, pathless); !ok {
			t.Error("want a fire: when: does not need a path")
		}
	})

	t.Run("default in: does not block a path-less call", func(t *testing.T) {
		r := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n") // in: defaults to **
		if _, ok := match(r, pathless); !ok {
			t.Error("want a fire: an unset in: is not a path requirement")
		}
	})

	cases := []struct {
		name string
		src  string
		gate string
	}{
		{"in", "```ask\nin: **/*.md\n\nAsk.\n```\n", "in"},
		{"not-in", "```ask\nnot-in: **/gen/**\n\nAsk.\n```\n", "not-in"},
		{"untouched", "```ask\nuntouched: migrations/**\n\nAsk.\n```\n", "untouched"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := parseOne(t, tc.src).Match(pathless)
			if res.OK {
				t.Fatalf("fired, want a rejection at %s: — this ask cannot honor %s: without a path", tc.gate, tc.gate)
			}
			if res.Gate != tc.gate {
				t.Errorf("rejected at %q, want %q (%s)", res.Gate, tc.gate, res.Why())
			}
			if !strings.Contains(res.Note, "a path") {
				t.Errorf("Note = %q, want it to say this ask needs a path", res.Note)
			}
		})
	}

	t.Run("on: mint/edit read Exists, not a path", func(t *testing.T) {
		mint := parseOne(t, "```ask\non: mint\n\nAsk.\n```\n")
		if _, ok := match(mint, Edit{New: "x", Exists: false}); !ok {
			t.Error("on: mint should fire on a path-less call with Exists: false")
		}
		if _, ok := match(mint, Edit{New: "x", Exists: true}); ok {
			t.Error("on: mint should not fire on a path-less call with Exists: true")
		}
	})
}

// requires: gates on the machine, not the file — go is whatever binary is
// running this test, so it is present by construction, and the nonsense name
// is never going to resolve.
func TestRequiresGatesOnPATH(t *testing.T) {
	p := filepath.FromSlash("/repo/corpus/chars/betty.md")
	e := Edit{Path: p, New: "x", Exists: true}

	t.Run("fires when the binary resolves", func(t *testing.T) {
		r := parseOne(t, "```ask\nrequires: go\n\nAsk.\n```\n")
		if _, ok := match(r, e); !ok {
			t.Error("want a fire: go is on $PATH")
		}
	})

	t.Run("rejects when it does not", func(t *testing.T) {
		r := parseOne(t, "```ask\nrequires: definitely-not-a-real-binary-onsetter-test\n\nAsk.\n```\n")
		if _, ok := match(r, e); ok {
			t.Error("want a rejection: this binary does not exist")
		}
	})

	t.Run("repeated requires: is an AND", func(t *testing.T) {
		r := parseOne(t, "```ask\nrequires: go\nrequires: definitely-not-a-real-binary-onsetter-test\n\nAsk.\n```\n")
		res := r.Match(e)
		if res.OK {
			t.Fatal("want a rejection: the second requires: does not resolve")
		}
		if res.Gate != "requires" || res.Pattern != "definitely-not-a-real-binary-onsetter-test" {
			t.Errorf("rejected at %q: %q, want requires: naming the missing binary (%s)", res.Gate, res.Pattern, res.Why())
		}
	})

	t.Run("checked before in:, so the rejection names the real reason", func(t *testing.T) {
		// This ask would also fail in: — betty.md is not under locations/.
		// requires: runs first, so that is what the rejection should name.
		r := parseOne(t, "```ask\nrequires: definitely-not-a-real-binary-onsetter-test\nin: locations/**\n\nAsk.\n```\n")
		res := r.Match(e)
		if res.Gate != "requires" {
			t.Errorf("rejected at %q, want requires: — it is checked first (%s)", res.Gate, res.Why())
		}
	})

	t.Run("path-less call still evaluates requires:", func(t *testing.T) {
		r := parseOne(t, "```ask\nrequires: definitely-not-a-real-binary-onsetter-test\n\nAsk.\n```\n")
		res := r.Match(Edit{New: "x", Exists: true})
		if res.Gate != "requires" {
			t.Errorf("rejected at %q, want requires: even with no path (%s)", res.Gate, res.Why())
		}
	})
}

// evokes: takes a caller-supplied predicate instead of doing any matching
// itself — Match stays pure and network-free, and the whole embedding
// pipeline (internal/embed) is exercised nowhere near this package.
func TestEvokesGatesOnAPredicate(t *testing.T) {
	p := filepath.FromSlash("/repo/corpus/chars/betty.md")
	e := Edit{Path: p, New: "x", Exists: true}

	t.Run("fires when the predicate matches", func(t *testing.T) {
		r := parseOne(t, "```ask\nevokes: committing without asking\n\nAsk.\n```\n")
		e := e
		e.Evokes = func(string) bool { return true }
		matched, ok := match(r, e)
		if !ok {
			t.Fatal("want a fire: predicate returns true")
		}
		if matched != "committing without asking" {
			t.Errorf("Matched = %q, want the evoked phrase", matched)
		}
	})

	t.Run("rejects when the predicate never matches", func(t *testing.T) {
		r := parseOne(t, "```ask\nevokes: committing without asking\n\nAsk.\n```\n")
		e := e
		e.Evokes = func(string) bool { return false }
		if _, ok := match(r, e); ok {
			t.Error("want a rejection: predicate returns false for every phrase")
		}
	})

	t.Run("repeated evokes: is an OR, unlike requires:'s AND", func(t *testing.T) {
		r := parseOne(t, "```ask\nevokes: pushing to prod\nevokes: committing without asking\n\nAsk.\n```\n")
		e := e
		e.Evokes = func(phrase string) bool { return phrase == "committing without asking" }
		matched, ok := match(r, e)
		if !ok {
			t.Fatal("want a fire: the second phrase matches, and evokes: is an OR")
		}
		if matched != "committing without asking" {
			t.Errorf("Matched = %q, want the phrase that actually matched", matched)
		}
	})

	t.Run("nil predicate rejects rather than blocking on it", func(t *testing.T) {
		r := parseOne(t, "```ask\nevokes: committing without asking\n\nAsk.\n```\n")
		if _, ok := match(r, e); ok { // e.Evokes is nil
			t.Error("want a rejection: no fuzzy stage ran for this call")
		}
	})

	t.Run("checked after when:, so a failing when: names the real reason", func(t *testing.T) {
		r := parseOne(t, "```ask\nwhen: TODO\nevokes: committing without asking\n\nAsk.\n```\n")
		e := e
		e.Evokes = func(string) bool { return true } // would fire, if reached
		res := r.Match(e)
		if res.OK {
			t.Fatal("want a rejection: when: does not match")
		}
		if res.Gate != "when" {
			t.Errorf("rejected at %q, want when: — evokes: runs last (%s)", res.Gate, res.Why())
		}
	})
}

// An evokes:-only ask narrows on content just as much as a when:-only one —
// onsetter lint should not flag it as a banner.
func TestGatedCountsEvokes(t *testing.T) {
	r := parseOne(t, "```ask\nevokes: committing without asking\n\nAsk.\n```\n")
	if !r.Gated() {
		t.Error("an evokes:-only ask should be Gated()")
	}
}

// Identity has to move when the phrase list does, the same as every other
// header: editing an ask re-arms it instead of staying silently fired.
func TestIDChangesWithEvokes(t *testing.T) {
	a := parseOne(t, "```ask\nevokes: one thing\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nevokes: a different thing\n\nAsk.\n```\n")
	if a.ID() == b.ID() {
		t.Error("two asks with different evokes: phrases got the same ID")
	}
}

func TestRevisitParsesAndDefaultsFalse(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nrevisit: true\n\nAsk.\n```\n")
	if !r.Revisit {
		t.Error("revisit: true did not set Revisit")
	}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	if plain.Revisit {
		t.Error("an ask with no revisit: header should default to false")
	}
}

// Case-insensitive the same way on: is: `on: MINT` already works, so
// `revisit: True` should too rather than rejecting on a technicality.
func TestRevisitAcceptsAnyCaseOfTrue(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nrevisit: True\n\nAsk.\n```\n")
	if !r.Revisit {
		t.Error("revisit: True (capitalized) did not set Revisit")
	}
}

// "true" is the only accepted spelling — there is no antonym, so a stray
// "false" would silently do nothing rather than the no-op it looks like.
func TestRevisitRejectsAnyOtherValue(t *testing.T) {
	for _, v := range []string{"false", "yes", "1"} {
		_, err := Parse(strings.NewReader("```ask\nrevisit: "+v+"\n\nAsk.\n```\n"), "/repo/CLAUDE.md")
		if err == nil {
			t.Errorf("revisit: %s parsed cleanly, want an error", v)
		}
	}
}

// revisit: never gates — Match fires or rejects exactly the same with or
// without it, because the hook dispatcher, not Match, is what reads it.
func TestRevisitNeverAffectsMatch(t *testing.T) {
	e := Edit{New: "TODO: fix this"}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	revisiting := parseOne(t, "```ask\nwhen: TODO\nrevisit: true\n\nAsk.\n```\n")
	pm, pok := match(plain, e)
	rm, rok := match(revisiting, e)
	if pok != rok || pm != rm {
		t.Errorf("revisit: true changed Match's outcome: (%q, %v) vs (%q, %v)", pm, pok, rm, rok)
	}
}

// The inverse of every other header, on purpose: revisit: true is retired
// (every matched ask always widens the session key now), so it no longer
// changes what an ask does, and ID() stops treating it as part of the
// question — unlike Requires and Name, which were always excluded, this is
// a deliberate change from prior behavior, pinned here the same way
// TestRevisitNeverAffectsMatch already pins Match's side of the same fact.
func TestIDDoesNotChangeWithRevisit(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nwhen: TODO\nrevisit: true\n\nAsk.\n```\n")
	if a.ID() != b.ID() {
		t.Error("adding revisit: true changed the ask's ID, but revisit: is retired and should no longer affect identity")
	}
}

func TestAlwaysParsesAndDefaultsFalse(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nalways: true\n\nAsk.\n```\n")
	if !r.Always {
		t.Error("always: true did not set Always")
	}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	if plain.Always {
		t.Error("an ask with no always: header should default to false")
	}
}

// "true" is the only accepted spelling, same reasoning as revisit:'s own
// test: there is no antonym, so a stray "false" would silently do nothing
// rather than the no-op it looks like.
func TestAlwaysRejectsAnyOtherValue(t *testing.T) {
	for _, v := range []string{"false", "yes", "1"} {
		_, err := Parse(strings.NewReader("```ask\nalways: "+v+"\n\nAsk.\n```\n"), "/repo/CLAUDE.md")
		if err == nil {
			t.Errorf("always: %s parsed cleanly, want an error", v)
		}
	}
}

// always: never gates, the same as revisit: — Match fires or rejects
// exactly the same with or without it, because the hook dispatcher, not
// Match, is what reads it.
func TestAlwaysNeverAffectsMatch(t *testing.T) {
	e := Edit{New: "TODO: fix this"}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	always := parseOne(t, "```ask\nwhen: TODO\nalways: true\n\nAsk.\n```\n")
	pm, pok := match(plain, e)
	am, aok := match(always, e)
	if pok != aok || pm != am {
		t.Errorf("always: true changed Match's outcome: (%q, %v) vs (%q, %v)", pm, pok, am, aok)
	}
}

// Identity has to move with always: too, the same as every other header.
func TestIDChangesWithAlways(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nwhen: TODO\nalways: true\n\nAsk.\n```\n")
	if a.ID() == b.ID() {
		t.Error("adding always: true did not change the ask's ID")
	}
}

func TestBlockParsesAlongsideAddedOrRemoved(t *testing.T) {
	added := parseOne(t, "```ask\nadded: TODO\nblock: true\n\nAsk.\n```\n")
	if !added.Block {
		t.Error("block: true did not set Block on an added: ask")
	}
	removed := parseOne(t, "```ask\nremoved: TODO\nblock: true\n\nAsk.\n```\n")
	if !removed.Block {
		t.Error("block: true did not set Block on a removed: ask")
	}
	plain := parseOne(t, "```ask\nadded: TODO\n\nAsk.\n```\n")
	if plain.Block {
		t.Error("an ask with no block: header should default to false")
	}
}

// block: true only makes sense next to a gate that hands back the exact
// text being denied — a when:-only or has:-only ask has nothing concrete to
// point at, so this is a parse error, not a silently-inert combination.
func TestBlockRequiresAddedOrRemoved(t *testing.T) {
	for _, src := range []string{
		"```ask\nwhen: TODO\nblock: true\n\nAsk.\n```\n",
		"```ask\nhas: TODO\nblock: true\n\nAsk.\n```\n",
		"```ask\nblock: true\n\nAsk.\n```\n",
	} {
		_, err := Parse(strings.NewReader(src), "/repo/CLAUDE.md")
		if err == nil {
			t.Errorf("block: true with no added:/removed: parsed cleanly, want an error: %s", src)
		}
	}
}

// "true" is the only accepted spelling, same reasoning as always:'s own test.
func TestBlockRejectsAnyOtherValue(t *testing.T) {
	for _, v := range []string{"false", "yes", "1"} {
		_, err := Parse(strings.NewReader("```ask\nadded: TODO\nblock: "+v+"\n\nAsk.\n```\n"), "/repo/CLAUDE.md")
		if err == nil {
			t.Errorf("block: %s parsed cleanly, want an error", v)
		}
	}
}

// block: never gates — Match fires or rejects exactly the same with or
// without it, because the hook dispatcher, not Match, is what reads it and
// decides whether to deny.
func TestBlockNeverAffectsMatch(t *testing.T) {
	e := Edit{New: "TODO: fix this"}
	plain := parseOne(t, "```ask\nadded: TODO\n\nAsk.\n```\n")
	blocked := parseOne(t, "```ask\nadded: TODO\nblock: true\n\nAsk.\n```\n")
	pm, pok := match(plain, e)
	bm, bok := match(blocked, e)
	if pok != bok || pm != bm {
		t.Errorf("block: true changed Match's outcome: (%q, %v) vs (%q, %v)", pm, pok, bm, bok)
	}
}

// Identity has to move with block: too, the same as always:.
func TestIDChangesWithBlock(t *testing.T) {
	a := parseOne(t, "```ask\nadded: TODO\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nadded: TODO\nblock: true\n\nAsk.\n```\n")
	if a.ID() == b.ID() {
		t.Error("adding block: true did not change the ask's ID")
	}
}

func TestFiresOnAndSilentOnParseAsRepeatableGlobs(t *testing.T) {
	r := parseOne(t, "```ask\nwhen: TODO\nfires-on: fixtures/bad.go\nfires-on: fixtures/also_bad.go\nsilent-on: fixtures/good.go\n\nAsk.\n```\n")
	if want := []string{"fixtures/bad.go", "fixtures/also_bad.go"}; !slices.Equal(r.FiresOn, want) {
		t.Errorf("FiresOn = %v, want %v", r.FiresOn, want)
	}
	if want := []string{"fixtures/good.go"}; !slices.Equal(r.SilentOn, want) {
		t.Errorf("SilentOn = %v, want %v", r.SilentOn, want)
	}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	if len(plain.FiresOn) != 0 || len(plain.SilentOn) != 0 {
		t.Error("an ask with no fires-on:/silent-on: header should default to empty")
	}
}

// fires-on:/silent-on: are read only by onsetter lint, never by Match —
// same reasoning as always:/block:'s own test.
func TestFiresOnAndSilentOnNeverAffectMatch(t *testing.T) {
	e := Edit{New: "TODO: fix this"}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	fixtured := parseOne(t, "```ask\nwhen: TODO\nfires-on: a.go\nsilent-on: b.go\n\nAsk.\n```\n")
	pm, pok := match(plain, e)
	fm, fok := match(fixtured, e)
	if pok != fok || pm != fm {
		t.Errorf("fires-on:/silent-on: changed Match's outcome: (%q, %v) vs (%q, %v)", pm, pok, fm, fok)
	}
}

// Unlike always: and block:, fires-on:/silent-on: don't change identity —
// they're lint's own fixture list, not part of the question the ask asks,
// the same reasoning that leaves name: out of ID().
func TestIDIgnoresFiresOnAndSilentOn(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nwhen: TODO\nfires-on: a.go\nsilent-on: b.go\n\nAsk.\n```\n")
	if a.ID() != b.ID() {
		t.Error("adding fires-on:/silent-on: changed the ask's ID, want it unchanged")
	}
}

// Every header the parser accepts is in Headers, so the parse error, the
// reference and replay's funnel cannot fall out of step with the switch.
func TestHeadersListsEveryHeaderTheParserAccepts(t *testing.T) {
	for _, h := range Headers {
		v := "x"
		prefix := ""
		switch h {
		case "on":
			v = "mint"
		case "revisit", "always":
			v = "true"
		case "block":
			// block: true only parses alongside added: or removed: — see
			// TestBlockRequiresAddedOrRemoved for that restriction itself.
			v = "true"
			prefix = "added: x\n"
		}
		if _, err := Parse(strings.NewReader("```ask\n"+prefix+h+": "+v+"\n\nAsk.\n```\n"), "/repo/CLAUDE.md"); err != nil {
			t.Errorf("Headers lists %q but the parser rejects it: %v", h, err)
		}
	}
	_, err := Parse(strings.NewReader("```ask\nnope: x\n\nAsk.\n```\n"), "/repo/CLAUDE.md")
	if err == nil {
		t.Fatal("an unknown header parsed cleanly")
	}
	for _, h := range Headers {
		if !strings.Contains(err.Error(), h) {
			t.Errorf("the unknown-header error does not mention %q: %v", h, err)
		}
	}
}

// The reference ships with the binary so an agent can read it without knowing
// a path. That only helps if it stays true: a header added to the parser and
// not to the doc is a header nobody drafting an ask will ever find.
func TestReferenceDocumentsEveryHeader(t *testing.T) {
	ref := Reference()
	for _, h := range Headers {
		if !strings.Contains(ref, "### `"+h+":`") && !strings.Contains(ref, "`"+h+":` and") &&
			!strings.Contains(ref, "and `"+h+":`") {
			t.Errorf("headers.md has no section for %q", h)
		}
		if !strings.Contains(ref, "| `"+h+"`") {
			t.Errorf("headers.md's table is missing a row for %q", h)
		}
	}
	// The doc's own example must be a block the parser accepts, or the first
	// thing a reader copies is broken.
	if _, err := Parse(strings.NewReader(ref), "/repo/CLAUDE.md"); err != nil {
		t.Errorf("an ```ask block in headers.md does not parse: %v", err)
	}
}

// The skill is retrieved by its description and by nothing else, so a header
// missing from that one paragraph is a header whose questions never reach the
// guide. Everything past the frontmatter can say what it likes; the front
// matter is the index.
func TestSkillFrontmatterNamesEveryHeader(t *testing.T) {
	doc := Skill()
	if !strings.HasPrefix(doc, "---\nname: onsetter\ndescription: ") {
		t.Fatalf("skill.md must open with frontmatter naming the skill; got %.40q", doc)
	}
	end := strings.Index(doc[4:], "\n---\n")
	if end < 0 {
		t.Fatal("skill.md's frontmatter is never closed")
	}
	front := doc[:end+4]
	// The literal list, not nine substring checks: "in" and "on" and "not" are
	// each a substring of half the prose in that paragraph, so a loose check
	// passes no matter what the description says.
	list := strings.Join(Headers, ", ")
	if !strings.Contains(front, list) {
		t.Errorf("skill.md's description must list the headers as %q, or the skill will not surface for the ones it omits", list)
	}
}

func TestNameParsesAndDefaultsEmpty(t *testing.T) {
	r := parseOne(t, "```ask\nname: check-token-scope\n\nAsk.\n```\n")
	if r.Name != "check-token-scope" {
		t.Errorf("name: did not set Name, got %q", r.Name)
	}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	if plain.Name != "" {
		t.Error("an ask with no name: header should default to empty")
	}
}

// Repeated name: is last wins, the same as in: and on: — a silent overwrite
// rather than an accumulation, unlike a regex or requires: header.
func TestNameRepeatedIsLastWins(t *testing.T) {
	r := parseOne(t, "```ask\nname: first\nname: second\n\nAsk.\n```\n")
	if r.Name != "second" {
		t.Errorf("repeated name: should be last wins, got %q", r.Name)
	}
}

func TestCuesRepeatedIsAppend(t *testing.T) {
	r := parseOne(t, "```ask\ncues: alpha\ncues: beta\n\nAsk.\n```\n")
	if len(r.Cues) != 2 || r.Cues[0] != "alpha" || r.Cues[1] != "beta" {
		t.Errorf("cues: should accumulate, got %v", r.Cues)
	}
}

// name: and cues: are never gates — Match fires or rejects exactly the same
// with or without them, the same guarantee TestRevisitNeverAffectsMatch
// makes for revisit:.
func TestNameAndCuesNeverAffectMatch(t *testing.T) {
	e := Edit{New: "TODO: fix this"}
	plain := parseOne(t, "```ask\nwhen: TODO\n\nAsk.\n```\n")
	decorated := parseOne(t, "```ask\nwhen: TODO\nname: x\ncues: y\n\nAsk.\n```\n")
	pm, pok := match(plain, e)
	dm, dok := match(decorated, e)
	if pok != dok || pm != dm {
		t.Errorf("name:/cues: changed Match's outcome: (%q, %v) vs (%q, %v)", pm, pok, dm, dok)
	}
}

// Identity must not move with name: alone — renaming an ask (to fix a
// collision, say) should not re-arm every already-answered session
// instance of it, the same treatment Requires already gets.
func TestIDIgnoresName(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: x\nname: first\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nwhen: x\nname: second\n\nAsk.\n```\n")
	if a.ID() != b.ID() {
		t.Error("ID changed when only name: changed")
	}
}

// Identity has to move with cues:, unlike name: — a freshly wired cue on an
// ask that already fired this session (with an unchanged quote) needs a new
// key or it never gets a chance to walk during that session.
func TestIDChangesWithCues(t *testing.T) {
	a := parseOne(t, "```ask\nwhen: x\n\nAsk.\n```\n")
	b := parseOne(t, "```ask\nwhen: x\ncues: something\n\nAsk.\n```\n")
	if a.ID() == b.ID() {
		t.Error("adding cues: did not change the ask's ID")
	}
}

func TestByNameSkipsUnnamed(t *testing.T) {
	named := parseOne(t, "```ask\nname: alpha\n\nAsk.\n```\n")
	unnamed := parseOne(t, "```ask\nwhen: x\n\nAsk.\n```\n")
	m := ByName([]*Ask{named, unnamed})
	if len(m) != 1 || m["alpha"] != named {
		t.Errorf("ByName should index only the named ask, got %v", m)
	}
}

func parseAt(t *testing.T, source, src string) *Ask {
	t.Helper()
	rs, err := Parse(strings.NewReader(src), source)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(rs) != 1 {
		t.Fatalf("got %d asks, want 1", len(rs))
	}
	return rs[0]
}

func TestValidateCuesFindsDuplicateName(t *testing.T) {
	a := parseAt(t, "/repo/CLAUDE.md", "```ask\nname: dup\n\nFirst.\n```\n")
	b := parseAt(t, "/repo/CLAUDE.md", "```ask\nname: dup\n\nSecond.\n```\n")
	problems := ValidateCues([]*Ask{a, b})
	if len(problems) != 1 || problems[0].Kind != "duplicate-name" || problems[0].Value != "dup" {
		t.Errorf("want one duplicate-name problem for %q, got %v", "dup", problems)
	}
}

func TestValidateCuesFindsDanglingCue(t *testing.T) {
	a := parseAt(t, "/repo/CLAUDE.md", "```ask\ncues: nowhere\n\nAsk.\n```\n")
	problems := ValidateCues([]*Ask{a})
	if len(problems) != 1 || problems[0].Kind != "dangling-cue" || problems[0].Value != "nowhere" {
		t.Errorf("want one dangling-cue problem for %q, got %v", "nowhere", problems)
	}
}

// A target declared below the citer's own directory resolves in a
// whole-repo ByName view but can never actually be reached at hook time,
// since discover.Asks only ever walks upward from the edited file. This is
// the scope-mismatch gap the design has to flag rather than silently trust.
func TestValidateCuesFindsCueOutOfScope(t *testing.T) {
	citer := parseAt(t, "/repo/CLAUDE.md", "```ask\ncues: nested\n\nAsk.\n```\n")
	target := parseAt(t, "/repo/sub/CLAUDE.md", "```ask\nname: nested\n\nTarget.\n```\n")
	problems := ValidateCues([]*Ask{citer, target})
	if len(problems) != 1 || problems[0].Kind != "cue-out-of-scope" || problems[0].Value != "nested" {
		t.Errorf("want one cue-out-of-scope problem for %q, got %v", "nested", problems)
	}
}

// The mirror image of the scope test above: a target declared in an
// ancestor of the citer (or the same file) is exactly the supported shape
// and should report clean.
func TestValidateCuesAllowsAncestorAndSameFileTargets(t *testing.T) {
	root := parseAt(t, "/repo/CLAUDE.md", "```ask\nname: root-target\n\nRoot.\n```\n")
	nested := parseAt(t, "/repo/sub/CLAUDE.md", "```ask\ncues: root-target\n\nAsk.\n```\n")
	sameFileA := parseAt(t, "/repo/CLAUDE.md", "```ask\nname: sibling\n\nA.\n```\n")
	sameFileB := parseAt(t, "/repo/CLAUDE.md", "```ask\ncues: sibling\n\nB.\n```\n")
	if problems := ValidateCues([]*Ask{root, nested, sameFileA, sameFileB}); len(problems) != 0 {
		t.Errorf("want no problems for an ancestor or same-file target, got %v", problems)
	}
}

func TestCascadeFiresACuedTargetWithNoQuote(t *testing.T) {
	target := parseOne(t, "```ask\nname: check-token-scope\nwhen: never-matches-directly\n\nCued prose.\n```\n")
	citer := parseOne(t, "```ask\ncues: check-token-scope\n\nCiting ask.\n```\n")
	asks := []*Ask{citer, target}
	direct := []CascadeHit{{Ask: citer, Matched: "fetch(creds)"}}

	out := Cascade(asks, direct)
	if len(out) != 2 {
		t.Fatalf("want 2 hits (direct + cued), got %d: %v", len(out), out)
	}
	cued := out[1]
	if cued.Ask != target || cued.Matched != "" || cued.Via != citer {
		t.Errorf("cued hit wrong shape: %+v", cued)
	}
}

// A cycle (A cues B, B cues A) must not loop, and must fire each ask
// exactly once.
func TestCascadeCycleFiresEachOnce(t *testing.T) {
	a := parseOne(t, "```ask\nname: a\ncues: b\n\nA.\n```\n")
	b := parseOne(t, "```ask\nname: b\ncues: a\n\nB.\n```\n")
	asks := []*Ask{a, b}
	direct := []CascadeHit{{Ask: a}}

	out := Cascade(asks, direct)
	if len(out) != 2 {
		t.Fatalf("want exactly 2 hits from a cycle, got %d: %v", len(out), out)
	}
}

// A diamond (A and B both cue C) must fire C exactly once, not twice.
func TestCascadeDiamondFiresTargetOnce(t *testing.T) {
	c := parseOne(t, "```ask\nname: c\n\nC.\n```\n")
	a := parseOne(t, "```ask\ncues: c\n\nA.\n```\n")
	b := parseOne(t, "```ask\ncues: c\n\nB.\n```\n")
	asks := []*Ask{a, b, c}
	direct := []CascadeHit{{Ask: a}, {Ask: b}}

	out := Cascade(asks, direct)
	if len(out) != 3 {
		t.Fatalf("want 3 hits (2 direct + C once), got %d: %v", len(out), out)
	}
}

// A cued ask naming an uninstalled tool still shouldn't inject — requires:
// is the one gate a cued firing does not bypass.
func TestCascadeSkipsCuedTargetMissingRequires(t *testing.T) {
	target := parseOne(t, "```ask\nname: needs-tool\nrequires: definitely-not-a-real-binary-xyz\n\nCued.\n```\n")
	citer := parseOne(t, "```ask\ncues: needs-tool\n\nCiter.\n```\n")
	asks := []*Ask{citer, target}
	direct := []CascadeHit{{Ask: citer}}

	out := Cascade(asks, direct)
	if len(out) != 1 {
		t.Errorf("want only the direct hit, cued target should be skipped for missing requires:, got %v", out)
	}
}

// A cues: value naming nothing in asks contributes nothing — the same
// fail-soft shape as an unwarmed evokes: phrase or a missing requires:
// binary, never an error at match time.
func TestCascadeSilentOnDanglingCue(t *testing.T) {
	citer := parseOne(t, "```ask\ncues: nowhere\n\nCiter.\n```\n")
	asks := []*Ask{citer}
	direct := []CascadeHit{{Ask: citer}}

	out := Cascade(asks, direct)
	if len(out) != 1 {
		t.Errorf("want only the direct hit for a dangling cue, got %v", out)
	}
}
