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

	t.Run("rejected -> error", func(t *testing.T) {
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Message:   []*MessageView{mkMsg("rejected")},
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageError, c.Stage)
	})

	t.Run("canceled turn -> canceled", func(t *testing.T) {
		turnStatus := "canceled"
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Status:    turnStatus,
			Message:   []*MessageView{mkMsg("canceled")},
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageCanceled, c.Stage)
	})

	t.Run("failed latest turn with user-only message -> error", func(t *testing.T) {
		userStatus := "rejected"
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Status:    "failed",
			Message: []*MessageView{{
				Id:        "m-user",
				Role:      "user",
				CreatedAt: now,
				Status:    &userStatus,
				Type:      "text",
			}},
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageError, c.Stage)
	})

	t.Run("failed latest turn without messages -> error", func(t *testing.T) {
		c := &ConversationView{Transcript: []*TranscriptView{{
			CreatedAt: now,
			Status:    "failed",
		}}}
		c.OnRelation(nil)
		assert.EqualValues(t, StageError, c.Stage)
	})
}

func TestComputeTurnStage_FailedTurnStatus(t *testing.T) {
	now := time.Now()

	t.Run("failed user-only turn -> error", func(t *testing.T) {
		userStatus := "rejected"
		turn := &TranscriptView{
			CreatedAt: now,
			Status:    "failed",
			Message: []*MessageView{{
				Id:        "m-user",
				Role:      "user",
				CreatedAt: now,
				Status:    &userStatus,
				Type:      "text",
			}},
		}
		assert.EqualValues(t, StageError, computeTurnStage(turn))
	})

	t.Run("failed empty turn -> error", func(t *testing.T) {
		turn := &TranscriptView{
			CreatedAt: now,
			Status:    "failed",
		}
		assert.EqualValues(t, StageError, computeTurnStage(turn))
	})
}
