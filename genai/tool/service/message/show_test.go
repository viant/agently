package message

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShow_TransformAndRanges(t *testing.T) {
	svc := New(nil)
	tests := []struct {
		name        string
		in          ShowInput
		wantContent string
		wantOffset  int
	}{
		{
			name:        "no transform full body",
			in:          ShowInput{Body: "hello\nworld\n"},
			wantContent: "hello\nworld\n",
			wantOffset:  0,
		},
		{
			name:        "line range",
			in:          ShowInput{Body: "a\nb\nc\n", LineRange: &IntRange{From: intPtr(1), To: intPtr(2)}},
			wantContent: "b",
			wantOffset:  2, // after "a\n"
		},
		{
			name:        "byte range",
			in:          ShowInput{Body: "abcdef", ByteRange: &IntRange{From: intPtr(2), To: intPtr(5)}},
			wantContent: "cde",
			wantOffset:  2,
		},
		{
			name:        "sed replace",
			in:          ShowInput{Body: "foo bar", SedExpr: "s/foo/baz/g"},
			wantContent: "baz bar",
			wantOffset:  0,
		},
		{
			name:        "transform csv dot-path",
			in:          ShowInput{Body: `{"data":[{"a":1,"b":"x"},{"a":2,"b":"y"}]}`, Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
		{
			name:        "transform ndjson object",
			in:          ShowInput{Body: `{"a":1}`, Transform: &TransformSpec{Format: "ndjson"}},
			wantContent: "{\"a\":1}\n",
			wantOffset:  0,
		},
		{
			name:        "transform csv with payload preface",
			in:          ShowInput{Body: "status: ok\npayload: {\"data\":[{\"a\":1,\"b\":\"x\"},{\"a\":2,\"b\":\"y\"}]}", Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
		{
			name:        "transform csv with fenced json",
			in:          ShowInput{Body: "Here is the result:\n```json\n{\n  \"data\": [ { \"a\": 1, \"b\": \"x\" }, { \"a\": 2, \"b\": \"y\" } ]\n}\n```\nThanks", Transform: &TransformSpec{Selector: "data", Format: "csv", Fields: []string{"a", "b"}}},
			wantContent: "a,b\n1,x\n2,y\n",
			wantOffset:  0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out ShowOutput
			err := svc.show(nil, &tc.in, &out)
			assert.NoError(t, err)
			assert.EqualValues(t, tc.wantContent, out.Content)
			assert.EqualValues(t, tc.wantOffset, out.Offset)
		})
	}
}

func intPtr(i int) *int { return &i }
