package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/tool"
	"github.com/viant/agently/service"
)

// ChatCmd handles interactive/chat queries.
type ChatCmd struct {
	AgentName string `short:"a" long:"agent-name" description:"agent name"`
	Query     string `short:"q" long:"query"    description:"user query"`
	ConvID    string `short:"c" long:"conv"     description:"conversation ID (optional)"`
	Policy    string `short:"p" long:"policy" description:"tool policy: auto|ask|deny" default:"auto"`
	ResetLogs bool   `long:"reset-logs" description:"truncate/clean log files before each run"  `
	Timeout   int    `short:"t" long:"timeout" description:"timeout in seconds for the agent response (0=none)" `

	// Arbitrary JSON payload that will be forwarded to the agent as contextual
	// information. It can be supplied either as an inline JSON string or as
	// @<path> pointing to a file containing the JSON document.
	Context string `long:"context" description:"inline JSON object or @file with context data"`
}

// cliInteractionHandler satisfies service.InteractionHandler by prompting the
// user on STDIN when the assistant requests additional information.
type cliInteractionHandler struct{}

func (cliInteractionHandler) Accept(ctx context.Context, el *plan.Elicitation) (service.AcceptResult, error) {
	res, err := newStdinAwaiter().AwaitElicitation(ctx, el)
	if err != nil {
		return service.AcceptResult{}, err
	}
	switch res.Action {
	case plan.ElicitResultActionDecline:
		return service.AcceptResult{Action: service.ActionDecline}, nil
	case plan.ElicitResultActionAccept:
		if len(res.Payload) == 0 {
			return service.AcceptResult{Action: service.ActionDecline}, nil
		}
		data, err := json.Marshal(res.Payload)
		if err != nil {
			return service.AcceptResult{}, err
		}
		return service.AcceptResult{Action: service.ActionAccept, Payload: data}, nil
	default:
		return service.AcceptResult{Action: service.ActionDecline}, nil
	}
}

func (c *ChatCmd) Execute(_ []string) error {
	// Fallbacks -------------------------------------------------------
	if c.AgentName == "" {
		c.AgentName = "chat" // default agent shipped with embedded config
	}

	// -----------------------------------------------------------------
	// Parse --context flag (inline JSON or @file) once so that it is reused
	// across all turns in the conversation.
	var contextData map[string]interface{}
	if ctxArg := strings.TrimSpace(c.Context); ctxArg != "" {
		var data []byte
		if strings.HasPrefix(ctxArg, "@") {
			filePath := strings.TrimPrefix(ctxArg, "@")
			b, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("read context file: %w", err)
			}
			data = b
		} else {
			data = []byte(ctxArg)
		}
		if err := json.Unmarshal(data, &contextData); err != nil {
			return fmt.Errorf("parse context JSON: %w", err)
		}
	}

	// Build executor and service --------------------------------------------
	svcExec := executorSingleton()

	ctxBase, cancel := context.WithCancel(context.Background())
	defer cancel()

	fluxPol := buildFluxorPolicy(c.Policy)
	toolPol := &tool.Policy{Mode: fluxPol.Mode, Ask: stdinAsk}

	stopApprove := startApprovalLoop(ctxBase, svcExec, fluxPol)
	defer stopApprove()

	serviceOpts := service.Options{Interaction: cliInteractionHandler{}}
	svc := service.New(svcExec, serviceOpts)

	convID := c.ConvID

	callChat := func(userQuery string) error {
		ctx := tool.WithPolicy(ctxBase, toolPol)
		ctx = withFluxorPolicy(ctx, fluxPol)
		// Guarantee non-empty conversation ID so downstream components can rely
		// on its presence when the very first turn is executed.
		if convID == "" {
			convID = uuid.NewString()
		}
		ctx = conversation.WithID(ctx, convID)
		resp, err := svc.Chat(ctx, service.ChatRequest{
			ConversationID: convID,
			AgentPath:      c.AgentName,
			Query:          userQuery,
			Context:        contextData,
			Timeout:        time.Duration(c.Timeout) * time.Second,
		})
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("[no response] - timeout")
				return nil
			}
			return err
		}
		convID = resp.ConversationID
		if strings.TrimSpace(resp.Content) == "" {
			fmt.Println("[no response] - no content")
		} else {
			fmt.Println(resp.Content)
		}
		return nil
	}

	// Single-turn when -q provided.
	if c.Query != "" {
		if err := callChat(c.Query); err != nil {
			return err
		}
		fmt.Printf("[conversation-id] %s\n", convID)
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" || line == "exit" || line == "quit" {
			fmt.Printf("[conversation-id] %s\n", convID)
			break
		}
		if err := callChat(line); err != nil {
			return err
		}
	}
	return nil
}
