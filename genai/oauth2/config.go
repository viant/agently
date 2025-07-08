package oauth2

import "golang.org/x/oauth2"

// Endpoint matches oauth2.Endpoint but with YAML/JSON tags for workspace files.
type Endpoint struct {
	AuthURL  string `yaml:"authURL"  json:"authURL"`
	TokenURL string `yaml:"tokenURL" json:"tokenURL"`
}

// Config represents an OAuth2 client-credentials entry that can be stored in
// the workspace under workspace/oauth/<id>.yaml. The ClientSecret is stored
// encrypted – only its locator is persisted in YAML.
type Config struct {
	Name          string
	oauth2.Config `yaml:",inline"`
}
