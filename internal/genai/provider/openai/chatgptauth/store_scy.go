package chatgptauth

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/viant/scy"
)

type TokenStateStore interface {
	Load(ctx context.Context) (*TokenState, error)
	Save(ctx context.Context, state *TokenState) error
}

type OAuthClientLoader interface {
	Load(ctx context.Context) (*OAuthClientConfig, error)
}

type ScyTokenStateStore struct {
	service   *scy.Service
	tokensURL string
}

func NewScyTokenStateStore(tokensURL string) *ScyTokenStateStore {
	return &ScyTokenStateStore{
		service:   scy.New(),
		tokensURL: tokensURL,
	}
}

func (s *ScyTokenStateStore) Load(ctx context.Context) (*TokenState, error) {
	if s.tokensURL == "" {
		return nil, fmt.Errorf("tokensURL was empty")
	}
	resource := scy.EncodedResource(s.tokensURL).Decode(ctx, reflect.TypeOf(TokenState{}))
	secret, err := s.service.Load(ctx, resource)
	if err != nil {
		if isNotFoundError(err) {
			return nil, &TokenStateNotFoundError{TokensURL: s.tokensURL}
		}
		return nil, err
	}
	state, ok := secret.Target.(*TokenState)
	if !ok {
		return nil, fmt.Errorf("unexpected token state type: %T", secret.Target)
	}
	return state, nil
}

func (s *ScyTokenStateStore) Save(ctx context.Context, state *TokenState) error {
	if state == nil {
		return fmt.Errorf("token state was nil")
	}
	if s.tokensURL == "" {
		return fmt.Errorf("tokensURL was empty")
	}
	resource := scy.EncodedResource(s.tokensURL).Decode(ctx, reflect.TypeOf(TokenState{}))
	secret := scy.NewSecret(state, resource)
	return s.service.Store(ctx, secret)
}

type ScyOAuthClientLoader struct {
	service   *scy.Service
	clientURL string
}

func NewScyOAuthClientLoader(clientURL string) *ScyOAuthClientLoader {
	return &ScyOAuthClientLoader{
		service:   scy.New(),
		clientURL: clientURL,
	}
}

func (l *ScyOAuthClientLoader) Load(ctx context.Context) (*OAuthClientConfig, error) {
	if l.clientURL == "" {
		return nil, fmt.Errorf("clientURL was empty")
	}
	resource := scy.EncodedResource(l.clientURL).Decode(ctx, reflect.TypeOf(OAuthClientConfig{}))
	secret, err := l.service.Load(ctx, resource)
	if err != nil {
		return nil, err
	}
	config, ok := secret.Target.(*OAuthClientConfig)
	if !ok {
		return nil, fmt.Errorf("unexpected oauth client type: %T", secret.Target)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	if os.IsNotExist(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such file or directory") || strings.Contains(msg, "not found")
}
