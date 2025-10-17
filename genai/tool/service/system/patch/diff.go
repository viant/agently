package patch

// Diff generation helpers compatible with Codex CLI/Rust behaviour.

import (
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// DiffStats captures basic statistics about a unified-diff output.

// GenerateDiff produces a GNU unified diff between old and new file contents.
// It returns the diff string along with insertion/deletion statistics.
// If the two inputs are identical, an empty diff string is returned.
func GenerateDiff(oldContent, newContent []byte, filePath string, contextLines int) (string, DiffStats, error) {
	if contextLines <= 0 {
		contextLines = 3
	}

	if string(oldContent) == string(newContent) {
		return "", DiffStats{}, nil
	}

	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(oldContent)),
		B:        difflib.SplitLines(string(newContent)),
		FromFile: filePath + " (original)",
		ToFile:   filePath + " (modified)",
		Context:  contextLines,
	}

	patch, err := difflib.GetUnifiedDiffString(ud)
	if err != nil {
		return "", DiffStats{}, err
	}

	// compute stats
	var stats DiffStats
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			stats.Added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			stats.Removed++
		}
	}

	return patch, stats, nil
}
