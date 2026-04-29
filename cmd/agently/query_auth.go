package agently

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/viant/agently-core/sdk"
)

func (c *ChatCmd) resolveBaseURL(ctx context.Context) (string, []authProviderInfo, string, string, string, []string, error) {
	if strings.TrimSpace(c.API) != "" {
		baseURL := strings.TrimSpace(c.API)
		providers, _ := fetchAuthProviders(ctx, baseURL)
		meta, _ := fetchWorkspaceMetadata(ctx, baseURL)
		if meta == nil {
			return baseURL, providers, "", "", "", nil, nil
		}
		c.elicitationTimeout = meta.ElicitationTimeout
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
		c.elicitationTimeout = inst.ElicitationTimeout
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
	line, cancelled, rerr := readPromptLine(ctx, reader)
	if rerr != nil {
		return "", nil, "", "", "", nil, fmt.Errorf("read selection: %w", rerr)
	}
	if cancelled || line == "" {
		inst := instances[0]
		c.elicitationTimeout = inst.ElicitationTimeout
		return inst.BaseURL, inst.Providers, inst.WorkspaceRoot, inst.DefaultAgent, inst.DefaultModel, inst.Models, nil
	}
	choice, err := strconv.Atoi(line)
	if err != nil || choice < 1 || choice > len(instances) {
		return "", nil, "", "", "", nil, fmt.Errorf("invalid selection")
	}
	inst := instances[choice-1]
	c.elicitationTimeout = inst.ElicitationTimeout
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
	if err := tryTokenAuth(ctx, client, c.Token); err == nil {
		return nil
	}
	hasBFF := findProvider(providers, "bff") != nil
	if strings.TrimSpace(c.OOB) != "" {
		return c.authenticateWithOOB(ctx, client, strings.TrimSpace(c.OOB), parseScopes(c.OAuthScp))
	}
	if envSec := strings.TrimSpace(os.Getenv("AGENTLY_OOB_SECRETS")); envSec != "" {
		if err := c.authenticateWithOOB(ctx, client, envSec, parseScopes(os.Getenv("AGENTLY_OOB_SCOPES"))); err == nil {
			if _, err := client.AuthMe(ctx); err == nil {
				return nil
			}
		}
	}
	if hasBFF {
		if err := client.AuthBrowserSession(ctx); err != nil {
			return fmt.Errorf("authorization required: browser login failed: %w", err)
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
		// No advertised auth providers means the server is operating without an
		// interactive auth flow; allow the request path to continue and let the
		// API itself return 401/403 when applicable.
		return nil
	}
	if localErr != nil {
		return fmt.Errorf("authorization required: local login failed: %w", localErr)
	}
	return fmt.Errorf("authorization required")
}

func (c *ChatCmd) authenticateWithOOB(ctx context.Context, client *sdk.HTTPClient, secretRef string, scopes []string) error {
	return authenticateWithOOB(ctx, client, secretRef, strings.TrimSpace(c.OAuthCfg), scopes)
}

func findProvider(providers []authProviderInfo, kind string) *authProviderInfo {
	for i := range providers {
		if strings.EqualFold(strings.TrimSpace(providers[i].Type), strings.TrimSpace(kind)) {
			return &providers[i]
		}
	}
	return nil
}
