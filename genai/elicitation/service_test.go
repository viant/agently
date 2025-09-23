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
		{"accept", "accepted"},
		{"ACCEPTED", "accepted"},
		{"approve", "accepted"},
		{"approved", "accepted"},
		{"yes", "accepted"},
		{"y", "accepted"},
		{"decline", "rejected"},
		{"rejected", "rejected"},
		{"reject", "rejected"},
		{"no", "rejected"},
		{"n", "rejected"},
		{"cancel", "cancel"},
		{"canceled", "cancel"},
		{"cancelled", "cancel"},
		{"", "rejected"},
	}
	for _, tc := range testCases {
		got := NormalizeAction(tc.In)
		assert.EqualValues(t, tc.Out, got)
	}
}
