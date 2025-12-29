package textclip

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClipHeadAndTail(t *testing.T) {
	text := "a\nb\nc\nd"
	total := len(text)
	type expected struct {
		content   string
		returned  int
		remaining int
	}
	cases := []struct {
		name     string
		useTail  bool
		maxBytes int
		maxLines int
		expect   expected
	}{
		{
			name:     "head limits lines",
			maxLines: 2,
			expect:   expected{content: "a\nb", returned: len("a\nb"), remaining: total - len("a\nb")},
		},
		{
			name:     "tail limits lines",
			useTail:  true,
			maxLines: 2,
			expect:   expected{content: "c\nd", returned: len("c\nd"), remaining: total - len("c\nd")},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotText string
			var returned, remaining int
			if tc.useTail {
				gotText, returned, remaining = ClipTail(text, total, tc.maxLines)
			} else {
				gotText, returned, remaining = ClipHead(text, total, tc.maxLines)
			}
			assert.EqualValues(t, tc.expect.content, gotText)
			assert.EqualValues(t, tc.expect.returned, returned)
			assert.EqualValues(t, tc.expect.remaining, remaining)
		})
	}
}

func TestExtractSignatures(t *testing.T) {
	content := `
package foo

import "fmt"

func main() {}
// comment
var unused = 1
`
	got := ExtractSignatures(content, 0)
	assert.EqualValues(t, "package foo\nimport \"fmt\"\nfunc main() {}", got)

	gotLimited := ExtractSignatures(content, 10)
	assert.EqualValues(t, gotLimited, got[:10])
}
