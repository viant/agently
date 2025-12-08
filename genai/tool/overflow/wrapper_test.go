package overflow

import (
	"github.com/stretchr/testify/assert"
	"github.com/viant/mcp-protocol/extension"
	yaml "gopkg.in/yaml.v3"
	"testing"
)

func TestBuildOverflowYAML_Bytes(t *testing.T) {
	cont := &extension.Continuation{
		HasMore:   true,
		Remaining: 300,
		Returned:  100,
		NextRange: &extension.RangeHint{
			Bytes: &extension.ByteRange{Offset: 100, Length: 100},
		},
	}
	got, err := BuildOverflowYAML("msg-123", cont)
	assert.NoError(t, err)
	var m map[string]any
	err = yaml.Unmarshal([]byte(got), &m)
	assert.NoError(t, err)

	expected := map[string]any{
		"overflow":  true,
		"messageId": "msg-123",
		"returned":  100,
		"remaining": 300,
		"nextRange": "100-200",
		"bytes": map[string]any{
			"offset": 100,
			"length": 100,
		},
		"hint": "Call internal_message-show with messageId and byteRange.from/to from nextRange.",
	}
	assert.EqualValues(t, expected, m)
}
