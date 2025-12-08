package preview

import (
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCompact_StringTrim(t *testing.T) {
	input := map[string]interface{}{
		"id":   "123",
		"text": string(make([]byte, 0, 0)),
	}
	// Build a long string of 10k 'a'
	input["text"] = repeat('a', 10000)

	out, meta, err := Compact(input, Options{BudgetBytes: 1024, MaxString: 256})
	assert.NoError(t, err)
	// meta may or may not be truncated depending on serialization; ensure budget respected
	assert.LessOrEqual(t, meta.ApproxReturnedBytes, 1024)
	// ensure string is trimmed to <= 256 runes
	m := out.(map[string]interface{})
	got := m["text"].(string)
	assert.LessOrEqual(t, len([]rune(got)), 256)
}

func TestCompact_ArraySummarize(t *testing.T) {
	input := map[string]interface{}{
		"items": []interface{}{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}
	// Compute approx original size to set a slightly smaller budget
	b, _ := json.Marshal(input)
	budget := len(b) - 1
	if budget < 1 {
		budget = 1
	}
	out, _, err := Compact(input, Options{BudgetBytes: budget, MaxArray: 3})
	assert.NoError(t, err)
	m := out.(map[string]interface{})
	items := m["items"]
	if arr, ok := items.([]interface{}); ok {
		assert.LessOrEqual(t, len(arr), 3)
	} else if sum, ok := items.(map[string]interface{}); ok {
		// summarized node present
		_, has := sum["__summary"]
		assert.True(t, has)
	} else {
		t.Fatalf("unexpected items type %T", items)
	}
}

func TestCompact_ObjectSummarizeLargest(t *testing.T) {
	big := map[string]interface{}{"x": repeat('b', 8000)}
	input := map[string]interface{}{
		"id":    "abc",
		"small": "ok",
		"big":   big,
	}
	out, meta, err := Compact(input, Options{BudgetBytes: 1024, MaxString: 0, PreserveKeys: []string{"id"}})
	assert.NoError(t, err)
	assert.LessOrEqual(t, meta.ApproxReturnedBytes, 1024)
	m := out.(map[string]interface{})
	// "big" should be summarized
	bsum := m["big"].(map[string]interface{})
	assert.EqualValues(t, "object", bsum["__summary"])
}

func repeat(ch rune, n int) string {
	r := make([]rune, n)
	for i := 0; i < n; i++ {
		r[i] = ch
	}
	b, _ := json.Marshal(string(r))
	var s string
	_ = json.Unmarshal(b, &s)
	return s
}
