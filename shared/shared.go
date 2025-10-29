package shared

// RuneTruncate safely truncates a UTF-8 string to at most n runes without
// splitting a multi-byte character. When n <= 0 it returns an empty string.
func RuneTruncate(s string, n int) string {
	if n <= 0 || s == "" {
		return ""
	}
	i := 0
	for idx := range s { // idx iterates over valid rune boundaries
		if i == n {
			return s[:idx]
		}
		i++
	}
	return s
}
