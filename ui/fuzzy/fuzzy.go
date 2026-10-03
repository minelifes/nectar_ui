// Package fuzzy matches a typed pattern against names the way command
// palettes and "go to file" boxes do: the pattern's characters must appear
// in order (not necessarily together), and matches at word starts, in a
// row, and with the same case rank higher. "gtf" finds "Go To File",
// "fsave" finds "File: Save".
package fuzzy

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

// Score points.
const (
	scoreMatch       = 16
	bonusConsecutive = 24
	bonusWordStart   = 32
	bonusFirstChar   = 16
	bonusCase        = 2
	penaltyGap       = 3 // per skipped rune (capped)
	penaltyLeading   = 1 // per rune before the first match (capped)
)

// Match is one item that matched.
type Match struct {
	Index     int   // index in the items given to Filter
	Score     int   // higher is better
	Positions []int // byte offsets of the matched runes in the item
}

// Score matches pattern against s. ok is false if the pattern's runes don't
// all appear in s in order (an empty pattern matches everything with score
// 0). Matching ignores case; spaces in the pattern are ignored.
func Score(pattern, s string) (score int, positions []int, ok bool) {
	p := make([]rune, 0, len(pattern))
	for _, r := range pattern {
		if !unicode.IsSpace(r) {
			p = append(p, r)
		}
	}
	if len(p) == 0 {
		return 0, nil, true
	}
	runes := make([]rune, 0, len(s))
	offs := make([]int, 0, len(s))
	for i, r := range s {
		runes = append(runes, r)
		offs = append(offs, i)
	}
	n, m := len(runes), len(p)
	if m > n {
		return 0, nil, false
	}
	// Quick reject: a subsequence must exist.
	j := 0
	for i := 0; i < n && j < m; i++ {
		if fold(runes[i]) == fold(p[j]) {
			j++
		}
	}
	if j < m {
		return 0, nil, false
	}

	bonus := make([]int, n)
	for i := range runes {
		bonus[i] = boundaryBonus(runes, i)
	}
	// best[j][i]: best score with p[:j+1] matched and p[j] at runes[i].
	const neg = -1 << 30
	best := make([][]int, m)
	from := make([][]int, m)
	for j := range best {
		best[j] = make([]int, n)
		from[j] = make([]int, n)
		for i := range best[j] {
			best[j][i] = neg
		}
	}
	for j := 0; j < m; j++ {
		// Gaps longer than 10 runes cost the same, so the best start
		// further back than that is a running maximum (keeps this
		// O(len(pattern)·len(s))).
		farBest, farArg := neg, -1
		for i := j; i < n; i++ {
			if j > 0 {
				if k := i - 12; k >= j-1 && best[j-1][k] > farBest {
					farBest, farArg = best[j-1][k], k
				}
			}
			if fold(runes[i]) != fold(p[j]) {
				continue
			}
			gain := scoreMatch + bonus[i]
			if runes[i] == p[j] {
				gain += bonusCase
			}
			if j == 0 {
				lead := min(i, 8) * penaltyLeading
				if i == 0 {
					gain += bonusFirstChar
				}
				best[j][i] = gain - lead
				from[j][i] = -1
				continue
			}
			if farArg >= 0 {
				best[j][i] = farBest + gain - 10*penaltyGap
				from[j][i] = farArg
			}
			for k := max(j-1, i-11); k < i; k++ {
				if best[j-1][k] == neg {
					continue
				}
				v := best[j-1][k] + gain
				if k == i-1 {
					v += bonusConsecutive
				} else {
					v -= min(i-k-1, 10) * penaltyGap
				}
				if v > best[j][i] {
					best[j][i] = v
					from[j][i] = k
				}
			}
		}
	}
	end, top := -1, neg
	for i := 0; i < n; i++ {
		if best[m-1][i] > top {
			top, end = best[m-1][i], i
		}
	}
	if end < 0 {
		return 0, nil, false
	}
	positions = make([]int, m)
	for j, i := m-1, end; j >= 0; j-- {
		positions[j] = offs[i]
		i = from[j][i]
	}
	// Prefer shorter items when everything else is equal.
	return top - min(n-m, 32)/4, positions, true
}

func fold(r rune) rune { return unicode.ToLower(r) }

// boundaryBonus rewards runes that start a word: after a separator, an
// uppercase letter after a lowercase one (camelCase), a letter after a digit.
func boundaryBonus(rs []rune, i int) int {
	if i == 0 {
		return bonusWordStart
	}
	prev, cur := rs[i-1], rs[i]
	switch {
	case isSep(prev) && !isSep(cur):
		return bonusWordStart
	case unicode.IsLower(prev) && unicode.IsUpper(cur):
		return bonusWordStart
	case unicode.IsDigit(prev) != unicode.IsDigit(cur) && unicode.IsLetter(cur):
		return bonusWordStart / 2
	}
	return 0
}

func isSep(r rune) bool {
	switch r {
	case ' ', '_', '-', '.', '/', '\\', ':', '(', ')', '[', ']', ',':
		return true
	}
	return unicode.IsSpace(r)
}

// Filter returns the items that match pattern, best first (ties keep the
// items' order). An empty pattern returns every item in order.
func Filter(pattern string, items []string) []Match {
	out := make([]Match, 0, len(items))
	for i, s := range items {
		if sc, pos, ok := Score(pattern, s); ok {
			out = append(out, Match{Index: i, Score: sc, Positions: pos})
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Score > out[b].Score })
	return out
}

// Ranges merges matched positions into byte ranges [start, end) of s, for
// highlighting.
func Ranges(s string, positions []int) [][2]int {
	var out [][2]int
	for _, p := range positions {
		if p < 0 || p >= len(s) {
			continue
		}
		_, w := utf8.DecodeRuneInString(s[p:])
		if n := len(out); n > 0 && out[n-1][1] == p {
			out[n-1][1] = p + w
			continue
		}
		out = append(out, [2]int{p, p + w})
	}
	return out
}
