package main

import (
	"fmt"
	"time"

	"github.com/justinstimatze/onsetter/internal/discover"
	"github.com/justinstimatze/onsetter/internal/embed"
)

// warmBudget is generous compared to embed.DefaultBudget: this runs offline,
// once, with a human waiting on purpose — not per edit, with a tool call
// blocked behind it.
const warmBudget = 5 * time.Second

// cmdWarm builds the evokes: cache: every distinct phrase under dir, embedded
// and saved. `onsetter hook` never calls Ollama to fill a cache miss — a
// phrase added since the last warm rejects silently, same as every other
// fail-soft gate here, until this runs again.
func cmdWarm(args []string) error {
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}
	sources, err := discover.Sources(dir)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	var phrases []string
	for _, src := range sources {
		asks, _ := discover.ParseSource(src) // a bad block is lint's job, not warm's
		for _, r := range asks {
			for _, p := range r.Evokes {
				if !seen[p] {
					seen[p] = true
					phrases = append(phrases, p)
				}
			}
		}
	}
	if len(phrases) == 0 {
		fmt.Println("onsetter warm: no evokes: phrases found under", dir)
		return nil
	}

	cache := embed.OpenCache()
	var built, warm, failed int
	for _, p := range phrases {
		if cache.Warm(p) {
			warm++
			continue
		}
		if err := cache.WarmQuery(p, warmBudget); err != nil {
			failed++
			fmt.Printf("onsetter warm: %q: %v\n", p, err)
			continue
		}
		built++
	}
	if built > 0 {
		if err := cache.Save(); err != nil {
			return fmt.Errorf("saving cache: %w", err)
		}
	}
	fmt.Printf("onsetter warm: %d built, %d already warm, %d failed (%d phrase(s) total)\n",
		built, warm, failed, len(phrases))
	if failed > 0 {
		return fmt.Errorf("%d phrase(s) failed to embed — is Ollama running with %s pulled?", failed, embed.DefaultModel)
	}
	return nil
}
