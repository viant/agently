package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/viant/agently-core/sdk"
)

// TranscriptCmd fetches a conversation transcript from a local or remote Agently server.
type TranscriptCmd struct {
	ConvID           string `short:"c" long:"conv" description:"conversation ID" required:"true"`
	API              string `long:"api" description:"Agently base URL (skips auto-detect)"`
	Token            string `long:"token" description:"Bearer token for API requests (overrides AGENTLY_TOKEN)"`
	OOB              string `long:"oob" description:"Use local scy OAuth2 out-of-band login with the supplied secrets URL"`
	OAuthCfg         string `long:"oauth-config" description:"Optional scy OAuth config URL override for client-side OOB login"`
	OAuthScp         string `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	User             string `short:"u" long:"user" description:"user id for local auth fallback" default:"devuser"`
	Since            string `long:"since" description:"only include transcript data after the specified message ID"`
	IncludeModelCall bool   `long:"include-model-calls" description:"include model call details"`
	IncludeToolCall  bool   `long:"include-tool-calls" description:"include tool call details"`
	Pretty           bool   `long:"pretty" description:"pretty-print JSON output"`
}

func (c *TranscriptCmd) Execute(_ []string) error {
	baseURL, providers, _, _, _, _, err := c.asChat().resolveBaseURL(context.Background())
	if err != nil {
		return err
	}

	httpClient := &http.Client{Jar: cliCookieJar()}
	opts := []sdk.HTTPOption{sdk.WithHTTPClient(httpClient)}
	if token := resolvedToken(c.Token); token != "" {
		opts = append(opts, sdk.WithAuthToken(token))
	}
	client, err := sdk.NewHTTP(baseURL, opts...)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := c.asChat().ensureAuth(ctx, client, providers); err != nil {
		return err
	}

	includeModelCalls := c.IncludeModelCall
	includeToolCalls := c.IncludeToolCall
	if !includeModelCalls && !includeToolCalls {
		includeModelCalls = true
		includeToolCalls = true
	}

	out, err := client.GetTranscript(ctx, &sdk.GetTranscriptInput{
		ConversationID:    strings.TrimSpace(c.ConvID),
		Since:             strings.TrimSpace(c.Since),
		IncludeModelCalls: includeModelCalls,
		IncludeToolCalls:  includeToolCalls,
	})
	if err != nil {
		return err
	}

	if c.Pretty {
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal transcript: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}
	data, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("marshal transcript: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func (c *TranscriptCmd) asChat() *ChatCmd {
	return &ChatCmd{
		API:      strings.TrimSpace(c.API),
		Token:    strings.TrimSpace(c.Token),
		OOB:      strings.TrimSpace(c.OOB),
		OAuthCfg: strings.TrimSpace(c.OAuthCfg),
		OAuthScp: strings.TrimSpace(c.OAuthScp),
		User:     strings.TrimSpace(c.User),
	}
}
