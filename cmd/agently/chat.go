package agently

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/viant/agently/client/sdk"
	"github.com/viant/agently/genai/agent/plan"
	"github.com/viant/agently/internal/workspace"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// ChatCmd handles interactive/chat queries.
type ChatCmd struct {
	AgentID   string   `short:"a" long:"agent-id" description:"agent id"`
	Query     []string `short:"q" long:"query"    description:"user query (repeatable)"`
	ConvID    string   `short:"c" long:"conv"     description:"conversation ID (optional)"`
	ResetLogs bool     `long:"reset-logs" description:"truncate/clean log files before each run"  `
	Timeout   int      `short:"t" long:"timeout" description:"timeout in seconds for the agent response (0=none)" `
	User      string   `short:"u" long:"user" description:"user id for the chat" default:"devuser"`
	API       string   `long:"api" description:"Agently base URL (skips auto-detect)" `
	Token     string   `long:"token" description:"Bearer token for API requests (overrides AGENTLY_TOKEN)" `
	OOB       bool     `long:"oob" description:"Use server-side OAuth2 out-of-band login (requires --oauth-secrets)"`
	OAuthCfg  string   `long:"oauth-config" description:"scy OAuth config URL for client-side OOB login (unused for server OOB)"`
	OAuthSec  string   `long:"oauth-secrets" description:"scy OAuth secrets URL for OOB login"`
	OAuthScp  string   `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	Stream    bool     `long:"stream" description:"force SSE streaming (disable poll fallback)"`
	ElicitDef string   `long:"elicitation-default" description:"JSON or @file to auto-accept elicitations when stdin is not a TTY"`

	// Arbitrary JSON payload that will be forwarded to the agent as contextual
	// information. It can be supplied either as an inline JSON string or as
	// @<path> pointing to a file containing the JSON document.
	Context string `long:"context" description:"inline JSON object or @file with context data"`

	// Attach allows adding one or more files to the LLM request. Repeatable.
	// Format: <path>
	Attach []string `long:"attach" description:"file to attach (repeatable). Format: <path>"`
}

type attachSpec struct {
	Path    string
	Name    string
	Content []byte
	Mime    string
}

func (c *ChatCmd) Execute(_ []string) error {
	if strings.TrimSpace(c.AgentID) == "" {
		c.AgentID = "chatter"
	}

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

	var attachments []attachSpec
	for _, spec := range c.Attach {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		pathPart := spec
		data, err := os.ReadFile(pathPart)
		if err != nil {
			return fmt.Errorf("read attachment %q: %w", pathPart, err)
		}
		attachments = append(attachments, attachSpec{
			Path:    pathPart,
			Name:    filepath.Base(pathPart),
			Mime:    mime.TypeByExtension(filepath.Ext(pathPart)),
			Content: data,
		})
	}

	ctxBase := context.Background()
	baseURL, providers, workspaceRoot, defaultAgent, defaultModel, models, err := c.resolveBaseURL(ctxBase)
	if err != nil {
		return err
	}
	if strings.TrimSpace(defaultAgent) != "" && strings.TrimSpace(c.AgentID) == "chatter" {
		c.AgentID = strings.TrimSpace(defaultAgent)
	}
	modelOverride := pickModel(defaultModel, models)

	jar := cliCookieJar()
	var opts []sdk.Option
	opts = append(opts, sdk.WithCookieJar(jar))
	if c.Timeout > 0 {
		opts = append(opts, sdk.WithTimeout(time.Duration(c.Timeout)*time.Second))
	}
	token := strings.TrimSpace(c.Token)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("AGENTLY_TOKEN"))
	}
	if token != "" {
		tok := token
		opts = append(opts, sdk.WithTokenProvider(func(context.Context) (string, error) {
			return tok, nil
		}))
	}
	client := sdk.New(baseURL, opts...)
	if err := c.ensureAuth(ctxBase, client, providers); err != nil {
		return err
	}
	if strings.TrimSpace(workspaceRoot) != "" {
		fmt.Printf("[workspace] %s\n", workspaceRoot)
	} else {
		fmt.Printf("[workspace] %s\n", workspace.Root())
	}
	if strings.TrimSpace(modelOverride) != "" {
		fmt.Printf("[agent] %s [model] %s\n", c.AgentID, modelOverride)
	} else {
		fmt.Printf("[agent] %s\n", c.AgentID)
	}

	convID := strings.TrimSpace(c.ConvID)
	sentAttachments := false
	var elicitationDefault map[string]interface{}
	if strings.TrimSpace(c.ElicitDef) != "" {
		payload, err := parseJSONArg(c.ElicitDef)
		if err != nil {
			return fmt.Errorf("parse --elicitation-default: %w", err)
		}
		if payload != nil {
			elicitationDefault = payload
		}
	}

	var outputOpen bool
	var lastElicitationPayload map[string]interface{}
	var queryLog io.Writer
	if len(c.Query) > 0 {
		logDir := filepath.Join(workspace.Root(), "cli", "logs")
		_ = os.MkdirAll(logDir, 0o700)
		logPath := filepath.Join(logDir, "last_query.log")
		if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600); err == nil {
			queryLog = f
			fmt.Fprintf(queryLog, "[query] %s\n", time.Now().Format(time.RFC3339))
		}
	}
	callChat := func(userQuery string) error {
		ctx := ctxBase
		if c.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
			defer cancel()
		}
		if convID == "" {
			resp, err := client.CreateConversation(ctx, &sdk.CreateConversationRequest{
				Agent: c.AgentID,
			})
			if err != nil {
				return err
			}
			convID = resp.ID
		}
		req := &sdk.PostMessageRequest{
			Content: userQuery,
			Agent:   c.AgentID,
			Context: contextData,
		}
		if strings.TrimSpace(modelOverride) != "" {
			req.Model = strings.TrimSpace(modelOverride)
		}
		if !sentAttachments && len(attachments) > 0 {
			up, err := uploadAttachments(ctx, client, attachments)
			if err != nil {
				return err
			}
			req.Attachments = up
			sentAttachments = true
		}
		lastAssistant := ""
		if convID != "" {
			lastAssistant = fetchLastAssistantContent(ctx, client, convID)
		}
		postStart := time.Now()
		postResp, err := client.PostMessage(ctx, convID, req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("[no response] - timeout")
				return nil
			}
			return err
		}
		msgID := ""
		if postResp != nil {
			msgID = strings.TrimSpace(postResp.ID)
			if msgID == "" {
				msgID = strings.TrimSpace(postResp.TurnID)
			}
		}
		outputOpen = false
		if err := streamConversationTurn(ctx, client, convID, msgID, userQuery, !c.Stream, elicitationDefault, postStart, lastAssistant, &outputOpen, &lastElicitationPayload, queryLog); err != nil {
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				fmt.Println("[no response] - timeout")
				return nil
			}
			return err
		}
		return nil
	}

	// Single-turn when -q provided.
	if len(c.Query) > 0 {
		for _, q := range c.Query {
			if strings.TrimSpace(q) == "" {
				continue
			}
			if err := callChat(q); err != nil {
				return err
			}
		}
		if c, ok := queryLog.(io.Closer); ok && c != nil {
			_ = c.Close()
		}
		if outputOpen {
			fmt.Print("\n")
			outputOpen = false
		}
		fmt.Printf("[conversation-id] %s\n", convID)
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	handledPending := map[string]struct{}{}
	for {
		if convID != "" {
			handled, err := handlePendingElicitation(ctxBase, client, convID, handledPending, elicitationDefault, &lastElicitationPayload)
			if err != nil {
				return err
			}
			if handled {
				continue
			}
		}
		if outputOpen {
			fmt.Print("\n")
			outputOpen = false
		}
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

func uploadAttachments(ctx context.Context, client *sdk.Client, attachments []attachSpec) ([]sdk.UploadedAttachment, error) {
	var out []sdk.UploadedAttachment
	for _, att := range attachments {
		up, err := client.UploadAttachment(ctx, att.Name, bytesReader(att.Content))
		if err != nil {
			return nil, err
		}
		out = append(out, sdk.UploadedAttachment{
			Name:          up.Name,
			URI:           up.URI,
			Size:          int(up.Size),
			StagingFolder: up.StagingFolder,
			Mime:          att.Mime,
		})
	}
	return out, nil
}

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

func cliCookieJar() http.CookieJar {
	dir := filepath.Join(workspace.Root(), "cli")
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "cookies.json")
	jar, err := authtransport.NewFileJar(path)
	if err == nil && jar != nil {
		return jar
	}
	fallback, _ := cookiejar.New(nil)
	return fallback
}

func streamConversationTurn(ctx context.Context, client *sdk.Client, convID string, msgID string, userQuery string, allowFallback bool, elicitationDefault map[string]interface{}, postStart time.Time, lastAssistant string, outputOpen *bool, lastElicitationPayload *map[string]interface{}, logw io.Writer) error {
	since := ""
	if strings.TrimSpace(msgID) != "" {
		since = strings.TrimSpace(msgID)
	}
	events, errs, err := client.StreamTurnEvents(ctx, convID, since, []string{"text", "tool_op", "control", "elicitation"}, false)
	if err != nil {
		return err
	}
	seenAssistant := false
	lineOpen := false
	handledElicitations := map[string]struct{}{}
	activeElicitationTurns := map[string]struct{}{}
	handledElicitation := false
	lastPrinted := map[string]string{}
	lastOutputText := ""
	var lastOutputAt time.Time
	printedTextAt := map[string]time.Time{}
	lastMsgIDPrinted := ""
	hadOutput := false
	activeTurnID := ""
	ticker := time.NewTicker(1 * time.Second)
	var fallback *time.Timer
	if allowFallback {
		fallback = time.NewTimer(3 * time.Second)
	}
	defer ticker.Stop()
	if fallback != nil {
		defer fallback.Stop()
	}
	lastStatus := ""
	var statusStart time.Time
	statusFinalized := false
	defer func() {
		if !statusFinalized {
			if lastStatus == "" && hadOutput {
				lastStatus = "succeeded"
				if statusStart.IsZero() {
					statusStart = time.Now()
				}
			} else {
				markStatusSucceeded(&lastStatus, &statusStart)
			}
			if lastStatus != "" {
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
			}
		}
	}()
	for {
		select {
		case <-func() <-chan time.Time {
			if fallback == nil {
				return nil
			}
			return fallback.C
		}():
			if !seenAssistant {
				return streamConversationPoll(ctx, client, convID, msgID, elicitationDefault, postStart, lastAssistant, outputOpen, lastElicitationPayload, logw)
			}
		case ev, ok := <-events:
			if !ok {
				markStatusSucceeded(&lastStatus, &statusStart)
				if lastStatus != "" {
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				}
				if outputOpen != nil && *outputOpen {
					fmt.Print("\n")
				}
				return nil
			}
			if ev == nil {
				continue
			}
			logTurnEvent(logw, ev)
			switch ev.Type {
			case sdk.TurnEventElicitation:
				if ev.Elicitation == nil || strings.TrimSpace(ev.Elicitation.ElicitationID) == "" {
					continue
				}
				logElicitationEvent(logw, ev)
				if tid := strings.TrimSpace(ev.TurnID); tid != "" {
					activeElicitationTurns[tid] = struct{}{}
				}
				if _, ok := handledElicitations[ev.Elicitation.ElicitationID]; ok {
					continue
				}
				handledElicitations[ev.Elicitation.ElicitationID] = struct{}{}
				if err := resolveElicitation(ctx, client, ev.ConversationID, ev.Elicitation, elicitationDefault, lastElicitationPayload); err != nil {
					return err
				}
				handledElicitation = true
				if tid := strings.TrimSpace(ev.TurnID); tid != "" {
					delete(activeElicitationTurns, tid)
				}
				continue
			case sdk.TurnEventTool:
				phase := strings.TrimSpace(ev.ToolPhase)
				if phase == "" {
					phase = "event"
				}
				fmt.Printf("\n[tool] %s %s\n", phase, strings.TrimSpace(ev.ToolName))
				if logw != nil {
					fmt.Fprintf(logw, "tool phase=%s name=%s\n", phase, strings.TrimSpace(ev.ToolName))
				}
				continue
			case sdk.TurnEventDelta:
				// proceed
			default:
				continue
			}

			if shouldSkipByTime(ev.CreatedAt, postStart) {
				continue
			}
			if activeTurnID == "" && strings.TrimSpace(ev.TurnID) != "" {
				activeTurnID = strings.TrimSpace(ev.TurnID)
			}
			if activeTurnID != "" && strings.TrimSpace(ev.TurnID) != "" && strings.TrimSpace(ev.TurnID) != activeTurnID {
				continue
			}
			if tid := strings.TrimSpace(ev.TurnID); tid != "" {
				if _, ok := activeElicitationTurns[tid]; ok {
					continue
				}
			}
			text := ev.TextFull
			if strings.TrimSpace(text) == "" {
				text = ev.Text
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			if looksLikeElicitationStream(text) || looksLikeElicitationJSON(text) {
				handledElicitation = true
				continue
			}
			normText := normalizeOutputText(text)
			if lastOutputText != "" && normText == lastOutputText && time.Since(lastOutputAt) < 2*time.Second {
				continue
			}
			if ts, ok := printedTextAt[normText]; ok && time.Since(ts) < 5*time.Second {
				continue
			}
			if !seenAssistant {
				if lastStatus != "" {
					fmt.Print("\n")
				}
				seenAssistant = true
			}
			msgID := strings.TrimSpace(ev.MessageID)
			if msgID != "" && msgID != lastMsgIDPrinted && (lineOpen || (outputOpen != nil && *outputOpen)) {
				fmt.Print("\n")
				lineOpen = false
			}
			if printDeltaText(lastPrinted, msgID, text) {
				lineOpen = true
				if outputOpen != nil {
					*outputOpen = true
				}
				hadOutput = true
				if normText != "" {
					printedTextAt[normText] = time.Now()
				}
				lastMsgIDPrinted = msgID
			}
			lastOutputText = normalizeOutputText(text)
			lastOutputAt = time.Now()
		case err := <-errs:
			if err != nil {
				return err
			}
		case <-ticker.C:
			if msgID == "" {
				continue
			}
			if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
				return err
			} else if handled {
				continue
			}
			status, errMsg, ok := pollTurnStatus(ctx, client, convID, msgID)
			if !ok {
				continue
			}
			if status != "" && !seenAssistant && !hadOutput {
				renderStatus(&lastStatus, &statusStart, status)
			}
			if logw != nil && status != "" {
				fmt.Fprintf(logw, "status=%s\n", status)
			}
			if status == "failed" {
				if strings.TrimSpace(errMsg) != "" {
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					return fmt.Errorf("turn failed: %s", strings.TrimSpace(errMsg))
				}
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				return fmt.Errorf("turn failed")
			}
			if status == "succeeded" {
				if handledElicitation && !seenAssistant && !hadOutput {
					// Wait for the post-elicitation assistant response; don't replay last assistant.
					continue
				}
				if !seenAssistant && !hadOutput {
					if printed := printLastAssistant(ctx, client, convID, postStart); printed {
						if outputOpen != nil {
							*outputOpen = true
						}
						hadOutput = true
					}
				}
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				if lineOpen || (outputOpen != nil && *outputOpen) {
					fmt.Print("\n")
					lineOpen = false
				}
				return nil
			}
			if status == "canceled" {
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				return fmt.Errorf("turn canceled")
			}
		case <-ctx.Done():
			if !seenAssistant && !hadOutput {
				if printed := printLastAssistant(ctx, client, convID, postStart); printed {
					if outputOpen != nil {
						*outputOpen = true
					}
					hadOutput = true
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					fmt.Print("\n")
					return nil
				}
			}
			return ctx.Err()
		}
	}
}

func streamConversation(ctx context.Context, client *sdk.Client, convID string, msgID string, userQuery string, allowFallback bool, elicitationDefault map[string]interface{}, postStart time.Time, lastAssistant string, outputOpen *bool, lastElicitationPayload *map[string]interface{}) error {
	since := ""
	if strings.TrimSpace(msgID) != "" {
		since = strings.TrimSpace(msgID)
	}
	events, errs, err := client.StreamEventsWithOptions(ctx, convID, since, []string{"text", "tool_op", "control", "elicitation"}, false)
	if err != nil {
		return err
	}
	buf := sdk.NewMessageBuffer()
	seenAssistant := false
	seenDelta := false
	lineOpen := false
	handledElicitations := map[string]struct{}{}
	lastPrinted := map[string]string{}
	heldElicitation := map[string]string{}
	lastOutputText := ""
	var lastOutputAt time.Time
	printedTextAt := map[string]time.Time{}
	lastMsgIDPrinted := ""
	hadOutput := false
	activeTurnID := ""
	ticker := time.NewTicker(1 * time.Second)
	var fallback *time.Timer
	if allowFallback {
		fallback = time.NewTimer(3 * time.Second)
	}
	defer ticker.Stop()
	if fallback != nil {
		defer fallback.Stop()
	}
	lastStatus := ""
	var statusStart time.Time
	statusFinalized := false
	defer func() {
		if !statusFinalized {
			if lastStatus == "" && hadOutput {
				lastStatus = "succeeded"
				if statusStart.IsZero() {
					statusStart = time.Now()
				}
			} else {
				markStatusSucceeded(&lastStatus, &statusStart)
			}
			if lastStatus != "" {
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
			}
		}
	}()
	for {
		select {
		case <-func() <-chan time.Time {
			if fallback == nil {
				return nil
			}
			return fallback.C
		}():
			if !seenAssistant {
				return streamConversationPoll(ctx, client, convID, msgID, elicitationDefault, postStart, lastAssistant, outputOpen, lastElicitationPayload, nil)
			}
		case ev, ok := <-events:
			if !ok {
				markStatusSucceeded(&lastStatus, &statusStart)
				if lastStatus != "" {
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				}
				if outputOpen != nil && *outputOpen {
					fmt.Print("\n")
				}
				return nil
			}
			if ev == nil || ev.Message == nil {
				continue
			}
			if err := maybeResolveElicitation(ctx, client, ev, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
				return err
			}
			role := strings.ToLower(strings.TrimSpace(ev.Message.Role))
			if role == "user" && activeTurnID == "" {
				if ev.Message.Content != nil && strings.TrimSpace(*ev.Message.Content) == strings.TrimSpace(userQuery) {
					if ev.Message.TurnId != nil && strings.TrimSpace(*ev.Message.TurnId) != "" {
						activeTurnID = strings.TrimSpace(*ev.Message.TurnId)
					}
				}
			}
			if role == "assistant" {
				if sdk.IsElicitationPending(ev.Message) {
					continue
				}
				if shouldSkipByTime(ev.Message.CreatedAt, postStart) {
					continue
				}
				if activeTurnID != "" && ev.Message.TurnId != nil {
					if strings.TrimSpace(*ev.Message.TurnId) != activeTurnID {
						continue
					}
				}
				if ev.Event.IsDelta() {
					msgID, text, changed := buf.ApplyEvent(ev)
					if changed && strings.TrimSpace(text) != "" {
						if looksLikeProviderJSON(text) {
							continue
						}
						if shouldHoldElicitationText(msgID, text, heldElicitation) {
							continue
						}
						if looksLikeElicitationJSON(text) || looksLikeElicitationStream(text) {
							continue
						}
						if !seenAssistant && lastAssistant != "" && strings.TrimSpace(text) == lastAssistant {
							continue
						}
						normText := normalizeOutputText(text)
						if lastOutputText != "" && normText == lastOutputText && time.Since(lastOutputAt) < 2*time.Second {
							continue
						}
						if ts, ok := printedTextAt[normText]; ok && time.Since(ts) < 5*time.Second {
							continue
						}
						seenDelta = true
						if !seenAssistant {
							if lastStatus != "" {
								fmt.Print("\n")
							}
							seenAssistant = true
						}
						if msgID != "" && msgID != lastMsgIDPrinted && (lineOpen || (outputOpen != nil && *outputOpen)) {
							fmt.Print("\n")
							lineOpen = false
						}
						if printDeltaText(lastPrinted, msgID, text) {
							lineOpen = true
							if outputOpen != nil {
								*outputOpen = true
							}
							hadOutput = true
							if normText != "" {
								printedTextAt[normText] = time.Now()
							}
							lastMsgIDPrinted = msgID
						}
						lastOutputText = normalizeOutputText(text)
						lastOutputAt = time.Now()
					}
					if streamStop(ev) {
						if lineOpen {
							fmt.Print("\n")
							lineOpen = false
						}
						printed, wasElic := flushHeldIfNotElicitation(msgID, heldElicitation, lastPrinted)
						if printed {
							lineOpen = true
							fmt.Print("\n")
							seenAssistant = true
							hadOutput = true
						}
						if wasElic {
							if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
								return err
							} else if handled {
								return nil
							}
						}
						markStatusSucceeded(&lastStatus, &statusStart)
						finalizeStatus(&lastStatus, &statusStart, &lineOpen)
						statusFinalized = true
						fmt.Print("\n")
						return nil
					}
					if ev.Message.Interim == 0 && seenAssistant {
						printed, wasElic := flushHeldIfNotElicitation(msgID, heldElicitation, lastPrinted)
						if printed {
							lineOpen = true
							fmt.Print("\n")
							if outputOpen != nil {
								*outputOpen = true
							}
						}
						if wasElic {
							if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
								return err
							} else if handled {
								return nil
							}
						}
						markStatusSucceeded(&lastStatus, &statusStart)
						finalizeStatus(&lastStatus, &statusStart, &lineOpen)
						statusFinalized = true
						fmt.Print("\n")
						return nil
					}
					continue
				}
				if seenDelta {
					if ev.Message.Interim == 0 && seenAssistant {
						finalizeStatus(&lastStatus, &statusStart, &lineOpen)
						fmt.Print("\n")
						return nil
					}
					continue
				}
				msgID, text, changed := buf.ApplyEvent(ev)
				if changed && strings.TrimSpace(text) != "" {
					if looksLikeProviderJSON(text) {
						continue
					}
					if shouldHoldElicitationText(msgID, text, heldElicitation) {
						continue
					}
					if looksLikeElicitationJSON(text) || looksLikeElicitationStream(text) {
						continue
					}
					if !seenAssistant && lastAssistant != "" && strings.TrimSpace(text) == lastAssistant {
						continue
					}
					normText := normalizeOutputText(text)
					if lastOutputText != "" && normText == lastOutputText && time.Since(lastOutputAt) < 2*time.Second {
						continue
					}
					if ts, ok := printedTextAt[normText]; ok && time.Since(ts) < 5*time.Second {
						continue
					}
					if !seenAssistant {
						if lastStatus != "" {
							fmt.Print("\n")
						}
						seenAssistant = true
					}
					if msgID != "" && msgID != lastMsgIDPrinted && (lineOpen || (outputOpen != nil && *outputOpen)) {
						fmt.Print("\n")
						lineOpen = false
					}
					if printDeltaText(lastPrinted, msgID, text) {
						lineOpen = true
						if outputOpen != nil {
							*outputOpen = true
						}
						hadOutput = true
						if normText != "" {
							printedTextAt[normText] = time.Now()
						}
						lastMsgIDPrinted = msgID
					}
					lastOutputText = normalizeOutputText(text)
					lastOutputAt = time.Now()
				}
				if streamStop(ev) {
					if lineOpen {
						fmt.Print("\n")
						lineOpen = false
					}
					printed, wasElic := flushHeldIfNotElicitation(msgID, heldElicitation, lastPrinted)
					if printed {
						lineOpen = true
						fmt.Print("\n")
						seenAssistant = true
						hadOutput = true
					}
					if wasElic {
						if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
							return err
						} else if handled {
							return nil
						}
					}
					markStatusSucceeded(&lastStatus, &statusStart)
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					fmt.Print("\n")
					return nil
				}
				if ev.Message.Interim == 0 && seenAssistant {
					printed, wasElic := flushHeldIfNotElicitation(msgID, heldElicitation, lastPrinted)
					if printed {
						lineOpen = true
						fmt.Print("\n")
						hadOutput = true
					}
					if wasElic {
						if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
							return err
						} else if handled {
							return nil
						}
					}
					if lineOpen {
						fmt.Print("\n")
						lineOpen = false
					}
					markStatusSucceeded(&lastStatus, &statusStart)
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					fmt.Print("\n")
					return nil
				}
			}
			if name := sdk.ToolName(ev); name != "" {
				phase := sdk.ToolPhase(ev)
				if phase == "" {
					phase = "event"
				}
				fmt.Printf("\n[tool] %s %s\n", phase, name)
			}
		case err := <-errs:
			if err != nil {
				return err
			}
		case <-ticker.C:
			if msgID == "" || seenAssistant {
				continue
			}
			if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
				return err
			} else if handled {
				continue
			}
			status, errMsg, ok := pollTurnStatus(ctx, client, convID, msgID)
			if !ok {
				continue
			}
			if status != "" {
				renderStatus(&lastStatus, &statusStart, status)
			}
			if status == "failed" {
				if strings.TrimSpace(errMsg) != "" {
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					return fmt.Errorf("turn failed: %s", strings.TrimSpace(errMsg))
				}
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				return fmt.Errorf("turn failed")
			}
			if status == "succeeded" && !seenAssistant && !hadOutput {
				if printed := printLastAssistant(ctx, client, convID, postStart); printed {
					if outputOpen != nil {
						*outputOpen = true
					}
					hadOutput = true
				}
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				fmt.Print("\n")
				return nil
			}
			if status == "canceled" {
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
				statusFinalized = true
				return fmt.Errorf("turn canceled")
			}
		case <-ctx.Done():
			if !seenAssistant && !hadOutput {
				if printed := printLastAssistant(ctx, client, convID, postStart); printed {
					if outputOpen != nil {
						*outputOpen = true
					}
					hadOutput = true
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					fmt.Print("\n")
					return nil
				}
			}
			return ctx.Err()
		}
	}
}

func streamConversationPoll(ctx context.Context, client *sdk.Client, convID string, msgID string, elicitationDefault map[string]interface{}, postStart time.Time, lastAssistant string, outputOpen *bool, lastElicitationPayload *map[string]interface{}, logw io.Writer) error {
	buf := sdk.NewMessageBuffer()
	seenAssistant := false
	lineOpen := false
	handledElicitations := map[string]struct{}{}
	lastPrinted := map[string]string{}
	heldElicitation := map[string]string{}
	lastOutputText := ""
	var lastOutputAt time.Time
	printedTextAt := map[string]time.Time{}
	lastMsgIDPrinted := ""
	hadOutput := false
	lastStatus := ""
	var statusStart time.Time
	statusFinalized := false
	defer func() {
		if !statusFinalized {
			if lastStatus == "" && hadOutput {
				lastStatus = "succeeded"
				if statusStart.IsZero() {
					statusStart = time.Now()
				}
			} else {
				markStatusSucceeded(&lastStatus, &statusStart)
			}
			if lastStatus != "" {
				finalizeStatus(&lastStatus, &statusStart, &lineOpen)
			}
		}
	}()
	since := ""
	if strings.TrimSpace(msgID) != "" {
		since = strings.TrimSpace(msgID)
	}

	for {
		resp, err := client.PollEvents(ctx, convID, since, []string{"text", "tool_op", "control", "elicitation"}, 1000*time.Millisecond)
		if err != nil {
			return err
		}
		if resp != nil && resp.Since != "" {
			since = resp.Since
		}
		if resp != nil {
			for _, ev := range resp.Events {
				if ev == nil || ev.Message == nil {
					continue
				}
				if logw != nil && ev.Event != "" {
					fmt.Fprintf(logw, "poll event=%s msg=%s\n", ev.Event, ev.Message.Id)
				}
				if err := maybeResolveElicitation(ctx, client, ev, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
					return err
				}
				role := strings.ToLower(strings.TrimSpace(ev.Message.Role))
				if role == "assistant" {
					if sdk.IsElicitationPending(ev.Message) {
						continue
					}
					if shouldSkipByTime(ev.Message.CreatedAt, postStart) {
						continue
					}
					msgID, text, changed := buf.ApplyEvent(ev)
					if changed && strings.TrimSpace(text) != "" {
						if looksLikeProviderJSON(text) {
							continue
						}
						if shouldHoldElicitationText(msgID, text, heldElicitation) {
							continue
						}
						if looksLikeElicitationJSON(text) || looksLikeElicitationStream(text) {
							continue
						}
						if !seenAssistant && lastAssistant != "" && strings.TrimSpace(text) == lastAssistant {
							continue
						}
						normText := normalizeOutputText(text)
						if lastOutputText != "" && normText == lastOutputText && time.Since(lastOutputAt) < 2*time.Second {
							continue
						}
						if ts, ok := printedTextAt[normText]; ok && time.Since(ts) < 5*time.Second {
							continue
						}
						if !seenAssistant {
							if lastStatus != "" {
								fmt.Print("\n")
							}
							seenAssistant = true
						}
						if msgID != "" && msgID != lastMsgIDPrinted && (lineOpen || (outputOpen != nil && *outputOpen)) {
							fmt.Print("\n")
							lineOpen = false
						}
						if printDeltaText(lastPrinted, msgID, text) {
							lineOpen = true
							if outputOpen != nil {
								*outputOpen = true
							}
							hadOutput = true
							if normText != "" {
								printedTextAt[normText] = time.Now()
							}
							lastMsgIDPrinted = msgID
						}
						lastOutputText = normalizeOutputText(text)
						lastOutputAt = time.Now()
					}
					if ev.Message.Interim == 0 && seenAssistant {
						printed, wasElic := flushHeldIfNotElicitation(msgID, heldElicitation, lastPrinted)
						if printed {
							lineOpen = true
							fmt.Print("\n")
							hadOutput = true
						}
						if wasElic {
							if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
								return err
							} else if handled {
								return nil
							}
						}
						if lineOpen {
							fmt.Print("\n")
							lineOpen = false
						}
						markStatusSucceeded(&lastStatus, &statusStart)
						finalizeStatus(&lastStatus, &statusStart, &lineOpen)
						statusFinalized = true
						fmt.Print("\n")
						return nil
					}
				}
				if name := sdk.ToolName(ev); name != "" {
					phase := sdk.ToolPhase(ev)
					if phase == "" {
						phase = "event"
					}
					fmt.Printf("\n[tool] %s %s\n", phase, name)
				}
			}
		}
		if msgID != "" && !seenAssistant {
			if handled, err := handlePendingElicitation(ctx, client, convID, handledElicitations, elicitationDefault, lastElicitationPayload); err != nil {
				return err
			} else if handled {
				continue
			}
			status, errMsg, ok := pollTurnStatus(ctx, client, convID, msgID)
			if ok {
				if status != "" {
					renderStatus(&lastStatus, &statusStart, status)
				}
				if status == "failed" {
					if strings.TrimSpace(errMsg) != "" {
						finalizeStatus(&lastStatus, &statusStart, &lineOpen)
						statusFinalized = true
						return fmt.Errorf("turn failed: %s", strings.TrimSpace(errMsg))
					}
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					return fmt.Errorf("turn failed")
				}
				if status == "succeeded" && !seenAssistant && !hadOutput {
					if printed := printLastAssistant(ctx, client, convID, postStart); printed {
						if outputOpen != nil {
							*outputOpen = true
						}
						hadOutput = true
					}
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					fmt.Print("\n")
					return nil
				}
				if status == "canceled" {
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					return fmt.Errorf("turn canceled")
				}
			}
		}
		if ctx.Err() != nil {
			if !seenAssistant && !hadOutput {
				if printed := printLastAssistant(ctx, client, convID, postStart); printed {
					if outputOpen != nil {
						*outputOpen = true
					}
					hadOutput = true
					finalizeStatus(&lastStatus, &statusStart, &lineOpen)
					statusFinalized = true
					fmt.Print("\n")
					return nil
				}
			}
			return ctx.Err()
		}
	}
}

func printDeltaText(last map[string]string, msgID string, text string) bool {
	if last == nil {
		fmt.Print(text)
		return text != ""
	}
	id := strings.TrimSpace(msgID)
	if id == "" {
		fmt.Print(text)
		return text != ""
	}
	prev := last[id]
	if prev == text {
		return false
	}
	if strings.HasPrefix(text, prev) {
		delta := text[len(prev):]
		if delta != "" {
			fmt.Print(delta)
			last[id] = text
			return true
		}
		last[id] = text
		return false
	}
	fmt.Print(text)
	last[id] = text
	return text != ""
}

func shouldSkipByTime(createdAt time.Time, postStart time.Time) bool {
	if postStart.IsZero() || createdAt.IsZero() {
		return false
	}
	// Allow small clock skew between server/client.
	cutoff := postStart.Add(-2 * time.Second)
	return createdAt.Before(cutoff)
}

func printLastAssistant(ctx context.Context, client *sdk.Client, convID string, postStart time.Time) bool {
	conv, err := client.GetMessages(ctx, convID, "")
	if err != nil || conv == nil {
		return false
	}
	var last string
	for _, turn := range conv.Transcript {
		if turn == nil || turn.Message == nil {
			continue
		}
		for _, m := range turn.Message {
			if m == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
				continue
			}
			if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
				continue
			}
			if m.Content != nil && strings.TrimSpace(*m.Content) != "" {
				c := strings.TrimSpace(*m.Content)
				if looksLikeElicitationJSON(c) {
					continue
				}
				if shouldSkipByTime(m.CreatedAt, postStart) {
					continue
				}
				last = c
			}
		}
	}
	if strings.TrimSpace(last) == "" {
		return false
	}
	fmt.Print("\r" + last)
	return true
}

func fetchLastAssistantContent(ctx context.Context, client *sdk.Client, convID string) string {
	conv, err := client.GetMessages(ctx, convID, "")
	if err != nil || conv == nil {
		return ""
	}
	var last string
	for _, turn := range conv.Transcript {
		if turn == nil || turn.Message == nil {
			continue
		}
		for _, m := range turn.Message {
			if m == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(m.Role)) != "assistant" {
				continue
			}
			if m.ElicitationId != nil && strings.TrimSpace(*m.ElicitationId) != "" {
				continue
			}
			if m.Content != nil && strings.TrimSpace(*m.Content) != "" {
				c := strings.TrimSpace(*m.Content)
				if looksLikeElicitationJSON(c) {
					continue
				}
				last = c
			}
		}
	}
	return last
}

func normalizeOutputText(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func pollTurnStatus(ctx context.Context, client *sdk.Client, convID string, msgID string) (string, string, bool) {
	conv, err := client.GetMessages(ctx, convID, "")
	if err != nil || conv == nil {
		return "", "", false
	}
	for _, turn := range conv.Transcript {
		if turn == nil {
			continue
		}
		if strings.TrimSpace(turn.Id) == strings.TrimSpace(msgID) {
			errMsg := ""
			if turn.ErrorMessage != nil {
				errMsg = strings.TrimSpace(*turn.ErrorMessage)
			}
			return strings.ToLower(strings.TrimSpace(turn.Status)), errMsg, true
		}
		if turn.StartedByMessageId != nil && strings.TrimSpace(*turn.StartedByMessageId) == strings.TrimSpace(msgID) {
			errMsg := ""
			if turn.ErrorMessage != nil {
				errMsg = strings.TrimSpace(*turn.ErrorMessage)
			}
			return strings.ToLower(strings.TrimSpace(turn.Status)), errMsg, true
		}
		for _, m := range turn.Message {
			if m == nil || strings.TrimSpace(m.Id) == "" {
				continue
			}
			if strings.TrimSpace(m.Id) == strings.TrimSpace(msgID) {
				errMsg := ""
				if turn.ErrorMessage != nil {
					errMsg = strings.TrimSpace(*turn.ErrorMessage)
				}
				return strings.ToLower(strings.TrimSpace(turn.Status)), errMsg, true
			}
		}
	}
	return "", "", false
}

func renderStatus(last *string, started *time.Time, status string) {
	if last == nil || started == nil {
		return
	}
	if *last == "" {
		*last = status
		*started = time.Now()
		return
	}
	elapsed := time.Since(*started).Round(time.Second)
	if status == *last {
		fmt.Printf("\r[status] %s (%s)", *last, elapsed)
		return
	}
	fmt.Printf("\r[status] %s (%s)\n", *last, elapsed)
	*last = status
	*started = time.Now()
}

func finalizeStatus(last *string, started *time.Time, lineOpen *bool) {
	if last == nil || started == nil || *last == "" {
		return
	}
	if lineOpen != nil && *lineOpen {
		fmt.Print("\n")
		*lineOpen = false
	} else {
		// ensure status starts on a new line when assistant output was printed
		fmt.Print("\n")
	}
	elapsed := time.Since(*started).Round(time.Second)
	fmt.Printf("[status] %s (%s)\n", *last, elapsed)
	*last = ""
}

func markStatusSucceeded(last *string, started *time.Time) {
	if last == nil || *last == "" {
		return
	}
	switch strings.ToLower(strings.TrimSpace(*last)) {
	case "queued", "running":
		*last = "succeeded"
		if started != nil && started.IsZero() {
			*started = time.Now()
		}
	}
}

func streamStop(ev *sdk.StreamEventEnvelope) bool {
	if ev == nil || ev.Content == nil {
		if ev != nil && ev.Message != nil && ev.Message.Content != nil {
			raw := strings.TrimSpace(*ev.Message.Content)
			if strings.HasPrefix(raw, "{") {
				if isStreamStopJSON(raw) {
					return true
				}
			}
		}
		return false
	}
	typ, ok := ev.Content["type"].(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(typ) {
	case "message_stop":
		return true
	case "message_delta":
		if _, ok := ev.Content["stop_reason"]; ok {
			return true
		}
		if d, ok := ev.Content["delta"].(map[string]interface{}); ok {
			if _, ok := d["stop_reason"]; ok {
				return true
			}
		}
	}
	return false
}

func isStreamStopJSON(raw string) bool {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	typ, ok := payload["type"].(string)
	if !ok {
		return false
	}
	switch strings.TrimSpace(typ) {
	case "message_stop":
		return true
	case "message_delta":
		if _, ok := payload["stop_reason"]; ok {
			return true
		}
		if d, ok := payload["delta"].(map[string]interface{}); ok {
			if _, ok := d["stop_reason"]; ok {
				return true
			}
		}
	}
	return false
}

func looksLikeProviderJSON(text string) bool {
	raw := strings.TrimSpace(text)
	if !strings.HasPrefix(raw, "{") {
		return false
	}
	// OpenAI Responses stream envelopes
	if strings.Contains(raw, "\"type\":\"response.") || strings.Contains(raw, "\"type\":\"response_") {
		return true
	}
	// Anthropic stream envelopes
	if strings.Contains(raw, "\"type\":\"content_block_") || strings.Contains(raw, "\"type\":\"message_") {
		return true
	}
	return false
}

func looksLikeElicitationJSON(text string) bool {
	raw := strings.TrimSpace(text)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "```"))
		if strings.HasPrefix(raw, "json") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}
	if !strings.HasPrefix(raw, "{") {
		return false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	typ, _ := payload["type"].(string)
	return strings.EqualFold(strings.TrimSpace(typ), "elicitation")
}

func looksLikeElicitationStream(text string) bool {
	raw := strings.TrimSpace(text)
	if raw == "" {
		return false
	}
	if strings.HasPrefix(raw, "```json") || strings.HasPrefix(raw, "```") {
		if strings.Contains(strings.ToLower(raw), "\"type\"") {
			return true
		}
	}
	if strings.Contains(strings.ToLower(raw), "\"requestedschema\"") {
		return true
	}
	if strings.Contains(raw, "\"type\"") && strings.Contains(raw, "elicitation") {
		return true
	}
	return false
}

func shouldHoldElicitationText(msgID string, text string, held map[string]string) bool {
	if held == nil {
		return false
	}
	id := strings.TrimSpace(msgID)
	if id == "" {
		return false
	}
	raw := strings.TrimSpace(text)
	if raw == "" {
		return false
	}
	// If already holding, just update and keep holding.
	if _, ok := held[id]; ok {
		held[id] = raw
		return true
	}
	// Hold when the message starts like a JSON/JSON-fenced payload that could be elicitation.
	if strings.HasPrefix(raw, "```") {
		held[id] = raw
		return true
	}
	if strings.HasPrefix(raw, "{") {
		unwrapped := unwrapJSON(raw)
		if strings.HasPrefix(unwrapped, "{") {
			if strings.Contains(unwrapped, "\"type\"") || strings.Contains(unwrapped, "elicitation") || strings.Contains(unwrapped, "\"requestedSchema\"") {
				held[id] = raw
				return true
			}
			// If it starts with a JSON object but doesn't yet include type, hold briefly.
			if len(unwrapped) < 256 {
				held[id] = raw
				return true
			}
		}
	}
	return false
}

func flushHeldIfNotElicitation(msgID string, held map[string]string, last map[string]string) (bool, bool) {
	if held == nil {
		return false, false
	}
	id := strings.TrimSpace(msgID)
	if id == "" {
		return false, false
	}
	raw, ok := held[id]
	if !ok {
		return false, false
	}
	delete(held, id)
	if looksLikeElicitationJSON(raw) {
		return false, true
	}
	if strings.TrimSpace(raw) == "" {
		return false, false
	}
	fmt.Print(raw)
	if last != nil {
		last[id] = raw
	}
	return true, false
}

func (c *ChatCmd) resolveBaseURL(ctx context.Context) (string, []authProviderInfo, string, string, string, []string, error) {
	if strings.TrimSpace(c.API) != "" {
		baseURL := strings.TrimSpace(c.API)
		providers, _ := fetchAuthProviders(ctx, baseURL)
		meta, _ := fetchWorkspaceMetadata(ctx, baseURL)
		workspaceRoot := ""
		defaultAgent := ""
		defaultModel := ""
		var models []string
		if meta != nil {
			workspaceRoot = meta.WorkspaceRoot
			defaultAgent = meta.DefaultAgent
			defaultModel = meta.DefaultModel
			models = meta.Models
		}
		return baseURL, providers, workspaceRoot, defaultAgent, defaultModel, models, nil
	}
	instances, err := detectLocalInstances(ctx)
	if err != nil {
		return "", nil, "", "", "", nil, err
	}
	if len(instances) == 0 {
		return "", nil, "", "", "", nil, fmt.Errorf("no local agently instance detected; use --api to specify the server URL")
	}
	if len(instances) == 1 {
		return instances[0].BaseURL, instances[0].Providers, instances[0].WorkspaceRoot, instances[0].DefaultAgent, instances[0].DefaultModel, instances[0].Models, nil
	}
	fmt.Println("Detected Agently instances:")
	for i, inst := range instances {
		root := strings.TrimSpace(inst.WorkspaceRoot)
		if root == "" {
			root = "<unknown>"
		}
		fmt.Printf("  %d) %s (workspace: %s)\n", i+1, inst.BaseURL, root)
	}
	fmt.Print("Select instance [1]: ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return instances[0].BaseURL, instances[0].Providers, instances[0].WorkspaceRoot, instances[0].DefaultAgent, instances[0].DefaultModel, instances[0].Models, nil
	}
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > len(instances) {
		return "", nil, "", "", "", nil, fmt.Errorf("invalid selection")
	}
	inst := instances[choice-1]
	return inst.BaseURL, inst.Providers, inst.WorkspaceRoot, inst.DefaultAgent, inst.DefaultModel, inst.Models, nil
}

func pickModel(defaultModel string, models []string) string {
	def := strings.TrimSpace(defaultModel)
	if def != "" {
		for _, m := range models {
			if strings.TrimSpace(m) == def {
				return def
			}
		}
	}
	for _, m := range models {
		if strings.TrimSpace(m) != "" {
			return strings.TrimSpace(m)
		}
	}
	return ""
}

func (c *ChatCmd) ensureAuth(ctx context.Context, client *sdk.Client, providers []authProviderInfo) error {
	if _, err := client.AuthMe(ctx); err == nil {
		return nil
	}
	// Try local login first (best effort). This keeps `agently chat` zero-config
	// when local auth is enabled.
	name := strings.TrimSpace(c.User)
	if name == "" {
		if local := findProvider(providers, "local"); local != nil {
			name = strings.TrimSpace(local.DefaultUsername)
		}
	}
	if name == "" {
		name = "devuser"
	}
	if err := client.AuthLocalLogin(ctx, name); err == nil {
		return nil
	}
	if len(providers) == 0 {
		// No provider discovery; fall through to explicit auth options.
	} else if local := findProvider(providers, "local"); local != nil {
		// local provider advertised, but login failed; continue to other modes.
	}
	if c.OOB {
		if strings.TrimSpace(c.OAuthSec) == "" {
			return fmt.Errorf("--oob requires --oauth-secrets")
		}
		scopes := parseScopes(c.OAuthScp)
		if err := client.AuthOOBSession(ctx, strings.TrimSpace(c.OAuthSec), scopes); err != nil {
			return err
		}
		if _, err := client.AuthMe(ctx); err != nil {
			return fmt.Errorf("oauth login succeeded, but session was not established")
		}
		return nil
	}
	if envSec := strings.TrimSpace(os.Getenv("AGENTLY_OOB_SECRETS")); envSec != "" {
		scopes := parseScopes(os.Getenv("AGENTLY_OOB_SCOPES"))
		if err := client.AuthOOBSession(ctx, envSec, scopes); err == nil {
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
	}
	if findProvider(providers, "bff") != nil {
		return c.runBrowserOAuth(ctx, client)
	}
	// If providers are missing/blocked, still attempt BFF browser flow.
	if err := c.runBrowserOAuth(ctx, client); err == nil {
		return nil
	} else if err != nil {
		return err
	}
	return fmt.Errorf("authentication required: provide --token or use --oob / AGENTLY_OOB_SECRETS")
}

func (c *ChatCmd) runBrowserOAuth(ctx context.Context, client *sdk.Client) error {
	fmt.Println("[auth] starting browser OAuth (PKCE)...")
	if err := client.AuthBrowserSession(ctx); err != nil {
		return err
	}
	if _, err := client.AuthMe(ctx); err != nil {
		return fmt.Errorf("oauth login succeeded, but session was not established")
	}
	return nil
}

func findProvider(providers []authProviderInfo, typ string) *authProviderInfo {
	for _, p := range providers {
		if strings.EqualFold(strings.TrimSpace(p.Type), strings.TrimSpace(typ)) {
			return &p
		}
	}
	return nil
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func maybeResolveElicitation(ctx context.Context, client *sdk.Client, ev *sdk.StreamEventEnvelope, handled map[string]struct{}, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) error {
	if ev == nil || ev.Message == nil {
		return nil
	}
	if !sdk.IsElicitationPending(ev.Message) {
		return nil
	}
	el := sdk.ElicitationFromEvent(ev)
	if el == nil || strings.TrimSpace(el.ElicitationID) == "" {
		return nil
	}
	if _, ok := handled[el.ElicitationID]; ok {
		return nil
	}
	handled[el.ElicitationID] = struct{}{}
	return resolveElicitation(ctx, client, el.ConversationID, el, defaultPayload, seedPayload)
}

func handlePendingElicitation(ctx context.Context, client *sdk.Client, convID string, handled map[string]struct{}, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (bool, error) {
	if client == nil {
		return false, nil
	}
	el, err := findPendingElicitation(ctx, client, convID, handled)
	if err != nil || el == nil {
		return false, err
	}
	if _, ok := handled[el.ElicitationID]; ok {
		return false, nil
	}
	handled[el.ElicitationID] = struct{}{}
	if err := resolveElicitation(ctx, client, convID, el, defaultPayload, seedPayload); err != nil {
		return false, err
	}
	return true, nil
}

func findPendingElicitation(ctx context.Context, client *sdk.Client, convID string, handled map[string]struct{}) (*sdk.Elicitation, error) {
	els, err := client.ListPendingElicitations(ctx, convID)
	if err != nil {
		return nil, err
	}
	var chosen *sdk.Elicitation
	for _, el := range els {
		if el == nil {
			continue
		}
		if strings.TrimSpace(el.ElicitationID) == "" {
			continue
		}
		if handled != nil {
			if _, ok := handled[el.ElicitationID]; ok {
				continue
			}
		}
		if chosen == nil || el.CreatedAt.After(chosen.CreatedAt) {
			chosen = el
		}
	}
	return chosen, nil
}

func resolveElicitation(ctx context.Context, client *sdk.Client, convID string, el *sdk.Elicitation, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) error {
	if el == nil {
		return nil
	}
	req := planElicitationFromSDK(el)
	if req == nil {
		return nil
	}
	if seedPayload != nil {
		applyElicitationDefaults(req, *seedPayload)
	}
	convID = strings.TrimSpace(convID)
	if convID == "" {
		convID = strings.TrimSpace(el.ConversationID)
	}
	if !stdinIsTTY() {
		if len(defaultPayload) == 0 {
			return fmt.Errorf("elicitation required; run interactively or provide --elicitation-default")
		}
		return client.ResolveElicitation(ctx, convID, el.ElicitationID, "accept", defaultPayload, "")
	}
	res, err := newStdinAwaiter().AwaitElicitation(ctx, req)
	if err != nil {
		return err
	}
	if res == nil {
		return nil
	}
	switch res.Action {
	case plan.ElicitResultActionAccept:
		if len(res.Payload) == 0 && strings.TrimSpace(req.Url) != "" {
			// URL-based flow: resolution comes from UI callback.
			return nil
		}
		if seedPayload != nil {
			mergePayload(seedPayload, res.Payload)
		}
		return client.ResolveElicitation(ctx, convID, el.ElicitationID, "accept", res.Payload, "")
	case plan.ElicitResultActionDecline:
		return client.ResolveElicitation(ctx, convID, el.ElicitationID, "decline", nil, strings.TrimSpace(res.Reason))
	default:
		return nil
	}
}

func planElicitationFromSDK(el *sdk.Elicitation) *plan.Elicitation {
	if el == nil {
		return nil
	}
	req := &plan.Elicitation{}
	if el.Request != nil {
		if raw, err := json.Marshal(el.Request); err == nil {
			_ = json.Unmarshal(raw, req)
		}
	} else if strings.TrimSpace(el.Content) != "" {
		raw := strings.TrimSpace(el.Content)
		raw = unwrapJSON(raw)
		_ = json.Unmarshal([]byte(raw), req)
	}
	if strings.TrimSpace(req.RequestedSchema.Type) == "" && len(req.RequestedSchema.Properties) > 0 {
		req.RequestedSchema.Type = "object"
	}
	if strings.TrimSpace(req.Message) == "" {
		req.Message = strings.TrimSpace(el.Content)
	}
	if strings.TrimSpace(req.ElicitationId) == "" {
		req.ElicitationId = strings.TrimSpace(el.ElicitationID)
	}
	return req
}

func applyElicitationDefaults(req *plan.Elicitation, seed map[string]interface{}) {
	if req == nil || len(seed) == 0 {
		return
	}
	props := req.RequestedSchema.Properties
	if len(props) == 0 {
		return
	}
	for k, v := range seed {
		prop, ok := props[k]
		if !ok {
			continue
		}
		mp, ok := prop.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := mp["default"]; !has {
			mp["default"] = v
		}
		props[k] = mp
	}
}

func mergePayload(dst *map[string]interface{}, payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if dst == nil {
		return
	}
	if *dst == nil {
		*dst = map[string]interface{}{}
	}
	for k, v := range payload {
		(*dst)[k] = v
	}
}

func logTurnEvent(w io.Writer, ev *sdk.TurnEvent) {
	if w == nil || ev == nil {
		return
	}
	fmt.Fprintf(w, "event=%s turn=%s msg=%s role=%s\n", ev.Type, ev.TurnID, ev.MessageID, ev.Role)
	if ev.Type == sdk.TurnEventDelta && strings.TrimSpace(ev.Text) != "" {
		snippet := ev.Text
		if len(snippet) > 80 {
			snippet = snippet[len(snippet)-80:]
		}
		fmt.Fprintf(w, "delta len=%d tail=%q\n", len(ev.Text), snippet)
	}
}

func logElicitationEvent(w io.Writer, ev *sdk.TurnEvent) {
	if w == nil || ev == nil || ev.Elicitation == nil {
		return
	}
	fmt.Fprintf(w, "elicitation id=%s\n", ev.Elicitation.ElicitationID)
	if len(ev.Elicitation.Request) > 0 {
		if b, err := json.Marshal(ev.Elicitation.Request); err == nil {
			fmt.Fprintf(w, "elicitation request=%s\n", string(b))
		}
	}
}

func unwrapJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		raw = strings.TrimPrefix(raw, "```")
		raw = strings.TrimSpace(raw)
		if strings.HasPrefix(raw, "json") {
			raw = strings.TrimSpace(strings.TrimPrefix(raw, "json"))
		}
		if idx := strings.LastIndex(raw, "```"); idx >= 0 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}
	return raw
}

func parseJSONArg(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "@") {
		b, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func stdinIsTTY() bool {
	if v := strings.TrimSpace(os.Getenv("AGENTLY_FORCE_NO_TTY")); v != "" {
		if strings.EqualFold(v, "1") || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes") {
			return false
		}
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
