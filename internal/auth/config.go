package auth

import (
	"fmt"
	"strings"
)

// Config defines global authentication settings.
// Modes:
//   - local  : username-only with HttpOnly session cookie
//   - bff    : backend-for-frontend OAuth (PKCE) setting HttpOnly cookie
//   - oidc   : frontend obtains tokens and calls APIs with Bearer; server validates
//   - mixed  : accept both Bearer and cookie
type Config struct {
	Enabled         bool     `yaml:"enabled" json:"enabled"`
	CookieName      string   `yaml:"cookieName" json:"cookieName"`
	DefaultUsername string   `yaml:"defaultUsername" json:"defaultUsername"`
	IpHashKey       string   `yaml:"ipHashKey" json:"ipHashKey"`
	TrustedProxies  []string `yaml:"trustedProxies" json:"trustedProxies"`
	RedirectPath    string   `yaml:"redirectPath" json:"redirectPath"`
	// New unified model
	OAuth *OAuth `yaml:"oauth" json:"oauth"`
	Local *Local `yaml:"local" json:"local"`
}

// New unified structures
type OAuth struct {
	Mode   string       `yaml:"mode" json:"mode"` // bearer|spa|bff|mixed
	Name   string       `yaml:"name" json:"name"`
	Label  string       `yaml:"label" json:"label"`
	Client *OAuthClient `yaml:"client" json:"client"`
}

type OAuthClient struct {
	ConfigURL    string   `yaml:"configURL" json:"configURL"`       // for bff
	DiscoveryURL string   `yaml:"discoveryURL" json:"discoveryURL"` // for spa/bearer
	JWKSURL      string   `yaml:"jwksURL" json:"jwksURL"`           // for bearer verifier
	RedirectURI  string   `yaml:"redirectURI" json:"redirectURI"`
	ClientID     string   `yaml:"clientID" json:"clientID"`
	Scopes       []string `yaml:"scopes" json:"scopes"`
	Issuer       string   `yaml:"issuer" json:"issuer"`       // optional expected iss claim
	Audiences    []string `yaml:"audiences" json:"audiences"` // optional expected aud claim(s)
}

type Local struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// Environment-based loading removed. Auth config must come from the central
// workspace configuration (executor.Config). This package now only provides
// the struct and validation helpers.

// Validate checks internal consistency; when disabled minimal fields are required.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if strings.TrimSpace(c.IpHashKey) == "" {
		return fmt.Errorf("auth: IpHashKey is required when enabled")
	}
	// CookieName required when local.enabled or oauth.mode includes bff|mixed
	needsCookie := (c.Local != nil && c.Local.Enabled)
	if c.OAuth != nil {
		m := strings.ToLower(strings.TrimSpace(c.OAuth.Mode))
		if m == "bff" || m == "mixed" {
			needsCookie = true
		}
	}
	if needsCookie && strings.TrimSpace(c.CookieName) == "" {
		return fmt.Errorf("auth: CookieName required for cookie-based modes")
	}
	return nil
}

// IsLocalAuth returns true when auth is enabled and the effective mode is
// local-only (i.e. cookie-based session, no OAuth mode configured).
func (c *Config) IsLocalAuth() bool {
	if c == nil || !c.Enabled {
		return false
	}
	if c.Local == nil || !c.Local.Enabled {
		return false
	}
	if c.OAuth == nil {
		return true
	}
	m := strings.ToLower(strings.TrimSpace(c.OAuth.Mode))
	return m == "" || m == "local"
}

// IsCookieAccepted returns true when a session cookie is an acceptable auth
// credential given the current configuration.
func (c *Config) IsCookieAccepted() bool {
	if c == nil || !c.Enabled {
		return false
	}
	if c.Local != nil && c.Local.Enabled {
		return true
	}
	if c.OAuth != nil {
		m := strings.ToLower(strings.TrimSpace(c.OAuth.Mode))
		if m == "bff" || m == "mixed" {
			return true
		}
	}
	return false
}

// IsBearerAccepted returns true when a Bearer token is an acceptable auth
// credential given the current configuration.
func (c *Config) IsBearerAccepted() bool {
	if c == nil || !c.Enabled || c.OAuth == nil {
		return false
	}
	m := strings.ToLower(strings.TrimSpace(c.OAuth.Mode))
	switch m {
	case "spa", "bearer", "mixed":
		return true
	}
	return false
}
