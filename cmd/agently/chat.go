package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/viant/agently/cmd/service"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/genai/conversation"
	"github.com/viant/agently/genai/service/core"
	"github.com/viant/agently/genai/tool"
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
	var attachments []*core.Attachment
	for _, spec := range c.Attach {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		var pathPart, caption string
		parts := strings.SplitN(spec, "::", 2)
		pathPart = parts[0]
		if len(parts) == 2 {
			caption = parts[1]
		}
		data, err := os.ReadFile(pathPart)
		if err != nil {
			return fmt.Errorf("read attachment %q: %w", pathPart, err)
		}
		attachments = append(attachments, &core.Attachment{
			Name:    filepath.Base(pathPart),
			Content: caption,
			Data:    data,
		})
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
	sentAttachments := false

	callChat := func(userQuery string) error {
		ctx := tool.WithPolicy(ctxBase, toolPol)
		ctx = withFluxorPolicy(ctx, fluxPol)
		// Guarantee non-empty conversation ID so downstream components can rely
		// on its presence when the very first turn is executed.
		if convID == "" {
			convID = uuid.NewString()
		}
		ctx = conversation.WithID(ctx, convID)
		req := service.ChatRequest{
			ConversationID: convID,
			AgentPath:      c.AgentName,
			Query:          userQuery,
			Context:        contextData,
			Timeout:        time.Duration(c.Timeout) * time.Second,
		}
		if !sentAttachments && len(attachments) > 0 {
			req.Attachments = attachments
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
		sentAttachments = true
		if strings.TrimSpace(resp.Content) == "" {
			fmt.Println("[no response] - no content")
		} else {
			fmt.Println(resp.Content)
		}
		return nil
	}

	// one-off variant for a single turn with explicit attachments (drag-and-drop)
	callChatWithAtts := func(userQuery string, turnAtts []*core.Attachment) error {
		ctx := tool.WithPolicy(ctxBase, toolPol)
		ctx = withFluxorPolicy(ctx, fluxPol)
		if convID == "" {
			convID = uuid.NewString()
		}
		ctx = conversation.WithID(ctx, convID)
		req := service.ChatRequest{
			ConversationID: convID,
			AgentPath:      c.AgentName,
			Query:          userQuery,
			Context:        contextData,
			Timeout:        time.Duration(c.Timeout) * time.Second,
			Attachments:    turnAtts,
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

		// Detect drag-and-dropped file paths optionally followed by ':: caption'.
		if dropAtts, caption, ok := parseDropLine(line); ok {
			if err := callChatWithAtts(caption, dropAtts); err != nil {
				return err
			}
			continue
		}
		if err := callChat(line); err != nil {
			return err
		}
	}
	return nil
}

// parseDropLine detects whether the input line contains only file path tokens
// (supporting quoted paths) optionally followed by ':: caption'. When all
// path tokens are valid files, it returns attachments, caption, and ok=true.
func parseDropLine(line string) ([]*core.Attachment, string, bool) {
	if strings.TrimSpace(line) == "" {
		return nil, "", false
	}
	var left, caption string
	if idx := strings.Index(line, "::"); idx >= 0 {
		left = strings.TrimSpace(line[:idx])
		caption = strings.TrimSpace(line[idx+2:])
	} else {
		left = strings.TrimSpace(line)
	}
	tokens := tokenizePaths(left)
	if len(tokens) == 0 {
		return nil, "", false
	}
	var atts []*core.Attachment
	firstNonPathIdx := -1
	for i, tok := range tokens {
		p := expandHome(tok)
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			firstNonPathIdx = i
			break
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, "", false
		}
		atts = append(atts, &core.Attachment{
			Name:    filepath.Base(p),
			Content: "",
			Data:    data,
		})
	}
	if len(atts) == 0 {
		return nil, "", false
	}
	if caption == "" && firstNonPathIdx >= 0 {
		caption = strings.TrimSpace(strings.Join(tokens[firstNonPathIdx:], " "))
	}
	return atts, caption, true
}

// tokenizePaths splits a string by whitespace while preserving quoted segments
// (single or double quotes).
func tokenizePaths(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote == 0 {
			switch ch {
			case '"', '\'':
				inQuote = ch
				continue
			case ' ', '\t':
				if cur.Len() > 0 {
					out = append(out, cur.String())
					cur.Reset()
				}
				continue
			}
			cur.WriteByte(ch)
		} else {
			if ch == inQuote {
				inQuote = 0
				continue
			}
			cur.WriteByte(ch)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	for i := range out {
		out[i] = strings.Trim(out[i], "\"'")
	}
	return out
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}
