package diagnostic

import "slices"

// Suggest returns up to maxResults names from candidates that are within
// edit distance threshold of target. Results are sorted by distance, then
// lexicographically.
func Suggest(target string, candidates []string, threshold int, maxResults int) []string {
	type hit struct {
		name string
		dist int
	}
	var hits []hit
	for _, c := range candidates {
		if c == target {
			continue
		}
		// Skip candidates that differ in length by more than threshold —
		// Levenshtein distance is at least |len(a)-len(b)|.
		if d := len(c) - len(target); d > threshold || d < -threshold {
			continue
		}
		if d := levenshtein(target, c); d <= threshold {
			hits = append(hits, hit{c, d})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int {
		if a.dist != b.dist {
			return a.dist - b.dist
		}
		if a.name < b.name {
			return -1
		}
		if a.name > b.name {
			return 1
		}
		return 0
	})
	out := make([]string, 0, maxResults)
	for i := 0; i < len(hits) && i < maxResults; i++ {
		out = append(out, hits[i].name)
	}
	return out
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	// Single-row DP.
	prev := make([]int, lb+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			ins := curr[j-1] + 1
			del := prev[j] + 1
			sub := prev[j-1] + cost
			curr[j] = min(ins, del, sub)
		}
		prev = curr
	}
	return prev[lb]
}
