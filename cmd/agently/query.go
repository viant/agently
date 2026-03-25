package agently

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	coreplan "github.com/viant/agently-core/protocol/agent/plan"
	"github.com/viant/agently-core/protocol/prompt"
	streamingrt "github.com/viant/agently-core/runtime/streaming"
	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
	authtransport "github.com/viant/mcp/client/auth/transport"
)

// ChatCmd handles interactive/chat queries.
type ChatCmd struct {
	AgentID   string   `short:"a" long:"agent-id" description:"agent id"`
	Query     []string `short:"q" long:"query"    description:"user query (repeatable)"`
	ConvID    string   `short:"c" long:"conv"     description:"conversation ID (optional)"`
	ResetLogs bool     `long:"reset-logs" description:"truncate/clean log files before each run"`
	Timeout   int      `short:"t" long:"timeout" description:"timeout in seconds for the agent response (0=none)"`
	User      string   `short:"u" long:"user" description:"user id for the chat" default:"devuser"`
	API       string   `long:"api" description:"Agently base URL (skips auto-detect)"`
	Token     string   `long:"token" description:"Bearer token for API requests (overrides AGENTLY_TOKEN)"`
	OOB       string   `long:"oob" description:"Use server-side OAuth2 out-of-band login with the supplied secrets URL"`
	OAuthCfg  string   `long:"oauth-config" description:"scy OAuth config URL for client-side OOB login (unused for server OOB)"`
	OAuthScp  string   `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	Stream    bool     `long:"stream" description:"reserved for compatibility; CLI output streams automatically"`
	ElicitDef string   `long:"elicitation-default" description:"JSON or @file to auto-accept elicitations when stdin is not a TTY"`
	Context   string   `long:"context" description:"inline JSON object or @file with context data"`
	Attach    []string `long:"attach" description:"file to attach (repeatable). Format: <path>"`
}

func (c *ChatCmd) Execute(_ []string) error {
	if strings.TrimSpace(c.AgentID) == "" {
		c.AgentID = "chatter"
	}

	contextData, err := parseContextArg(c.Context)
	if err != nil {
		return err
	}
	attachments, err := parseAttachments(c.Attach)
	if err != nil {
		return err
	}
	defaultElicitationPayload, err := parseJSONArg(c.ElicitDef)
	if err != nil {
		return fmt.Errorf("parse --elicitation-default: %w", err)
	}

	ctxBase := context.Background()
	baseURL, providers, workspaceRoot, defaultAgent, defaultModel, models, err := c.resolveBaseURL(ctxBase)
	if err != nil {
		return err
	}
	if strings.TrimSpace(defaultAgent) != "" && strings.TrimSpace(c.AgentID) == "chatter" {
		c.AgentID = strings.TrimSpace(defaultAgent)
	}

	httpClient := &http.Client{Jar: cliCookieJar()}
	if c.Timeout > 0 {
		httpClient.Timeout = time.Duration(c.Timeout) * time.Second
	}
	opts := []sdk.HTTPOption{sdk.WithHTTPClient(httpClient)}
	if token := resolvedToken(c.Token); token != "" {
		opts = append(opts, sdk.WithAuthToken(token))
	}
	client, err := sdk.NewHTTP(baseURL, opts...)
	if err != nil {
		return err
	}
	if err := c.ensureAuth(ctxBase, client, providers); err != nil {
		return err
	}
	// Protected servers may only expose workspace metadata after auth.
	if strings.TrimSpace(defaultModel) == "" || len(models) == 0 || strings.TrimSpace(defaultAgent) == "" {
		if meta, err := client.GetWorkspaceMetadata(ctxBase); err == nil && meta != nil {
			if strings.TrimSpace(defaultAgent) == "" {
				defaultAgent = strings.TrimSpace(meta.DefaultAgent)
				if defaultAgent == "" && meta.Defaults != nil {
					defaultAgent = strings.TrimSpace(meta.Defaults.Agent)
				}
			}
			if strings.TrimSpace(defaultModel) == "" {
				defaultModel = strings.TrimSpace(meta.DefaultModel)
				if defaultModel == "" && meta.Defaults != nil {
					defaultModel = strings.TrimSpace(meta.Defaults.Model)
				}
			}
			if len(models) == 0 {
				models = append([]string(nil), meta.Models...)
			}
		}
	}
	if strings.TrimSpace(defaultAgent) != "" && strings.TrimSpace(c.AgentID) == "chatter" {
		c.AgentID = strings.TrimSpace(defaultAgent)
	}
	modelOverride := pickModel(defaultModel, models)
	if strings.TrimSpace(modelOverride) == "" {
		return fmt.Errorf("server metadata did not provide a default model")
	}

	if strings.TrimSpace(workspaceRoot) != "" {
		fmt.Printf("[workspace] %s\n", workspaceRoot)
	} else {
		fmt.Printf("[workspace] <unknown>\n")
	}
	if modelOverride != "" {
		fmt.Printf("[agent] %s [model] %s\n", c.AgentID, modelOverride)
	} else {
		fmt.Printf("[agent] %s\n", c.AgentID)
	}

	convID := strings.TrimSpace(c.ConvID)
	var lastElicitationPayload map[string]interface{}
	sentAttachments := false

	runQuery := func(query string) error {
		query = strings.TrimSpace(query)
		if query == "" {
			return nil
		}
		ctx := ctxBase
		if c.Timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, time.Duration(c.Timeout)*time.Second)
			defer cancel()
		}
		input := &agentsvc.QueryInput{
			AgentID:        c.AgentID,
			ConversationID: convID,
			Query:          query,
			UserId:         strings.TrimSpace(c.User),
			Context:        buildQueryContext(contextData, defaultElicitationPayload, lastElicitationPayload),
			ModelOverride:  modelOverride,
		}
		if !sentAttachments && len(attachments) > 0 {
			input.Attachments = attachments
			sentAttachments = true
		}
		out, _, err := c.executeQuery(ctx, client, input, defaultElicitationPayload, &lastElicitationPayload)
		if err != nil {
			return err
		}
		convID = strings.TrimSpace(out.ConversationID)
		return nil
	}

	if len(c.Query) > 0 {
		for _, query := range c.Query {
			if err := runQuery(query); err != nil {
				return err
			}
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
			return nil
		}
		if err := runQuery(line); err != nil {
			return err
		}
	}
}

func parseContextArg(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		content, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, fmt.Errorf("read context file: %w", err)
		}
		data = content
	}
	result := map[string]interface{}{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse context JSON: %w", err)
	}
	return result, nil
}

func parseAttachments(values []string) ([]*prompt.Attachment, error) {
	var out []*prompt.Attachment
	for _, item := range values {
		path := strings.TrimSpace(item)
		if path == "" {
			return nil, fmt.Errorf("attachment path is required")
		}
		mimeType := mime.TypeByExtension(filepath.Ext(path))
		if strings.TrimSpace(mimeType) == "" {
			return nil, fmt.Errorf("unsupported attachment type for %q", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read attachment %q: %w", path, err)
		}
		out = append(out, &prompt.Attachment{
			Name: filepath.Base(path),
			Mime: mimeType,
			Data: data,
		})
	}
	return out, nil
}

func resolvedToken(flagValue string) string {
	if token := strings.TrimSpace(flagValue); token != "" {
		return token
	}
	return strings.TrimSpace(os.Getenv("AGENTLY_TOKEN"))
}

func parseJSONArg(raw string) (map[string]interface{}, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	data := []byte(raw)
	if strings.HasPrefix(raw, "@") {
		content, err := os.ReadFile(strings.TrimPrefix(raw, "@"))
		if err != nil {
			return nil, err
		}
		data = content
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func cliCookieJar() http.CookieJar {
	dir := cliStateDir()
	_ = os.MkdirAll(dir, 0o700)
	path := filepath.Join(dir, "cookies.json")
	jar, err := authtransport.NewFileJar(path)
	if err == nil && jar != nil {
		return jar
	}
	fallback, _ := cookiejar.New(nil)
	return fallback
}

func cliStateDir() string {
	if dir, err := os.UserConfigDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, "agently", "cli")
	}
	if dir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Join(dir, ".agently-cli")
	}
	return filepath.Join(os.TempDir(), "agently-cli")
}

func (c *ChatCmd) executeQuery(ctx context.Context, client *sdk.HTTPClient, input *agentsvc.QueryInput, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (*agentsvc.QueryOutput, bool, error) {
	if err := ensureConversation(ctx, client, input, strings.TrimSpace(input.Query)); err != nil {
		return nil, false, err
	}
	inlineElicitation := len(defaultPayload) > 0 || stdinIsTTY()
	if inlineElicitation {
		input.ElicitationMode = ""
	} else {
		input.ElicitationMode = "deferred"
	}
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	streamer, err := startChatStream(streamCtx, client, strings.TrimSpace(input.ConversationID))
	if err != nil {
		return nil, false, err
	}
	defer streamer.Close()

	startedAt := time.Now().UTC()
	resolverCtx, stopResolver := context.WithCancel(ctx)
	defer stopResolver()
	resolverErr := make(chan error, 1)
	if inlineElicitation {
		go watchPendingElicitations(resolverCtx, client, strings.TrimSpace(input.ConversationID), defaultPayload, seedPayload, resolverErr)
	}
	out, err := client.Query(ctx, input)
	if err != nil {
		return nil, false, err
	}
	select {
	case err := <-resolverErr:
		if err != nil {
			return nil, false, err
		}
	default:
	}
	stopResolver()
	if out == nil {
		return nil, false, fmt.Errorf("query returned no response")
	}
	if strings.TrimSpace(out.ConversationID) != "" {
		input.ConversationID = strings.TrimSpace(out.ConversationID)
	}
	if strings.TrimSpace(out.Content) != "" {
		streamer.Close()
		return out, streamer.Flush(out.Content), nil
	}
	elicitation := out.Elicitation
	if elicitation == nil && out.Plan != nil {
		elicitation = out.Plan.Elicitation
	}
	if elicitation != nil && !inlineElicitation {
		streamer.Close()
		return nil, false, fmt.Errorf("elicitation required; run interactively or provide --elicitation-default")
	}
	content, err := waitForAssistantContent(ctx, client, strings.TrimSpace(out.ConversationID), startedAt, defaultPayload, seedPayload)
	if err != nil {
		return nil, false, err
	}
	out.Content = content
	streamer.Close()
	return out, streamer.Flush(out.Content), nil
}

func ensureConversation(ctx context.Context, client *sdk.HTTPClient, input *agentsvc.QueryInput, title string) error {
	if input == nil {
		return fmt.Errorf("query input is required")
	}
	if strings.TrimSpace(input.ConversationID) != "" {
		return nil
	}
	conversation, err := client.CreateConversation(ctx, &sdk.CreateConversationInput{
		AgentID: strings.TrimSpace(input.AgentID),
		Title:   strings.TrimSpace(title),
	})
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	if conversation == nil || strings.TrimSpace(conversation.Id) == "" {
		return fmt.Errorf("create conversation returned no id")
	}
	input.ConversationID = strings.TrimSpace(conversation.Id)
	return nil
}

type chatStreamer struct {
	sub     streamingrt.Subscription
	done    chan struct{}
	content strings.Builder
	printed bool
}

func startChatStream(ctx context.Context, client *sdk.HTTPClient, conversationID string) (*chatStreamer, error) {
	sub, err := client.StreamEvents(ctx, &sdk.StreamEventsInput{ConversationID: conversationID})
	if err != nil {
		return nil, fmt.Errorf("stream events: %w", err)
	}
	streamer := &chatStreamer{
		sub:  sub,
		done: make(chan struct{}),
	}
	go streamer.consume()
	return streamer, nil
}

func (s *chatStreamer) consume() {
	defer close(s.done)
	if s == nil || s.sub == nil {
		return
	}
	for event := range s.sub.C() {
		if event == nil {
			continue
		}
		switch event.Type {
		case streamingrt.EventTypeTextDelta:
			if event.Content == "" {
				continue
			}
			fmt.Print(event.Content)
			s.content.WriteString(event.Content)
			s.printed = true
		case streamingrt.EventTypeError:
			if strings.TrimSpace(event.Error) != "" {
				fmt.Fprintf(os.Stderr, "[stream-error] %s\n", strings.TrimSpace(event.Error))
			}
		}
	}
}

func (s *chatStreamer) Flush(final string) bool {
	if s == nil {
		return false
	}
	streamed := s.content.String()
	final = strings.TrimSpace(final)
	if final == "" {
		if s.printed {
			fmt.Print("\n")
		}
		return s.printed
	}
	if streamed == "" {
		fmt.Print(final)
		fmt.Print("\n")
		s.printed = true
		return true
	}
	if strings.HasPrefix(final, streamed) {
		if remainder := final[len(streamed):]; remainder != "" {
			fmt.Print(remainder)
		}
		fmt.Print("\n")
		return true
	}
	if strings.TrimSpace(streamed) == final {
		fmt.Print("\n")
		return true
	}
	if normalizeCLIContent(streamed) == normalizeCLIContent(final) {
		fmt.Print("\n")
		return true
	}
	fmt.Print("\n")
	fmt.Print(final)
	fmt.Print("\n")
	return true
}

func normalizeCLIContent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func (s *chatStreamer) Close() {
	if s == nil {
		return
	}
	if s.sub != nil {
		_ = s.sub.Close()
	}
	if s.done != nil {
		select {
		case <-s.done:
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func waitForAssistantContent(ctx context.Context, client *sdk.HTTPClient, conversationID string, startedAt time.Time, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (string, error) {
	if strings.TrimSpace(conversationID) == "" {
		return "", nil
	}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if handled, err := handlePendingElicitation(ctx, client, conversationID, defaultPayload, seedPayload); err != nil {
				return "", err
			} else if handled {
				continue
			}
			transcript, err := client.GetTranscript(ctx, &sdk.GetTranscriptInput{
				ConversationID:    conversationID,
				IncludeModelCalls: false,
				IncludeToolCalls:  false,
			})
			if err != nil {
				return "", err
			}
			if transcript == nil || len(transcript.Turns) == 0 {
				continue
			}
			turn := transcript.Turns[len(transcript.Turns)-1]
			if turn == nil {
				continue
			}
			status := strings.ToLower(string(turn.Status))
			if status == "failed" {
				return "", fmt.Errorf("turn failed")
			}
			if status == "canceled" {
				return "", fmt.Errorf("turn canceled")
			}
			// Extract final assistant content from canonical state
			if turn.Assistant != nil && turn.Assistant.Final != nil {
				if content := strings.TrimSpace(turn.Assistant.Final.Content); content != "" {
					return content, nil
				}
			}
		}
	}
}

func handlePendingElicitation(ctx context.Context, client *sdk.HTTPClient, conversationID string, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) (bool, error) {
	rows, err := client.ListPendingElicitations(ctx, &sdk.ListPendingElicitationsInput{ConversationID: conversationID})
	if err != nil {
		return false, err
	}
	var pending *sdk.PendingElicitation
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.ElicitationID) == "" {
			continue
		}
		if pending == nil || row.CreatedAt.After(pending.CreatedAt) {
			pending = row
		}
	}
	if pending == nil {
		return false, nil
	}
	req := plannedElicitationFromPending(pending)
	if req == nil {
		return false, nil
	}
	if err := resolvePlannedElicitation(ctx, client, conversationID, req, defaultPayload, seedPayload); err != nil {
		return false, err
	}
	return true, nil
}

func watchPendingElicitations(ctx context.Context, client *sdk.HTTPClient, conversationID string, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}, errs chan<- error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	resolved := map[string]struct{}{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rows, err := client.ListPendingElicitations(ctx, &sdk.ListPendingElicitationsInput{ConversationID: conversationID})
			if err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
			var pending *sdk.PendingElicitation
			for _, row := range rows {
				if row == nil || strings.TrimSpace(row.ElicitationID) == "" {
					continue
				}
				elicitationID := strings.TrimSpace(row.ElicitationID)
				if _, ok := resolved[elicitationID]; ok {
					continue
				}
				if pending == nil || row.CreatedAt.After(pending.CreatedAt) {
					pending = row
				}
			}
			if pending == nil {
				continue
			}
			req := plannedElicitationFromPending(pending)
			if req == nil {
				continue
			}
			resolved[strings.TrimSpace(req.ElicitationId)] = struct{}{}
			if err := resolvePlannedElicitation(ctx, client, conversationID, req, defaultPayload, seedPayload); err != nil {
				select {
				case errs <- err:
				default:
				}
				return
			}
		}
	}
}

func plannedElicitationFromPending(input *sdk.PendingElicitation) *coreplan.Elicitation {
	if input == nil {
		return nil
	}
	req := &coreplan.Elicitation{}
	if len(input.Elicitation) > 0 {
		if data, err := json.Marshal(input.Elicitation); err == nil {
			_ = json.Unmarshal(data, req)
		}
	}
	if strings.TrimSpace(req.Message) == "" {
		req.Message = strings.TrimSpace(input.Content)
	}
	if strings.TrimSpace(req.ElicitationId) == "" {
		req.ElicitationId = strings.TrimSpace(input.ElicitationID)
	}
	if strings.TrimSpace(req.RequestedSchema.Type) == "" && len(req.RequestedSchema.Properties) == 0 {
		return nil
	}
	if strings.TrimSpace(req.RequestedSchema.Type) == "" {
		req.RequestedSchema.Type = "object"
	}
	return req
}

func resolvePlannedElicitation(ctx context.Context, client *sdk.HTTPClient, conversationID string, req *coreplan.Elicitation, defaultPayload map[string]interface{}, seedPayload *map[string]interface{}) error {
	if req == nil || strings.TrimSpace(req.ElicitationId) == "" {
		return nil
	}
	if seedPayload != nil {
		applyElicitationDefaults(req, *seedPayload)
	}
	if len(defaultPayload) > 0 {
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "accept",
			Payload:        defaultPayload,
		})
	}
	if !stdinIsTTY() {
		return fmt.Errorf("elicitation required; run interactively or provide --elicitation-default")
	}
	result, err := awaitCoreElicitation(ctx, req)
	if err != nil || result == nil {
		return err
	}
	switch result.Action {
	case coreplan.ElicitResultActionAccept:
		if seedPayload != nil {
			mergePayload(seedPayload, result.Payload)
		}
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "accept",
			Payload:        result.Payload,
		})
	case coreplan.ElicitResultActionDecline:
		return client.ResolveElicitation(ctx, &sdk.ResolveElicitationInput{
			ConversationID: conversationID,
			ElicitationID:  req.ElicitationId,
			Action:         "decline",
		})
	default:
		return nil
	}
}

func applyElicitationDefaults(req *coreplan.Elicitation, seed map[string]interface{}) {
	if req == nil || len(seed) == 0 {
		return
	}
	for key, value := range seed {
		property, ok := req.RequestedSchema.Properties[key]
		if !ok {
			continue
		}
		asMap, ok := property.(map[string]interface{})
		if !ok {
			continue
		}
		if _, exists := asMap["default"]; !exists {
			asMap["default"] = value
		}
	}
}

func mergePayload(dst *map[string]interface{}, src map[string]interface{}) {
	if dst == nil || len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]interface{}{}
	}
	for key, value := range src {
		(*dst)[key] = value
	}
}

func buildQueryContext(base map[string]interface{}, defaultPayload, seedPayload map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range defaultPayload {
		out[key] = value
	}
	for key, value := range seedPayload {
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && (info.Mode()&os.ModeCharDevice) != 0
}

func (c *ChatCmd) resolveBaseURL(ctx context.Context) (string, []authProviderInfo, string, string, string, []string, error) {
	if strings.TrimSpace(c.API) != "" {
		baseURL := strings.TrimSpace(c.API)
		providers, _ := fetchAuthProviders(ctx, baseURL)
		meta, _ := fetchWorkspaceMetadata(ctx, baseURL)
		if meta == nil {
			return baseURL, providers, "", "", "", nil, nil
		}
		return baseURL, providers, meta.WorkspaceRoot, meta.DefaultAgent, meta.DefaultModel, meta.Models, nil
	}
	instances, err := detectLocalInstances(ctx)
	if err != nil {
		return "", nil, "", "", "", nil, err
	}
	if len(instances) == 0 {
		return "", nil, "", "", "", nil, fmt.Errorf("no local agently instance detected; use --api to specify the server URL")
	}
	if len(instances) == 1 {
		inst := instances[0]
		return inst.BaseURL, inst.Providers, inst.WorkspaceRoot, inst.DefaultAgent, inst.DefaultModel, inst.Models, nil
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
		inst := instances[0]
		return inst.BaseURL, inst.Providers, inst.WorkspaceRoot, inst.DefaultAgent, inst.DefaultModel, inst.Models, nil
	}
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > len(instances) {
		return "", nil, "", "", "", nil, fmt.Errorf("invalid selection")
	}
	inst := instances[choice-1]
	return inst.BaseURL, inst.Providers, inst.WorkspaceRoot, inst.DefaultAgent, inst.DefaultModel, inst.Models, nil
}

func pickModel(defaultModel string, models []string) string {
	defaultModel = strings.TrimSpace(defaultModel)
	if defaultModel == "" {
		return ""
	}
	for _, item := range models {
		if strings.TrimSpace(item) == defaultModel {
			return defaultModel
		}
	}
	return defaultModel
}

func (c *ChatCmd) ensureAuth(ctx context.Context, client *sdk.HTTPClient, providers []authProviderInfo) error {
	hasBFF := findProvider(providers, "bff") != nil
	if strings.TrimSpace(c.OOB) != "" {
		if strings.TrimSpace(c.OOB) == "" {
			return fmt.Errorf("--oob requires a secrets URL value")
		}
		if err := client.AuthOOBSession(ctx, strings.TrimSpace(c.OOB), parseScopes(c.OAuthScp)); err != nil {
			return err
		}
		if _, err := client.AuthMe(ctx); err != nil {
			return fmt.Errorf("oauth login succeeded, but session was not established")
		}
		return nil
	}
	if envSec := strings.TrimSpace(os.Getenv("AGENTLY_OOB_SECRETS")); envSec != "" {
		if err := client.AuthOOBSession(ctx, envSec, parseScopes(os.Getenv("AGENTLY_OOB_SCOPES"))); err == nil {
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
	}
	if hasBFF {
		if err := client.AuthBrowserSession(ctx); err != nil {
			return fmt.Errorf("authentication required: browser login failed: %w", err)
		}
		if _, err := client.AuthMe(ctx); err != nil {
			return fmt.Errorf("browser login succeeded, but session was not established")
		}
		return nil
	}
	if _, err := client.AuthMe(ctx); err == nil {
		return nil
	}

	var localErr error
	if local := findProvider(providers, "local"); local != nil {
		name := strings.TrimSpace(c.User)
		if name == "" {
			name = strings.TrimSpace(local.DefaultUsername)
		}
		if name == "" {
			name = "devuser"
		}
		if err := client.AuthLocalLogin(ctx, name); err == nil {
			return nil
		} else {
			localErr = err
		}
	}

	if len(providers) == 0 {
		if err := client.AuthBrowserSession(ctx); err != nil {
			if localErr != nil {
				return fmt.Errorf("authentication required: local login failed: %v; browser login failed: %w", localErr, err)
			}
			return fmt.Errorf("authentication required: browser login failed: %w", err)
		}
		if _, err := client.AuthMe(ctx); err != nil {
			return fmt.Errorf("browser login succeeded, but session was not established")
		}
		return nil
	}
	if localErr != nil {
		return fmt.Errorf("authentication required: local login failed: %w", localErr)
	}
	return fmt.Errorf("authentication required")
}

func findProvider(providers []authProviderInfo, kind string) *authProviderInfo {
	for i := range providers {
		if strings.EqualFold(strings.TrimSpace(providers[i].Type), strings.TrimSpace(kind)) {
			return &providers[i]
		}
	}
	return nil
}

func parseScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	items := strings.Split(raw, ",")
	out := make([]string, 0, len(items))
	for _, item := range items {
		if value := strings.TrimSpace(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}
