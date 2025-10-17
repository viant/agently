package patch

import "strings"

//-----------------------------------------------------------------------------
// Helpers
//-----------------------------------------------------------------------------

// normalise removes all whitespace characters to make comparison whitespace count insensitive.
func normalise(s string) string {
	// Remove all whitespace characters (spaces, tabs, carriage returns)
	return strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), "\t", ""), "\r", "")
}

// rebuildNorm regenerates the canonical slice after oldLines has changed.
func rebuildNorm(lines []string) []string {
	n := make([]string, len(lines))
	for i, l := range lines {
		n[i] = normalise(l)
	}
	return n
}

// findSubSlice returns the first index of needle inside haystack (both
// canonicalised), or –1 if not found.
func findSubSlice(hay, need []string) int {
Outer:
	for i := 0; i <= len(hay)-len(need); i++ {
		for j := range need {
			if hay[i+j] != need[j] {
				continue Outer
			}
		}
		return i
	}
	return -1
}

// findFirstMatch returns the first index in hay whose canonical form matches
// any element of targets.  If none match it returns –1.
func findFirstMatch(hay, targets []string) int {
	set := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		set[t] = struct{}{}
	}
	for i, h := range hay {
		if _, ok := set[h]; ok {
			return i
		}
	}
	return -1
}

// replaceSlice replaces lenOld lines of src starting at start with repl.
func replaceSlice(src []string, start, lenOld int, repl []string) []string {
	out := append([]string{}, src[:start]...)
	out = append(out, repl...)
	return append(out, src[start+lenOld:]...)
}

//-----------------------------------------------------------------------------
// Core patch routine
//-----------------------------------------------------------------------------

// applyUpdate applies an UpdateFile patch to oldData and returns the new file
// as a slice of lines.  All comparisons are whitespace-insensitive.  No language
// -specific heuristics are used.
func (s *Session) applyUpdate(oldData []byte, h UpdateFile) []string {
	// 1. Split original file into lines and create canonical view.
	oldLines := strings.Split(strings.TrimRight(string(oldData), "\n"), "\n")
	normOld := rebuildNorm(oldLines)

	// 2. Process each chunk in order.
	for _, chunk := range h.Chunks {
		// Canonicalise chunk once.
		normOldChunk := make([]string, len(chunk.OldLines))
		for i, l := range chunk.OldLines {
			normOldChunk[i] = normalise(l)
		}
		normNewChunk := make([]string, len(chunk.NewLines))
		for i, l := range chunk.NewLines {
			normNewChunk[i] = normalise(l)
		}

		//---------------------------------------------------------------------
		// 2-A. Exact (canonical) match — fast path
		//---------------------------------------------------------------------
		if start := findSubSlice(normOld, normOldChunk); start >= 0 {
			oldLines = replaceSlice(oldLines, start, len(chunk.OldLines), chunk.NewLines)
			normOld = rebuildNorm(oldLines)
			continue
		}

		//---------------------------------------------------------------------
		// 2-B. Fuzzy anchor on first matching line
		//---------------------------------------------------------------------
		if anchor := findFirstMatch(normOld, normOldChunk); anchor >= 0 {
			// Build a set of canonical forms to be removed from the file.
			remove := make(map[string]struct{}, len(normOldChunk))
			for _, n := range normOldChunk {
				remove[n] = struct{}{}
			}

			var tmp []string
			for i, line := range oldLines {
				if _, drop := remove[normOld[i]]; drop {
					// Insert the new chunk only once, when we hit the anchor.
					if i == anchor {
						tmp = append(tmp, chunk.NewLines...)
					}
					continue // skip this (old) line
				}
				tmp = append(tmp, line)
			}
			oldLines = tmp
			normOld = rebuildNorm(oldLines)
			continue
		}

		//---------------------------------------------------------------------
		// 2-C. Could not locate chunk — skip to preserve file integrity
		//---------------------------------------------------------------------
	}

	return oldLines
}
