package approval

import (
	"encoding/json"
	"time"
)

type Event struct {
	Topic   string
	Data    interface{}
	Headers map[string]string `json:"headers,omitempty"`
}

const (
	TopicRequestCreated  = "request.created"
	TopicRequestUpdated  = "request.updated"
	TopicRequestExpired  = "request.expired"
	TopicDecisionCreated = "decision.created"

	LegacyTopicRequestNew  = "request.new"
	LegacyTopicDecisionNew = "decision.new"
)

type Request struct {
	ID          string                 `json:"id"`
	ProcessID   string                 `json:"processId"`
	ExecutionID string                 `json:"executionId"`
	Action      string                 `json:"action"`
	Args        json.RawMessage        `json:"args,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	ExpiresAt   *time.Time             `json:"expiresAt,omitempty"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
}

type Decision struct {
	ID        string    `json:"id"`
	Approved  bool      `json:"approved"`
	Reason    string    `json:"reason,omitempty"`
	DecidedAt time.Time `json:"decidedAt"`
}
