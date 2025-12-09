package elicitation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAction(t *testing.T) {
	testCases := []struct {
		In  string
		Out string
	}{
		{"accept", "accept"},
		{"ACCEPTED", "accept"},
		{"approve", "accept"},
		{"approved", "accept"},
		{"yes", "accept"},
		{"y", "accept"},
		{"decline", "decline"},
		{"rejected", "decline"},
		{"reject", "decline"},
		{"no", "decline"},
		{"n", "decline"},
		{"cancel", "cancel"},
		{"canceled", "cancel"},
		{"cancelled", "cancel"},
		{"", "decline"},
	}
	for _, tc := range testCases {
		got := NormalizeAction(tc.In)
		assert.EqualValues(t, tc.Out, got)
	}
}
