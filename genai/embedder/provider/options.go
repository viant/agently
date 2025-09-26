package provider

import "net/http"

type Options struct {
	Model          string   `yaml:"model,omitempty" json:"model,omitempty"`
	Provider       string   `yaml:"provider,omitempty" json:"provider,omitempty"`
	APIKeyURL      string   `yaml:"apiKeyURL,omitempty" json:"apiKeyURL,omitempty"`
	CredentialsURL string   `yaml:"credentialsURL,omitempty" json:"credentialsURL,omitempty"`
	URL            string   `yaml:"url,omitempty" json:"url,omitempty"`
	ProjectID      string   `yaml:"projectID,omitempty" json:"projectID,omitempty"`
	Location       string   `yaml:"location,omitempty" json:"location,omitempty"`
	Scopes         []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`

	httpClient    *http.Client                    `yaml:"-" json:"-"`
	usageListener func(data []string, tokens int) `yaml:"-" json:"-"` // usageListener is a callback function to handle token usage
}
