package a2a

import (
	"github.com/viant/afs"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/agently/internal/workspace/repository/base"
)

// A2AClientConfig matches the structure loaded from $AGENTLY_ROOT/a2a/*.yaml
// and corresponds to the external A2A client configuration format used in
// genai/executor/bootstrap.go
type A2AClientConfig struct {
	ID         string            `yaml:"id" json:"id"`
	Enabled    *bool             `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	JSONRPCURL string            `yaml:"jsonrpcURL" json:"jsonrpcURL"`
	StreamURL  string            `yaml:"streamURL,omitempty" json:"streamURL,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	Directory  struct {
		Name        string   `yaml:"name,omitempty" json:"name,omitempty"`
		Description string   `yaml:"description,omitempty" json:"description,omitempty"`
		Tags        []string `yaml:"tags,omitempty" json:"tags,omitempty"`
		Priority    int      `yaml:"priority,omitempty" json:"priority,omitempty"`
	} `yaml:"directory,omitempty" json:"directory,omitempty"`
}

// Repository manages A2A client configs stored in $AGENTLY_ROOT/a2a.
type Repository struct {
	*baserepo.Repository[A2AClientConfig]
}

func New(fs afs.Service) *Repository {
	return &Repository{baserepo.New[A2AClientConfig](fs, workspace.KindA2A)}
}
