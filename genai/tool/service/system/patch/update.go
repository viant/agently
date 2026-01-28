package patch

import (
	"fmt"
	"sort"
	"strings"
)

type replacement struct {
	start    int
	oldLen   int
	newLines []string
}

// applyUpdate applies an UpdateFile patch to oldData and returns the new file
// contents. It uses sequential matching, strict context validation, and
// consistent newline handling.
func (s *Session) applyUpdate(oldData []byte, h UpdateFile, path string) (string, error) {
	originalLines := strings.Split(string(oldData), "\n")
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}

	replacements, err := computeReplacements(originalLines, path, h.Chunks)
	if err != nil {
		return "", err
	}

	newLines := applyReplacements(originalLines, replacements)
	if len(newLines) == 0 || newLines[len(newLines)-1] != "" {
		newLines = append(newLines, "")
	}
	return strings.Join(newLines, "\n"), nil
}

func computeReplacements(lines []string, path string, chunks []UpdateChunk) ([]replacement, error) {
	var reps []replacement
	lineIndex := 0

	for _, chunk := range chunks {
		if chunk.ChangeContext != "" {
			idx := seekSequence(lines, []string{chunk.ChangeContext}, lineIndex, false)
			if idx < 0 {
				return nil, fmt.Errorf("failed to find context %q in %s", chunk.ChangeContext, path)
			}
			lineIndex = idx + 1
		}

		if len(chunk.OldLines) == 0 {
			insertionIdx := len(lines)
			if len(lines) > 0 && lines[len(lines)-1] == "" {
				insertionIdx = len(lines) - 1
			}
			reps = append(reps, replacement{
				start:    insertionIdx,
				oldLen:   0,
				newLines: append([]string{}, chunk.NewLines...),
			})
			continue
		}

		pattern := chunk.OldLines
		newSlice := chunk.NewLines
		found := seekSequence(lines, pattern, lineIndex, chunk.IsEOF)

		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newSlice) > 0 && newSlice[len(newSlice)-1] == "" {
				newSlice = newSlice[:len(newSlice)-1]
			}
			found = seekSequence(lines, pattern, lineIndex, chunk.IsEOF)
		}

		if found < 0 {
			return nil, fmt.Errorf("failed to find expected lines in %s:\n%s", path, strings.Join(chunk.OldLines, "\n"))
		}

		reps = append(reps, replacement{
			start:    found,
			oldLen:   len(pattern),
			newLines: append([]string{}, newSlice...),
		})
		lineIndex = found + len(pattern)
	}

	sort.Slice(reps, func(i, j int) bool {
		return reps[i].start < reps[j].start
	})
	return reps, nil
}

func applyReplacements(lines []string, reps []replacement) []string {
	out := append([]string{}, lines...)
	for i := len(reps) - 1; i >= 0; i-- {
		r := reps[i]
		if r.start < 0 {
			continue
		}
		end := r.start + r.oldLen
		if end > len(out) {
			end = len(out)
		}
		segment := append([]string{}, r.newLines...)
		out = append(out[:r.start], append(segment, out[end:]...)...)
	}
	return out
}
