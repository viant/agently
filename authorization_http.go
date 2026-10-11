package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/viant/authz/oauth"
)

// configuredHTTPAuthorizationProviders makes the normal executable usable
// without compiling an application-specific host or importing private IdP code.
func configuredHTTPAuthorizationProviders(ctx context.Context, identityRaw, evaluationRaw json.RawMessage) (AuthorizationProviders, error) {
	var identityConfig oauth.HTTPIdentityConfig
	if err := decodeHTTPAuthorityConfig(identityRaw, &identityConfig); err != nil {
		return AuthorizationProviders{}, fmt.Errorf("identity adapter: %w", err)
	}
	var evaluationConfig *oauth.HTTPEvaluationConfig
	if len(bytes.TrimSpace(evaluationRaw)) != 0 && !bytes.Equal(bytes.TrimSpace(evaluationRaw), []byte("null")) {
		evaluationConfig = &oauth.HTTPEvaluationConfig{}
		if err := decodeHTTPAuthorityConfig(evaluationRaw, evaluationConfig); err != nil {
			return AuthorizationProviders{}, fmt.Errorf("evaluation adapter: %w", err)
		}
	}
	providers, err := oauth.NewHTTPProviders(ctx, identityConfig, evaluationConfig)
	if err != nil {
		return AuthorizationProviders{}, err
	}
	return AuthorizationProviders{
		NativeWindowIdentity: providers.NativeWindowIdentity,
		Entitlements:         providers.Entitlements,
		AccountProjection:    providers.AccountProjection,
		DecisionScope:        providers.DecisionScope,
		MetadataScope:        providers.MetadataScope,
		Identity:             providers.Identity,
		AllowsTenant:         providers.AllowsTenant,
		EntityPermissions:    providers.EntityPermissions,
		PermissionWithLease:  providers.PermissionWithLease,
	}, nil
}

func decodeHTTPAuthorityConfig(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("explicit adapter configuration is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("adapter configuration must contain one JSON object")
	}
	return nil
}
