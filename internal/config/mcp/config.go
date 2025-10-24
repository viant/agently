package mcp

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/viant/agently/internal/auth/shared"
)

// Defaults for MCP auth reuse when not configured.
const (
	BuiltInDefaultReuseAuthorizer     = true
	BuiltInDefaultReuseAuthorizerMode = string(shared.ModeBearerFirst)
)

// MinTTL holds threshold durations for proactive refresh.
type MinTTL struct {
	Access time.Duration `yaml:"access" json:"access"`
	ID     time.Duration `yaml:"id" json:"id"`
}

// StoragePolicy config for where to store tokens.
// Values: "memory" or "encrypted". Defaults: access=id=memory, refresh=encrypted.
type StoragePolicy struct {
	Access  string `yaml:"access" json:"access"`
	ID      string `yaml:"id" json:"id"`
	Refresh string `yaml:"refresh" json:"refresh"`
}

// ProviderAuth holds per-provider overrides and metadata.
type ProviderAuth struct {
	ReuseAuthorizer     *bool          `yaml:"reuseAuthorizer" json:"reuseAuthorizer"`
	ReuseAuthorizerMode *string        `yaml:"reuseAuthorizerMode" json:"reuseAuthorizerMode"`
	Authority           string         `yaml:"authority" json:"authority"`
	Audience            string         `yaml:"audience" json:"audience"`
	MinTTL              *MinTTL        `yaml:"minTTL" json:"minTTL"`
	Storage             *StoragePolicy `yaml:"storage" json:"storage"`
}

// Provider defines an MCP provider entry.
type Provider struct {
	Name string       `yaml:"name" json:"name"`
	Auth ProviderAuth `yaml:"auth" json:"auth"`
}

// MCPConfig is the top-level MCP configuration section.
type MCPConfig struct {
	Providers []Provider `yaml:"providers" json:"providers"`
}

// Env overrides
func EnvReuseAuthorizer() *bool {
	if v, ok := os.LookupEnv("MCP_REUSE_AUTHORIZER"); ok {
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		if err == nil {
			return &b
		}
	}
	return nil
}

func EnvReuseAuthorizerMode() *string {
	if v, ok := os.LookupEnv("MCP_REUSE_AUTHORIZER_MODE"); ok {
		s := strings.TrimSpace(v)
		// normalization and validation occur during resolution; accept raw value here
		return &s
	}
	return nil
}

// ResolveEffective computes effective reuse settings using precedence.
// Inputs can be nil to indicate unspecified values.
// ResolveEffective computes effective reuse settings using precedence, taking global pointers directly.
// globalReuse/globalMode correspond to default.mcp.reuseAuthorizer and default.mcp.reuseAuthorizerMode.
func ResolveEffective(ctx context.Context, clientReuse *bool, clientMode *string, cliReuse *bool, cliMode *string, envReuse *bool, envMode *string, provider ProviderAuth, globalReuse *bool, globalMode *string) (reuse bool, mode shared.Mode) {
	reuse = shared.ResolveReuseAuthorizer(shared.ReuseAuthorizerResolutionInput{
		Ctx:         ctx,
		ClientOpt:   clientReuse,
		CLIOpt:      cliReuse,
		EnvOpt:      envReuse,
		ProviderOpt: provider.ReuseAuthorizer,
		GlobalOpt:   globalReuse,
	})

	mode = shared.ResolveReuseAuthorizerMode(shared.ReuseModeResolutionInput{
		Ctx:         ctx,
		ClientOpt:   clientMode,
		CLIOpt:      cliMode,
		EnvOpt:      envMode,
		ProviderOpt: provider.ReuseAuthorizerMode,
		GlobalOpt:   globalMode,
	})
	return reuse, mode
}
