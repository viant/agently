package patch

import "strings"

// seekSequence finds pattern in lines starting at start. Matching steps: exact,
// trim-end, trim-both, then normalized punctuation. Returns -1 if not found.
func seekSequence(lines []string, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}

	searchStart := start
	if eof && len(lines) >= len(pattern) {
		searchStart = len(lines) - len(pattern)
	}

	maxStart := len(lines) - len(pattern)
	if maxStart < 0 {
		return -1
	}

	// Exact match.
	for i := searchStart; i <= maxStart; i++ {
		if equalSlice(lines[i:i+len(pattern)], pattern) {
			return i
		}
	}

	// Trim-end match.
	for i := searchStart; i <= maxStart; i++ {
		ok := true
		for j, pat := range pattern {
			if strings.TrimRight(lines[i+j], " \t\r\n") != strings.TrimRight(pat, " \t\r\n") {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}

	// Trim both sides match.
	for i := searchStart; i <= maxStart; i++ {
		ok := true
		for j, pat := range pattern {
			if strings.TrimSpace(lines[i+j]) != strings.TrimSpace(pat) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}

	// Normalized punctuation match (best-effort).
	for i := searchStart; i <= maxStart; i++ {
		ok := true
		for j, pat := range pattern {
			if normalizePunctuation(lines[i+j]) != normalizePunctuation(pat) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}

	return -1
}

func equalSlice(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizePunctuation(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.TrimSpace(s) {
		switch r {
		case '\u2010', '\u2011', '\u2012', '\u2013', '\u2014', '\u2015', '\u2212':
			b.WriteRune('-')
		case '\u2018', '\u2019', '\u201A', '\u201B':
			b.WriteRune('\'')
		case '\u201C', '\u201D', '\u201E', '\u201F':
			b.WriteRune('"')
		case '\u00A0', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A', '\u202F', '\u205F', '\u3000':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
