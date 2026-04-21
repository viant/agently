package agently

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/viant/agently-core/sdk"
)

type MCPRunCmd struct {
	Name     string `short:"n" long:"name" description:"Exact tool name to execute"`
	Args     string `short:"a" long:"args" description:"Inline JSON object or @file with tool arguments"`
	API      string `long:"api" description:"Server URL (skip local auto-detect)"`
	Token    string `long:"token" description:"Bearer token for API requests (overrides AGENTLY_TOKEN)"`
	Session  string `long:"session" description:"Session cookie value for API requests (agently_session)"`
	OOB      string `long:"oob" description:"Use local scy OAuth2 out-of-band login with the supplied secrets URL"`
	OAuthCfg string `long:"oauth-config" description:"Optional scy OAuth config URL override for client-side OOB login"`
	OAuthScp string `long:"oauth-scopes" description:"comma-separated OAuth scopes for OOB login"`
	JSON     bool   `long:"json" description:"Print result as JSON envelope instead of plain text"`
}

func (c *MCPRunCmd) Execute(_ []string) error {
	ctx := context.Background()
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return fmt.Errorf("--name is required")
	}

	args, err := parseJSONArg(c.Args)
	if err != nil {
		return fmt.Errorf("parse --args: %w", err)
	}
	if args == nil {
		args = map[string]interface{}{}
	}

	baseURL, err := resolveToolBaseURL(ctx, strings.TrimSpace(c.API))
	if err != nil {
		return fmt.Errorf("cannot find agently server: %w", err)
	}
	providers, _ := fetchAuthProviders(ctx, baseURL)

	httpClient := &http.Client{Jar: cliCookieJar()}
	opts := []sdk.HTTPOption{sdk.WithHTTPClient(httpClient)}
	if token := resolvedToken(c.Token); token != "" {
		opts = append(opts, sdk.WithAuthToken(token))
	}
	client, err := sdk.NewHTTP(baseURL, opts...)
	if err != nil {
		return fmt.Errorf("sdk client: %w", err)
	}
	if err := ensureToolAuth(ctx, client, providers, c.Token, c.Session, c.OOB, c.OAuthCfg, c.OAuthScp); err != nil {
		return err
	}

	execName, err := resolveExecutableToolName(ctx, client, name)
	if err != nil {
		return err
	}

	result, err := client.ExecuteTool(ctx, execName, args)
	if err != nil {
		return fmt.Errorf("run mcp tool: %w", err)
	}

	if c.JSON {
		payload := map[string]interface{}{
			"name":   execName,
			"input":  name,
			"args":   args,
			"result": result,
		}
		data, _ := json.MarshalIndent(payload, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	fmt.Println(result)
	return nil
}

func resolveExecutableToolName(ctx context.Context, client *sdk.HTTPClient, input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", fmt.Errorf("--name is required")
	}
	defs, err := client.ListToolDefinitions(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve tool name: %w", err)
	}
	if len(defs) == 0 {
		return name, nil
	}

	candidates := toolNameCandidates(name)
	defMap := map[string]string{}
	for _, def := range defs {
		defName := strings.TrimSpace(def.Name)
		if defName == "" {
			continue
		}
		for _, candidate := range toolNameCandidates(defName) {
			if _, ok := defMap[candidate]; !ok {
				defMap[candidate] = defName
			}
		}
	}
	for _, candidate := range candidates {
		if resolved, ok := defMap[candidate]; ok {
			return executableToolName(resolved), nil
		}
	}
	return executableToolName(name), nil
}

func toolNameCandidates(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(name)
	add(strings.Replace(name, ":", "/", 1))
	add(strings.Replace(name, ":", "-", 1))
	add(strings.Replace(name, "/", ":", 1))
	add(strings.Replace(name, "/", "-", 1))
	add(strings.Replace(name, "-", ":", 1))
	add(strings.Replace(name, "-", "/", 1))
	sort.Strings(out)
	return out
}

func executableToolName(name string) string {
	name = strings.TrimSpace(name)
	if strings.Contains(name, ":") {
		return strings.Replace(name, ":", "/", 1)
	}
	return name
}
