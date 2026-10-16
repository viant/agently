package agently

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/viant/authz"
)

// ResourceSelectionProviderFactory binds persistent main/exposure revision
// mappings. It supplies selection only: ACL and requirements remain mandatory.
type ResourceSelectionProviderFactory func(context.Context, json.RawMessage, string, HostResourceAccess) (authz.SelectionStore, error)

func configureHostResourceSelections(options ServeOptions, config *hostAuthorizationFile, root string, access HostResourceAccess) (authz.SelectionStore, error) {
	if len(config.ResourceSelectionProvider) == 0 {
		return authz.NewStaticSelectionStore(config.ResourceSelections)
	}
	if len(config.ResourceSelections) > 0 || options.ResourceSelectionProviderFactory == nil {
		return nil, fmt.Errorf("external resource selections require one explicit provider and no inline selection documents")
	}
	store, err := options.ResourceSelectionProviderFactory(context.Background(), config.ResourceSelectionProvider, root, access)
	if err != nil {
		return nil, fmt.Errorf("resource selection provider: %w", err)
	}
	if store == nil {
		return nil, fmt.Errorf("resource selection provider returned no store")
	}
	return store, nil
}
