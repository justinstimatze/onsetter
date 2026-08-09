package ask

import (
	"path/filepath"
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

// Every header the parser accepts is in Headers, so the parse error, the
// reference and replay's funnel cannot fall out of step with the switch.
func TestHeadersListsEveryHeaderTheParserAccepts(t *testing.T) {
	for _, h := range Headers {
		v := "x"
		if h == "on" {
			v = "mint"
		}
		if _, err := Parse(strings.NewReader("```ask\n"+h+": "+v+"\n\nAsk.\n```\n"), "/repo/CLAUDE.md"); err != nil {
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
