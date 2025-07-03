package oauthloader

// Loader for OAuth2 workspace YAML files that store ClientSecret encrypted
// via viant/scy. The file layout mirrors other component loaders – the caller
// passes the absolute file URL (file:///path/to/oauth/github.yaml) or a plain
// filesystem path which is automatically turned into file:// URL.

import (
	"context"
	"fmt"
	"github.com/viant/agently/genai/oauth2"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/viant/scy"
	"github.com/viant/scy/cred"
)

// Service wraps a scy.SecretService so callers can reuse a shared instance.
type Service struct {
	scy *scy.Service
}

// New returns a new loader bound to the supplied scy.Service. When nil it
// initialises a default one (blowfish provider with default key).
func New(svc *scy.Service) *Service {
	if svc == nil {
		svc = scy.New()
	}
	return &Service{scy: svc}
}

// Load reads YAML from URL or local path and returns decrypted *oauth2.Config.
func (l *Service) Load(ctx context.Context, uri string) (*oauth2.Config, error) {

	res := scy.EncodedResource(uri).Decode(ctx, reflect.TypeOf(cred.Oauth2Config{}))

	secret, err := l.scy.Load(ctx, res)
	if err != nil {
		return nil, fmt.Errorf("oauth loader: %w", err)
	}

	wrap, ok := secret.Target.(*cred.Oauth2Config)
	if !ok || wrap == nil {
		return nil, fmt.Errorf("unexpected target type %T", secret.Target)
	}

	cfg := wrap.Config
	if cfg.ClientID == "" {
		return nil, fmt.Errorf("clientID missing in %s", uri)
	}
	return &oauth2.Config{Config: cfg, ID: deriveIDFromPath(uri)}, nil
}

func deriveIDFromPath(uri string) string {
	base := filepath.Base(uri)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
