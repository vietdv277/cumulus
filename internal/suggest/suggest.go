// Package suggest builds "did you mean" hints for names that failed a
// lookup: close matches from a candidate list, formatted for error messages.
package suggest

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Similar returns up to n candidates close to name, closest first.
//
// Candidates are compared case-insensitively; a candidate exactly equal to
// name is skipped and duplicates are dropped. A candidate qualifies when it
// is case-insensitively equal, within a Levenshtein distance threshold of
// name (1 when name is at most 3 runes, 2 when at most 7, else 3), or has
// name as a prefix (name at least 2 runes). A candidate containing ':' is
// also compared against the part after the last ':' and keeps the better
// score, so "prod" suggests "aws:prod".
//
// Order: case-insensitive equal first, then smaller distance, then prefix
// matches, then alphabetical.
func Similar(name string, candidates []string, n int) []string {
	if name == "" || n <= 0 {
		return nil
	}

	threshold := distanceThreshold(name)

	seen := make(map[string]bool, len(candidates))
	var matches []candScore
	for _, c := range candidates {
		if c == name || seen[c] {
			continue
		}
		seen[c] = true
		s := score(name, c)
		if s.eq || s.dist <= threshold || s.prefix {
			matches = append(matches, s)
		}
	}

	// insertion sort: suggestion lists are tiny (callers ask for 3)
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && ahead(matches[j], matches[j-1]); j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	if len(matches) == 0 {
		return nil
	}
	if len(matches) > n {
		matches = matches[:n]
	}
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.candidate
	}
	return names
}

// DidYouMean formats matches as `did you mean "a"?`, `did you mean "a" or "b"?`,
// `did you mean "a", "b" or "c"?`; it returns "" for no matches.
func DidYouMean(matches []string) string {
	if len(matches) == 0 {
		return ""
	}
	if len(matches) == 1 {
		return fmt.Sprintf("did you mean %q?", matches[0])
	}
	quoted := make([]string, len(matches))
	for i, m := range matches {
		quoted[i] = fmt.Sprintf("%q", m)
	}
	return "did you mean " + strings.Join(quoted[:len(quoted)-1], ", ") +
		" or " + quoted[len(quoted)-1] + "?"
}

// candScore is one candidate's best comparison result against name.
type candScore struct {
	candidate string
	eq        bool // case-insensitively equal to name
	dist      int  // Levenshtein distance of the lower-cased strings
	prefix    bool // lower-cased candidate starts with the lower-cased name
}

// distanceThreshold is the Levenshtein cutoff that still counts as a typo.
func distanceThreshold(name string) int {
	switch r := utf8.RuneCountInString(name); {
	case r <= 3:
		return 1
	case r <= 7:
		return 2
	default:
		return 3
	}
}

// score compares name with one candidate, keeping the better of the direct
// comparison and the comparison against the part after the last ':' while
// always reporting the full candidate.
func score(name, candidate string) candScore {
	best := compare(name, candidate)
	if i := strings.LastIndex(candidate, ":"); i >= 0 {
		if s := compare(name, candidate[i+1:]); better(s, best) {
			s.candidate = candidate
			best = s
		}
	}
	return best
}

// compare scores name against one comparison string.
func compare(name, candidate string) candScore {
	ln := strings.ToLower(name)
	lc := strings.ToLower(candidate)
	return candScore{
		candidate: candidate,
		eq:        ln == lc,
		dist:      levenshtein([]rune(ln), []rune(lc)),
		prefix:    utf8.RuneCountInString(ln) >= 2 && strings.HasPrefix(lc, ln),
	}
}

// better reports whether score a ranks ahead of score b for the ':' rule,
// where both scores describe the same candidate.
func better(a, b candScore) bool {
	if a.eq != b.eq {
		return a.eq
	}
	if a.dist != b.dist {
		return a.dist < b.dist
	}
	return a.prefix && !b.prefix
}

// ahead orders two candidates: equal first, then smaller distance, then
// prefix matches, then alphabetical.
func ahead(a, b candScore) bool {
	if a.eq != b.eq {
		return a.eq
	}
	if a.dist != b.dist {
		return a.dist < b.dist
	}
	if a.prefix != b.prefix {
		return a.prefix
	}
	return a.candidate < b.candidate
}

// levenshtein returns the rune-based edit distance between a and b.
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(min(prev[j]+1, curr[j-1]+1), prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
