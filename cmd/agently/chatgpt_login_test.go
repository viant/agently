package agently

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeriveTokensURLFromClientURL(t *testing.T) {
	type testCase struct {
		name      string
		clientURL string
		expected  string
		wantErr   bool
	}

	testCases := []testCase{
		{name: "json path", clientURL: "/Users/me/.secret/gpt.json", expected: "/Users/me/.secret/gpt.tokens.json"},
		{name: "yaml path", clientURL: "/Users/me/.secret/gpt.yaml", expected: "/Users/me/.secret/gpt.tokens.yaml"},
		{name: "no ext", clientURL: "/Users/me/.secret/gpt", expected: "/Users/me/.secret/gpt.tokens.json"},
		{name: "with key suffix", clientURL: "/Users/me/.secret/gpt.json|blowfish://default", expected: "/Users/me/.secret/gpt.tokens.json|blowfish://default"},
		{name: "file url", clientURL: "file:///Users/me/.secret/gpt.json", expected: "file:///Users/me/.secret/gpt.tokens.json"},
		{name: "file url with suffix", clientURL: "file:///Users/me/.secret/gpt.json|blowfish://default", expected: "file:///Users/me/.secret/gpt.tokens.json|blowfish://default"},
		{name: "empty", clientURL: "", expected: "", wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := deriveTokensURLFromClientURL(tc.clientURL)
			if tc.wantErr {
				assert.EqualValues(t, true, err != nil)
				return
			}
			assert.EqualValues(t, nil, err)
			assert.EqualValues(t, tc.expected, got)
		})
	}
}
