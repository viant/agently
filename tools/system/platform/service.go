package platform

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/viant/agently-core/protocol/tool/service"
	runtimerequestctx "github.com/viant/agently-core/runtime/requestctx"
)

const Name = "system/platform"

type Service struct {
	mu        sync.RWMutex
	exitCodes map[string]int
}

type SetExitCodeInput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Code           int    `json:"code"`
}

type ExitCodeInput struct {
	ConversationID string `json:"conversationId,omitempty"`
}

type ExitCodeOutput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Code           int    `json:"code"`
}

func New() *Service {
	return &Service{exitCodes: map[string]int{}}
}

func (s *Service) Name() string { return Name }

func (s *Service) Methods() service.Signatures {
	return []service.Signature{
		{
			Name:        "setExitCode",
			Description: "Store an exit code for the current conversation. Agently CLI uses it when the query command exits.",
			Input:       reflect.TypeOf(&SetExitCodeInput{}),
			Output:      reflect.TypeOf(&ExitCodeOutput{}),
		},
		{
			Name:        "exitCode",
			Description: "Return the stored exit code for the current conversation. Returns 0 when nothing was set.",
			Input:       reflect.TypeOf(&ExitCodeInput{}),
			Output:      reflect.TypeOf(&ExitCodeOutput{}),
		},
	}
}

func (s *Service) Method(name string) (service.Executable, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "setexitcode":
		return s.setExitCode, nil
	case "exitcode":
		return s.exitCode, nil
	default:
		return nil, service.NewMethodNotFoundError(name)
	}
}

func (s *Service) setExitCode(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*SetExitCodeInput)
	if !ok {
		return service.NewInvalidInputError(in)
	}
	output, ok := out.(*ExitCodeOutput)
	if !ok {
		return service.NewInvalidOutputError(out)
	}
	conversationID := resolveConversationID(ctx, input.ConversationID)
	if conversationID == "" {
		return fmt.Errorf("conversationId is required")
	}
	s.mu.Lock()
	s.exitCodes[conversationID] = input.Code
	s.mu.Unlock()
	output.ConversationID = conversationID
	output.Code = input.Code
	return nil
}

func (s *Service) exitCode(ctx context.Context, in, out interface{}) error {
	input, ok := in.(*ExitCodeInput)
	if !ok {
		return service.NewInvalidInputError(in)
	}
	output, ok := out.(*ExitCodeOutput)
	if !ok {
		return service.NewInvalidOutputError(out)
	}
	conversationID := resolveConversationID(ctx, input.ConversationID)
	if conversationID == "" {
		output.Code = 0
		return nil
	}
	s.mu.RLock()
	code := s.exitCodes[conversationID]
	s.mu.RUnlock()
	output.ConversationID = conversationID
	output.Code = code
	return nil
}

func resolveConversationID(ctx context.Context, provided string) string {
	if value := strings.TrimSpace(provided); value != "" {
		return value
	}
	return strings.TrimSpace(runtimerequestctx.ConversationIDFromContext(ctx))
}
