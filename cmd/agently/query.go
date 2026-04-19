package agently

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/viant/agently-core/sdk"
	agentsvc "github.com/viant/agently-core/service/agent"
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
	OOB       string   `long:"oob" description:"Use local scy OAuth2 out-of-band login with the supplied secrets URL"`
	OAuthCfg  string   `long:"oauth-config" description:"Optional scy OAuth config URL override for client-side OOB login"`
	OAuthScp  string   `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	ElicitDef string   `long:"elicitation-default" description:"JSON or @file to auto-accept elicitations when stdin is not a TTY"`
	Context   string   `long:"context" description:"inline JSON object or @file with context data"`
	Attach    []string `long:"attach" description:"file to attach (repeatable). Format: <path>"`

	// elicitationTimeout is sourced from the resolved instance's workspace
	// defaults. Zero means fall back to defaultElicitationResponseTimeout.
	elicitationTimeout time.Duration
}

func (c *ChatCmd) Execute(_ []string) error {
	if strings.TrimSpace(c.AgentID) == "" {
		c.AgentID = "chatter"
	}
	if c.ResetLogs {
		if err := resetDebugArtifacts(); err != nil {
			return fmt.Errorf("reset debug logs: %w", err)
		}
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
	if strings.TrimSpace(defaultModel) == "" || len(models) == 0 || strings.TrimSpace(defaultAgent) == "" || strings.TrimSpace(workspaceRoot) == "" {
		if meta, err := client.GetWorkspaceMetadata(ctxBase); err == nil && meta != nil {
			if strings.TrimSpace(workspaceRoot) == "" {
				workspaceRoot = strings.TrimSpace(meta.WorkspaceRoot)
			}
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
		code, err := resolveConversationExitCode(ctxBase, client, convID)
		if err != nil {
			return err
		}
		if code != 0 {
			return &commandExitCode{code: code}
		}
		return nil
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, cancelled, rerr := readPromptLine(ctxBase, reader)
		if rerr != nil {
			return fmt.Errorf("read stdin: %w", rerr)
		}
		if cancelled || line == "" || line == "exit" || line == "quit" {
			fmt.Printf("[conversation-id] %s\n", convID)
			code, err := resolveConversationExitCode(ctxBase, client, convID)
			if err != nil {
				return err
			}
			if code != 0 {
				return &commandExitCode{code: code}
			}
			return nil
		}
		if err := runQuery(line); err != nil {
			return err
		}
	}
}

var (
	defaultDebugLogPath = "/tmp/agently-debug.log"
	envTraceFilePath    = "AGENTLY_DEBUG_TRACE_FILE"
	envPayloadDirPath   = "AGENTLY_DEBUG_PAYLOAD_DIR"
)

func resetDebugArtifacts() error {
	if err := truncateOrCreateFile(defaultDebugLogPath); err != nil {
		return err
	}
	if tracePath := strings.TrimSpace(os.Getenv(envTraceFilePath)); tracePath != "" {
		if err := truncateOrCreateFile(tracePath); err != nil {
			return err
		}
	}
	if payloadDir := strings.TrimSpace(os.Getenv(envPayloadDirPath)); payloadDir != "" {
		if err := os.RemoveAll(payloadDir); err != nil {
			return err
		}
		if err := os.MkdirAll(payloadDir, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func truncateOrCreateFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
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
