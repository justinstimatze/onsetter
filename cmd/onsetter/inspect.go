package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/discover"
	"github.com/justinstimatze/onsetter/internal/embed"
)

// interactiveBudget is the embed-call ceiling for list and replay: both are
// run by a human waiting on purpose, not a blocked tool call, so this can sit
// well above embed.DefaultBudget the way lexicon's own UserPromptSubmit-cadence
// call does relative to its per-edit one.
const interactiveBudget = 3 * time.Second

// cmdList answers "what is watching this file, and why". Without it, an ask
// that fires from three directories up is a question with no visible author.
func cmdList(args []string) error {
	target := "."
	if len(args) > 0 {
		target = args[0]
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	asks, errs := discover.Asks(abs)
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", e)
	}
	if len(asks) == 0 {
		fmt.Printf("No asks govern %s.\n", target)
		fmt.Printf("Searched: %s\n", strings.Join(short(discover.Roots(abs)), ", "))
		return nil
	}

	content, readErr := os.ReadFile(abs)
	exists := readErr == nil
	base, _ := os.Getwd()

	var evokes func(string) bool
	for _, r := range asks {
		if len(r.Evokes) > 0 {
			evokes = embed.BuildPredicate(string(content), interactiveBudget)
			break
		}
	}

	// Match once per ask up front, not inside the render loop below: a
	// cue-only ask (in: nothing/**, say) always rejects its own gate, and
	// the render loop needs to know it was reached anyway, by whatever ask
	// cued it, before it gets to that ask's own turn.
	//
	// Two synthetic edits, not one: an on: read ask would always report
	// "turned away at on: read" against the write-shaped edit below, which
	// is accurate for the write case and actively misleading for what an
	// on: read ask's own author wants to know. New is "" on the read edit,
	// matching a real Read call, which never carries pending content.
	writeEdit := ask.Edit{Path: abs, New: string(content), Disk: string(content), Exists: exists, Evokes: evokes}
	readEdit := writeEdit
	readEdit.New = ""
	readEdit.IsRead = true

	results := make(map[*ask.Ask]ask.Result, len(asks))
	var direct []ask.CascadeHit
	for _, r := range asks {
		e := writeEdit
		if r.On == ask.ModeRead {
			e = readEdit
		}
		res := r.Match(e)
		results[r] = res
		if res.OK {
			direct = append(direct, ask.CascadeHit{Ask: r, Matched: res.Matched})
		}
	}
	viaOf := map[string]*ask.Ask{}
	for _, h := range ask.Cascade(asks, direct) {
		if h.Via != nil {
			viaOf[h.Ask.ID()] = h.Via
		}
	}

	fmt.Printf("%d ask(s) govern %s\n", len(asks), target)
	for _, r := range asks {
		res := results[r]
		// The rejection is marked on the header that caused it rather than
		// stated below, because a verdict line has to repeat the pattern to be
		// useful and the pattern is already on screen. Every gate that can
		// reject is printed, so the marker always lands somewhere.
		mark := func(gate, pattern string) string {
			if res.OK || res.Gate != gate || res.Pattern != pattern {
				return ""
			}
			return "   ← " + res.Note
		}

		fmt.Printf("\n▸ %s\n", r.Where(base))
		for _, req := range r.Requires {
			fmt.Printf("    requires:  %s%s\n", req, mark("requires", req))
		}
		fmt.Printf("    in:        %s   (relative to %s)%s\n", r.In, shortOne(r.Dir), mark("in", r.In))
		for _, x := range r.NotIn {
			fmt.Printf("    not-in:    %s%s\n", x, mark("not-in", x))
		}
		if r.On != ask.ModeAny {
			fmt.Printf("    on:        %s%s\n", r.On, mark("on", string(r.On)))
		}
		for _, n := range r.Not {
			fmt.Printf("    not:       %s%s\n", n, mark("not", n.String()))
		}
		for _, h := range r.Has {
			fmt.Printf("    has:       %s%s\n", h, mark("has", h.String()))
		}
		for _, u := range r.Untouched {
			fmt.Printf("    untouched: %s   (no session here, so treated as met)%s\n", u, mark("untouched", u))
		}
		for _, a := range r.Added {
			fmt.Printf("    added:     %s%s\n", a, mark("added", a.String()))
		}
		for _, x := range r.Removed {
			fmt.Printf("    removed:   %s%s\n", x, mark("removed", x.String()))
		}
		for _, w := range r.When {
			fmt.Printf("    when:      %s%s\n", w, mark("when", w.String()))
		}
		// Not mark(): evokes: rejects when none of its phrases match, so there
		// is no single villain the way a failing when: or has: has one — every
		// line earns the same note, not just the one Result happens to name.
		evokesNote := ""
		if !res.OK && res.Gate == "evokes" {
			evokesNote = "   ← " + res.Note
		}
		for _, ev := range r.Evokes {
			fmt.Printf("    evokes:    %s%s\n", ev, evokesNote)
		}
		if r.Name != "" {
			fmt.Printf("    name:      %s\n", r.Name)
		}
		for _, c := range r.Cues {
			fmt.Printf("    cues:      %s\n", c)
		}
		switch {
		case res.OK && res.Matched == "":
			fmt.Printf("    → would fire (no content gate)\n")
		case res.OK:
			fmt.Printf("    → would fire, matching %q\n", res.Matched)
		case viaOf[r.ID()] != nil:
			fmt.Printf("    → would fire, cued by %s\n", viaOf[r.ID()].Where(base))
		default:
			fmt.Printf("    → would not fire · turned away at %s:\n", res.Gate)
		}
		fmt.Printf("    %s\n", indent(r.Body, "    "))
	}
	return nil
}

// cmdReplay measures what a gate costs before it is wired. Every first draft
// over-fires: one draft here fired on every file in its corpus, because it
// gated on the file carrying a citation, which is what every file in that
// corpus is. A rate catches that; reading the regex never does.
//
// It cannot measure what a gate catches. The corpus has been cleaned of
// exactly the defect the ask looks for, so a healthy ask and a dead one both
// read as zero. That is a property of the corpus, not a gap to close here.
func cmdReplay(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("replay needs at least one path or glob, e.g. `onsetter replay 'corpus/**/*.md'`")
	}
	files, err := expand(args)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files matched %s", strings.Join(args, " "))
	}

	type stat struct {
		r        *ask.Ask
		eligible int
		fired    int
		cued     int // of fired, how many were cued rather than gate-matched
		samples  []string
		// turned is how many files each gate rejected, which is the only way to
		// tell a dead regex from a clean corpus: both read as 0%, but a dead
		// `when:` shows the files that reached it and a clean corpus does not.
		turned map[string]int
		reason map[string]string
	}
	stats := map[string]*stat{}
	var order []string
	base, _ := os.Getwd()

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		asks, _ := discover.Asks(f)
		var evokes func(string) bool
		for _, r := range asks {
			if len(r.Evokes) > 0 {
				evokes = embed.BuildPredicate(string(content), interactiveBudget)
				break
			}
		}
		// Match every ask first, so a cue-only ask's own (rejecting) result
		// can be overridden by Cascade before any stat is touched — same
		// two-pass shape cmdList uses, for the same reason: an ask reached
		// only by being cued always fails its own gate on its own.
		type outcome struct {
			res ask.Result
			via *ask.Ask
		}
		results := make(map[*ask.Ask]outcome, len(asks))
		var direct []ask.CascadeHit
		for _, r := range asks {
			exists := r.On != ask.ModeMint
			e := ask.Edit{Path: f, New: string(content), Disk: string(content), Exists: exists, Evokes: evokes}
			// Same reasoning as cmdList: an on: read ask needs the
			// Read-shaped edit or it always reports "turned away at on:
			// read" against this write-shaped one.
			if r.On == ask.ModeRead {
				e.New = ""
				e.IsRead = true
			}
			res := r.Match(e)
			results[r] = outcome{res: res}
			if res.OK {
				direct = append(direct, ask.CascadeHit{Ask: r, Matched: res.Matched})
			}
		}
		for _, h := range ask.Cascade(asks, direct) {
			if h.Via != nil {
				results[h.Ask] = outcome{res: results[h.Ask].res, via: h.Via}
			}
		}

		for _, r := range asks {
			id := r.ID()
			s, ok := stats[id]
			if !ok {
				s = &stat{r: r, turned: map[string]int{}, reason: map[string]string{}}
				stats[id] = s
				order = append(order, id)
			}
			s.eligible++
			o := results[r]
			switch {
			case o.res.OK:
				s.fired++
				if len(s.samples) < 3 {
					s.samples = append(s.samples, fmt.Sprintf("%s%s", shortOne(f), quoted(o.res.Matched)))
				}
			case o.via != nil:
				s.fired++
				s.cued++
				if len(s.samples) < 3 {
					s.samples = append(s.samples, fmt.Sprintf("%s (cued by %s)", shortOne(f), o.via.Where(base)))
				}
			default:
				s.turned[o.res.Gate]++
				s.reason[o.res.Gate] = o.res.Pattern
			}
		}
	}

	fmt.Printf("Replayed %d file(s).\n\n", len(files))
	sort.SliceStable(order, func(i, j int) bool {
		return rate(stats[order[i]].fired, stats[order[i]].eligible) >
			rate(stats[order[j]].fired, stats[order[j]].eligible)
	})
	for _, id := range order {
		s := stats[id]
		switch {
		// A mint ask with no content gate fires on every mint by
		// construction. Printing 100% next to the advice below would read as
		// a broken ask; the real denominator is mints, which replay has no
		// way to count.
		case s.r.On == ask.ModeMint && len(s.r.When) == 0:
			fmt.Printf("%-34s %s\n", s.r.Where(base), "every mint  (no rate to measure)")
			warnBlindToDiff(s.r)
			continue
		case s.r.On == ask.ModeMint:
			fmt.Printf("%-34s %5d/%-5d  %5.1f%%  [of mints, simulated]\n",
				s.r.Where(base), s.fired, s.eligible, rate(s.fired, s.eligible)*100)
		default:
			fmt.Printf("%-34s %5d/%-5d  %5.1f%%\n",
				s.r.Where(base), s.fired, s.eligible, rate(s.fired, s.eligible)*100)
		}
		warnBlindToDiff(s.r)
		if s.cued > 0 {
			fmt.Printf("    %d of those %d fire(s) were cued, not gate-matched\n", s.cued, s.fired)
		}
		for _, ex := range s.samples {
			fmt.Printf("    %s\n", ex)
		}
		if s.fired == 0 && s.eligible > 0 {
			fmt.Printf("    turned away at  %s\n", funnel(s.turned, s.reason))
		}
	}
	fmt.Printf("\nA gate tripping on more than a few percent of what it matches is a tax.\nNarrow it, or move the ask closer to the files it is about.\n")
	return nil
}

// cmdLint parses every block under dir. An ask that fails to parse is worse
// than no ask, because the author believes it is watching.
func cmdLint(args []string) error {
	root := "."
	if len(args) > 0 {
		root = args[0]
	}
	sources, walkErr := discover.Sources(root)
	if walkErr != nil {
		return walkErr
	}

	bad := 0
	total := 0
	var all []*ask.Ask
	for _, src := range sources {
		asks, err := discover.ParseSource(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			bad++
		}
		total += len(asks)
		all = append(all, asks...)
		for _, r := range asks {
			// A `not-in: **` ask can never fire on its own — the idiom for
			// prose meant only to be reached by cues: — so it is narrowed
			// enough not to be a banner even with no content gate. A merely
			// non-empty NotIn is not the same claim: `not-in: vendor/**`
			// still fires on everything else, so only the exact "**"
			// exemption applies.
			cueOnly := len(r.NotIn) == 1 && r.NotIn[0] == "**"
			if !r.Gated() && !cueOnly && (r.In == "**" || r.In == "**/*") && r.On == ask.ModeAny {
				fmt.Fprintf(os.Stderr,
					"%s: fires on every edit below %s with no content gate — that is a banner\n",
					r.Where(root), shortOne(r.Dir))
				bad++
			}
			// A Read call never carries incoming text, so when:/added:/
			// removed:/not:/evokes: — every gate that reads content, not
			// disk — can never match on: read. An ask combining them is
			// dead on arrival, the same failure shape a banner is.
			if r.On == ask.ModeRead {
				var dead []string
				if len(r.When) > 0 {
					dead = append(dead, "when:")
				}
				if len(r.Added) > 0 {
					dead = append(dead, "added:")
				}
				if len(r.Removed) > 0 {
					dead = append(dead, "removed:")
				}
				if len(r.Not) > 0 {
					dead = append(dead, "not:")
				}
				if len(r.Evokes) > 0 {
					dead = append(dead, "evokes:")
				}
				if len(dead) > 0 {
					fmt.Fprintf(os.Stderr,
						"%s: on: read combined with %s can never fire — a Read call has no incoming text for any of these to match\n",
						r.Where(root), strings.Join(dead, ", "))
					bad++
				}
			}
			// revisit: true used to widen the session key on its own; every
			// matched ask does that unconditionally now, so the header no
			// longer changes anything for one. Flagged, not silently
			// ignored — an author reading this ask would otherwise believe
			// revisit: is still doing something.
			if r.Revisit && (len(r.When) > 0 || len(r.Added) > 0 || len(r.Removed) > 0 || len(r.Evokes) > 0) {
				fmt.Fprintf(os.Stderr,
					"%s: revisit: true is redundant — every matched ask always widens the session key now; safe to remove\n",
					r.Where(root))
				bad++
			}
		}
	}
	for _, p := range ask.ValidateCues(all) {
		switch p.Kind {
		case "duplicate-name":
			fmt.Fprintf(os.Stderr, "%s: name: %q is also declared by another ask — cues: naming it would silently reach whichever one ByName happens to keep\n", p.Ask.Where(root), p.Value)
		case "dangling-cue":
			fmt.Fprintf(os.Stderr, "%s: cues: %q names no ask anywhere under %s\n", p.Ask.Where(root), p.Value, shortOne(root))
		case "cue-out-of-scope":
			fmt.Fprintf(os.Stderr, "%s: cues: %q resolves, but to an ask declared below this one's own CLAUDE.md — it can only ever be reached at hook time for edits inside that subdirectory\n", p.Ask.Where(root), p.Value)
		}
		bad++
	}
	fmt.Printf("%d ask(s) in %d file(s).\n", total, len(sources))
	if bad > 0 {
		return fmt.Errorf("%d problem(s)", bad)
	}
	return nil
}

// warnBlindToDiff says so when the rate above it did not measure the ask.
//
// Replay builds a synthetic edit from a file on disk, which has no old text, so
// `added:` degrades to `when:` and `removed:` can never fire. The rate is then
// about the ask's other gates and says nothing about the one the author most
// wants checked. A silent wrong number is worse than an absent one: a real
// `added: "aliases"` printed 70.0% here, which is the rate its `in:` and the
// file contents produce and not a thing that will ever happen at an edit.
//
// The honest check is to drive `onsetter hook` with an old/new pair.
func warnBlindToDiff(r *ask.Ask) {
	var gates []string
	if len(r.Added) > 0 {
		gates = append(gates, "added:")
	}
	if len(r.Removed) > 0 {
		gates = append(gates, "removed:")
	}
	if len(gates) == 0 {
		return
	}
	fmt.Printf("    ! rate does not measure %s — replay has no old text, so\n",
		strings.Join(gates, " or "))
	fmt.Printf("      there is no diff to gate on. Drive `onsetter hook` to check it.\n")
}

// funnel renders where a never-firing ask lost its files, in the order Match
// applies the gates. Reading left to right, each count is what survived the
// gate before it: `in: ×38 · when: ×2` says the glob reached two files and the
// regex matched neither, which is a live ask over the wrong corpus. A lone
// `in: ×40` says the glob reached nothing, which is a different bug.
func funnel(turned map[string]int, pattern map[string]string) string {
	var parts []string
	for _, g := range ask.Headers {
		if n := turned[g]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s: %s ×%d", g, pattern[g], n))
		}
	}
	if len(parts) == 0 {
		return "nothing (it fired on none and was turned away by none)"
	}
	return strings.Join(parts, "  ·  ")
}

func rate(fired, eligible int) float64 {
	if eligible == 0 {
		return 0
	}
	return float64(fired) / float64(eligible)
}

func quoted(m string) string {
	if m == "" {
		return ""
	}
	return fmt.Sprintf("  %q", m)
}

// expand turns paths, directories and doublestar globs into a file list.
func expand(args []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			return
		}
		seen[abs] = true
		out = append(out, abs)
	}
	for _, a := range args {
		if st, err := os.Stat(a); err == nil {
			if !st.IsDir() {
				add(a)
				continue
			}
			err = filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return nil //nolint:nilerr
				}
				if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
					return filepath.SkipDir
				}
				if !d.IsDir() {
					add(p)
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		matches, err := doublestar.FilepathGlob(a)
		if err != nil {
			return nil, fmt.Errorf("bad glob %q: %w", a, err)
		}
		for _, m := range matches {
			if st, err := os.Stat(m); err == nil && !st.IsDir() {
				add(m)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

func short(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = shortOne(p)
	}
	return out
}

func shortOne(p string) string {
	if base, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(base, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func indent(s, pad string) string {
	return strings.ReplaceAll(s, "\n", "\n"+pad)
}
