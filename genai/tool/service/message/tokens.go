package message

// estimateTokens provides a simple character-based token estimate heuristic.
func estimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len(s)
	if n < 8 {
		return 1
	}
	return (n + 3) / 4
}
