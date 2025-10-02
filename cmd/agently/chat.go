package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	mcpclienthandler "github.com/viant/agently/adapter/mcp"
	mcpmgr "github.com/viant/agently/adapter/mcp/manager"
	mcprouter "github.com/viant/agently/adapter/mcp/router"
	"github.com/viant/agently/client/conversation/factory"
	"github.com/viant/agently/cmd/service"
	"github.com/viant/agently/genai/agent/plan"
	elicitationpkg "github.com/viant/agently/genai/elicitation"
	"github.com/viant/agently/genai/executor"
	"github.com/viant/agently/genai/memory"
	promptpkg "github.com/viant/agently/genai/prompt"
	"github.com/viant/agently/genai/tool"
	protoclient "github.com/viant/mcp-protocol/client"
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

	// Attach allows adding one or more files to the LLM request. Repeatable.
	// Format: <path>[::caption]
	Attach []string `long:"attach" description:"file to attach (repeatable). Format: <path>[::caption]"`
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

	// Parse --attach flags into binary attachments
	var attachments []*promptpkg.Attachment
	for _, spec := range c.Attach {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		var pathPart, prompt string
		parts := strings.SplitN(spec, "::", 2)
		pathPart = parts[0]
		if len(parts) == 2 {
			prompt = parts[1]
		}
		data, err := os.ReadFile(pathPart)
		if err != nil {
			return fmt.Errorf("read attachment %q: %w", pathPart, err)
		}
		attachments = append(attachments, &promptpkg.Attachment{
			Name:    filepath.Base(pathPart),
			Mime:    mime.TypeByExtension(filepath.Ext(pathPart)),
			Content: prompt,
			Data:    data,
		})
	}
	convClient, err := factory.NewFromEnv(context.Background())
	if err != nil {
		return err
	}
	registerExecOption(executor.WithConversionClient(convClient))

	// Ensure per-conversation MCP manager is available for chat tool calls.
	// Use an interactive awaiter so elicitation is resolved inline on CLI.
	prov := mcpmgr.NewRepoProvider()
	r := mcprouter.New()
	mgr := mcpmgr.New(prov, mcpmgr.WithHandlerFactory(func() protoclient.Handler {
		el := elicitationpkg.New(convClient, nil, r, func() elicitationpkg.Awaiter { return newStdinAwaiter() })
		// Disable client auto-open
		return mcpclienthandler.NewClient(el, convClient, nil)
	}))
	registerExecOption(executor.WithMCPManager(mgr))
	// Also pass router to agent so assistant-originated elicitations integrate with the same flow
	registerExecOption(executor.WithElicitationRouter(r))
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
	sentAttachments := false

	callChat := func(userQuery string) error {
		ctx := tool.WithPolicy(ctxBase, toolPol)
		ctx = withFluxorPolicy(ctx, fluxPol)
		// Guarantee non-empty conversation ID so downstream components can rely
		// on its presence when the very first turn is executed.
		if convID == "" {
			convID = uuid.NewString()
		}
		ctx = memory.WithConversationID(ctx, convID)
		req := service.ChatRequest{
			ConversationID: convID,
			AgentPath:      c.AgentName,
			Query:          userQuery,
			Context:        contextData,
			Timeout:        time.Duration(c.Timeout) * time.Second,
		}
		if !sentAttachments && len(attachments) > 0 {
			req.Attachments = attachments
			sentAttachments = true
		}

		resp, err := svc.Chat(ctx, req)
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
