package conversation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestComputeStage_ElicitationStatus(t *testing.T) {
	now := time.Now()
	mkMsg := func(status string) *MessageView {
		s := status
		role := "assistant"
		id := "m1"
		elic := "elic-1"
		return &MessageView{
			Id:            id,
			Role:          role,
			CreatedAt:     now,
			Status:        &s,
			ElicitationId: &elic,
			Interim:       0,
			Type:          "text",
		}
	}

	t.Run("pending -> eliciting", func(t *testing.T) {
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Message:   []*MessageView{mkMsg("pending")},
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageEliciting, c.Stage)
	})

	t.Run("rejected -> done", func(t *testing.T) {
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Message:   []*MessageView{mkMsg("rejected")},
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageDone, c.Stage)
	})
}
