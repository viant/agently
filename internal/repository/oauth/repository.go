package oauthrepo

import (
	"context"
	"fmt"
	"github.com/viant/afs"
	"github.com/viant/agently/genai/oauth2"
	baserepo "github.com/viant/agently/internal/repository/base"
	"github.com/viant/agently/internal/workspace"
	"github.com/viant/scy"
	"github.com/viant/scy/cred"
)

// Repository persists OAuth2 configurations in the workspace under "oauth/".
// Client secrets are automatically encrypted/decrypted via scy.
type Repository struct {
	*baserepo.Repository[oauth2.Config]
	secrets *scy.Service
}

// New constructs a Repository that stores YAML files in $WORKSPACE/oauth.
// Secrets are encrypted with the provided scy.Service (blowfish default key
// when nil).
func New(fs afs.Service, scySvc *scy.Service) *Repository {
	if scySvc == nil {
		scySvc = scy.New()
	}
	base := baserepo.New[oauth2.Config](fs, workspace.KindOAuth)
	return &Repository{Repository: base, secrets: scySvc}
}

// Load unmarshals YAML/JSON into secured YAML under the derived filename.
func (r *Repository) Load(ctx context.Context, name string) (*oauth2.Config, error) {
	if r == nil {
		return nil, fmt.Errorf("oauth repo was nil")
	}
	filename := r.Filename(name)
	res := scy.NewResource(&cred.Oauth2Config{}, filename, "blowfish://default")

	secret, err := r.secrets.Load(ctx, res)
	if err != nil {
		return nil, err
	}
	if secret.Target == nil {
		return nil, fmt.Errorf("failed to load secret: %v", err)
	}

	config, ok := secret.Target.(*cred.Oauth2Config)
	if !ok {
		return nil, fmt.Errorf("failed to load secret: %v", err)
	}
	return &oauth2.Config{Name: name, Config: config.Config}, nil
}

// Save marshals cfg to YAML, encrypts its ClientSecret via scy and stores the
// secured YAML under the derived filename.
func (r *Repository) Save(ctx context.Context, name string, cfg *oauth2.Config) error {
	if r == nil || cfg == nil {
		return nil
	}
	filename := r.Filename(name)
	if prev, err := r.Load(ctx, name); err == nil {
		if cfg.ClientSecret == "" {
			cfg.ClientSecret = prev.ClientSecret
		}
	}
	wrapper := &cred.Oauth2Config{Config: cfg.Config}
	res := scy.NewResource(wrapper, filename, "blowfish://default")

	secret := scy.NewSecret(wrapper, res)
	err := r.secrets.Store(ctx, secret)
	if err != nil {
		return err
	}
	return nil
}

// Delete removes YAML and associated secret file (when locator points to file:).
func (r *Repository) Delete(ctx context.Context, name string) error {
	return r.Repository.Delete(ctx, name)
}
