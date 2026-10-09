package agently

import (
	"context"
	"fmt"
	"reflect"

	svcauth "github.com/viant/agently-core/service/auth"
	"github.com/viant/agently-core/service/ui/permittedview"
	"github.com/viant/authz"
)

// configuredAccountProjection preserves the explicit account ID vocabulary
// while adding only display fields supplied by a trusted verified projection.
func configuredAccountProjection(mode string, provider permittedview.AccountProjection) (permittedview.AccountProjection, error) {
	base := permittedview.NumericAccountProjection
	switch mode {
	case "", "numeric":
	case "opaque":
		base = permittedview.OpaqueAccountProjection
	default:
		return nil, fmt.Errorf("unsupported authorization accountProjection")
	}
	if provider == nil {
		return base, nil
	}
	return func(ctx context.Context, facts authz.Facts, accountID string) (map[string]any, error) {
		output, err := base(ctx, facts, accountID)
		if err != nil {
			return nil, err
		}
		verified, err := provider(svcauth.AuthzIDTokenContext(ctx), facts, accountID)
		if err != nil {
			return nil, err
		}
		if verified == nil || fmt.Sprint(verified["id"]) != accountID {
			return nil, fmt.Errorf("verified account projection ID mismatch")
		}
		for key, value := range verified {
			if key == "id" {
				continue
			}
			if prior, exists := output[key]; exists && !reflect.DeepEqual(prior, value) {
				return nil, fmt.Errorf("conflicting verified account display field")
			}
			output[key] = value
		}
		return output, nil
	}, nil
}
