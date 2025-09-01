package domain

import "time"

// Core domain entities. Keep these DTOs persistence-agnostic.

type UsageTotals struct {
	InputTokens     int
	OutputTokens    int
	EmbeddingTokens int
}

type Turn struct {
	ID             string
	ConversationID string
	CreatedAt      *time.Time
	Status         TurnStatus
}

type TurnStatus string

const (
	TurnPending        TurnStatus = "pending"
	TurnRunning        TurnStatus = "running"
	TurnWaitingForUser TurnStatus = "waiting_for_user"
	TurnSucceeded      TurnStatus = "succeeded"
	TurnFailed         TurnStatus = "failed"
	TurnCanceled       TurnStatus = "canceled"
)
