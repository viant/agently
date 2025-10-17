package patch

import "testing"

func TestGenerateDiff(t *testing.T) {
	oldText := "line1\nline2\nline3\n"
	newText := "line1\nline2 changed\nline3\n+added\n"

	diff, stats, err := GenerateDiff([]byte(oldText), []byte(newText), "sample.txt", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff == "" {
		t.Fatalf("expected diff, got empty string")
	}

	expectedAdded := 2 // modified + added
	expectedRemoved := 1

	if stats.Added != expectedAdded || stats.Removed != expectedRemoved {
		t.Fatalf("stats mismatch got %+v", stats)
	}
}
