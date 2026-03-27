package server

import (
	"strings"

	svcscheduler "github.com/viant/agently-core/service/scheduler"
)

// LoadSchedulerUserCredAuthConfig returns the public scheduler auth settings
// required by agently-core to authorize legacy user_cred_url resources.
func LoadSchedulerUserCredAuthConfig(workspaceRoot string) (*svcscheduler.UserCredAuthConfig, error) {
	cfg, err := LoadWorkspaceAuthConfig(workspaceRoot)
	if err != nil {
		return nil, err
	}
	if cfg == nil || cfg.OAuth == nil || cfg.OAuth.Client == nil {
		return nil, nil
	}
	return &svcscheduler.UserCredAuthConfig{
		Mode:            strings.TrimSpace(cfg.OAuth.Mode),
		ClientConfigURL: strings.TrimSpace(cfg.OAuth.Client.ConfigURL),
		Scopes:          append([]string(nil), cfg.OAuth.Client.Scopes...),
	}, nil
}
