package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/justinstimatze/onsetter/ask"
	"github.com/justinstimatze/onsetter/internal/discover"
	"github.com/justinstimatze/onsetter/internal/embed"
)

// cmdCalib is what onsetter replay is for every regex header, for evokes:.
// A regex's own match/no-match is its own ground truth, so replay needs no
// labels — a fuzzy trigger's "should this fire" is not self-evident from the
// score alone, so calib needs an author-supplied positive and negative
// example set instead of a bare corpus.
//
// The lesson this exists to act on: a threshold or prefix choice measured
// against a handful of hand-picked pairs does not safely generalize, even to
// an adjacent problem on the same model — lexicon's own held-out calibration
// corpus found the opposite result from onsetter's for a structurally
// different matching regime. calib is the tool an evokes: author reaches
// for instead of repeating that hand-measurement by hand.
func cmdCalib(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("calib needs a target and two globs: `onsetter calib CLAUDE.md:12 'fires/**' 'not/**'`")
	}
	target, err := resolveCalibTarget(args[0])
	if err != nil {
		return err
	}

	cache := embed.OpenCache()
	var unwarmed []string
	for _, p := range target.Evokes {
		if !cache.Warm(p) {
			unwarmed = append(unwarmed, p)
		}
	}
	if len(unwarmed) > 0 {
		return fmt.Errorf("%d evokes: phrase(s) not warmed yet — run `onsetter warm` first: %s",
			len(unwarmed), strings.Join(unwarmed, "; "))
	}

	positive, err := scoreExamples(cache, target, args[1])
	if err != nil {
		return fmt.Errorf("scoring positive examples: %w", err)
	}
	negative, err := scoreExamples(cache, target, args[2])
	if err != nil {
		return fmt.Errorf("scoring negative examples: %w", err)
	}
	if len(positive) == 0 {
		return fmt.Errorf("%q matched no files — calib needs at least one positive example", args[1])
	}
	if len(negative) == 0 {
		return fmt.Errorf("%q matched no files — calib needs at least one negative example", args[2])
	}

	base, _ := os.Getwd()
	fmt.Printf("%s\n", target.Where(base))
	printCalibReport(calibReport{positive: positive, negative: negative, threshold: embed.DefaultThreshold})
	return nil
}

// resolveCalibTarget parses a locator (a CLAUDE.md path, optionally
// `:line` when the file has more than one evokes: ask) into the specific
// Ask calib measures. The path:line shape is not new syntax — it is exactly
// what Ask.Where already renders, so an ambiguity error can print locators
// the caller pastes straight back in.
func resolveCalibTarget(locator string) (*ask.Ask, error) {
	path := locator
	wantLine, hasLine := 0, false
	if i := strings.LastIndex(locator, ":"); i >= 0 {
		if n, err := strconv.Atoi(locator[i+1:]); err == nil {
			path, wantLine, hasLine = locator[:i], n, true
		}
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	asks, err := discover.ParseSource(abs)
	if err != nil {
		return nil, err
	}
	var withEvokes []*ask.Ask
	for _, a := range asks {
		if len(a.Evokes) > 0 {
			withEvokes = append(withEvokes, a)
		}
	}
	switch {
	case len(withEvokes) == 0:
		return nil, fmt.Errorf("no evokes: ask in %s", path)
	case hasLine:
		for _, a := range withEvokes {
			if a.Line == wantLine {
				return a, nil
			}
		}
		return nil, fmt.Errorf("no evokes: ask at %s:%d", path, wantLine)
	case len(withEvokes) == 1:
		return withEvokes[0], nil
	default:
		base, _ := os.Getwd()
		var locs []string
		for _, a := range withEvokes {
			locs = append(locs, a.Where(base))
		}
		return nil, fmt.Errorf("%d evokes: asks in %s — name one: %s",
			len(withEvokes), path, strings.Join(locs, ", "))
	}
}

// scoreExamples embeds every file glob matches and scores it against
// target's best-matching evokes: phrase — the same OR Match itself applies,
// so the reported score is the number that actually decides whether the ask
// would fire on that file.
func scoreExamples(cache *embed.Cache, target *ask.Ask, glob string) ([]scored, error) {
	files, err := expand([]string{glob})
	if err != nil {
		return nil, err
	}
	var out []scored
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		vec, err := embed.EmbedDocument(string(content), interactiveBudget)
		if err != nil {
			return nil, fmt.Errorf("embedding %s: %w", f, err)
		}
		best := float32(-1)
		for _, phrase := range target.Evokes {
			if s, ok := cache.ScoreQuery(phrase, vec); ok && s > best {
				best = s
			}
		}
		out = append(out, scored{path: shortOne(f), score: best})
	}
	return out, nil
}

type scored struct {
	path  string
	score float32
}

type calibReport struct {
	positive, negative []scored
	threshold          float32
}

func (r calibReport) posFloor() scored {
	min := r.positive[0]
	for _, s := range r.positive[1:] {
		if s.score < min.score {
			min = s
		}
	}
	return min
}

func (r calibReport) negCeiling() scored {
	max := r.negative[0]
	for _, s := range r.negative[1:] {
		if s.score > max.score {
			max = s
		}
	}
	return max
}

func (r calibReport) recall() (fired, total int) {
	for _, s := range r.positive {
		total++
		if s.score >= r.threshold {
			fired++
		}
	}
	return
}

func (r calibReport) falseFires() (fired, total int) {
	for _, s := range r.negative {
		total++
		if s.score >= r.threshold {
			fired++
		}
	}
	return
}

// printCalibReport is separated from cmdCalib's I/O so the report itself —
// the floor/ceiling/gap arithmetic and how it reads — is testable without a
// real embed call.
func printCalibReport(r calibReport) {
	pos := append([]scored(nil), r.positive...)
	sort.Slice(pos, func(i, j int) bool { return pos[i].score < pos[j].score })
	neg := append([]scored(nil), r.negative...)
	sort.Slice(neg, func(i, j int) bool { return neg[i].score > neg[j].score })

	fmt.Printf("%d positive example(s), %d negative example(s), threshold %.2f\n\n",
		len(pos), len(neg), r.threshold)

	fmt.Println("positive scores (weakest first):")
	for _, s := range pos {
		mark := ""
		if s.score < r.threshold {
			mark = "   ← below threshold"
		}
		fmt.Printf("    %-40s %.3f%s\n", s.path, s.score, mark)
	}
	fmt.Println("\nnegative scores (strongest first):")
	for _, s := range neg {
		mark := ""
		if s.score >= r.threshold {
			mark = "   ← fires (false positive)"
		}
		fmt.Printf("    %-40s %.3f%s\n", s.path, s.score, mark)
	}

	floor, ceil := r.posFloor(), r.negCeiling()
	fmt.Printf("\nPOS floor %.3f (%s)   NEG ceiling %.3f (%s)\n",
		floor.score, floor.path, ceil.score, ceil.path)
	if floor.score > ceil.score {
		fmt.Printf("gap +%.3f — a threshold between these separates every example given\n", floor.score-ceil.score)
	} else {
		fmt.Printf("overlap %.3f — no single threshold separates every example given;\n"+
			"the evokes: phrases or the examples themselves need rework, not just a number\n",
			ceil.score-floor.score)
	}

	firedPos, totalPos := r.recall()
	firedNeg, totalNeg := r.falseFires()
	fmt.Printf("\nat threshold %.2f: %d/%d positive(s) fire, %d/%d negative(s) false-fire\n",
		r.threshold, firedPos, totalPos, firedNeg, totalNeg)
}
