package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/viant/agently-core/sdk"
)

type commandExitCode struct {
	code int
}

func (e *commandExitCode) Error() string {
	return fmt.Sprintf("command exited with code %d", e.code)
}

func (e *commandExitCode) ExitCode() int {
	return e.code
}

type platformExitCodeOutput struct {
	ConversationID string `json:"conversationId,omitempty"`
	Code           int    `json:"code"`
}

func resolveConversationExitCode(ctx context.Context, client *sdk.HTTPClient, conversationID string) (int, error) {
	if client == nil || strings.TrimSpace(conversationID) == "" {
		return 0, nil
	}
	result, err := client.ExecuteTool(ctx, "system/platform/exitCode", map[string]interface{}{
		"conversationId": strings.TrimSpace(conversationID),
	})
	if err != nil {
		return 0, err
	}
	var output platformExitCodeOutput
	if err := json.Unmarshal([]byte(result), &output); err != nil {
		return 0, fmt.Errorf("decode platform exit code: %w", err)
	}
	return output.Code, nil
}
