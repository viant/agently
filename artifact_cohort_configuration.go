package agently

import (
	"fmt"
	"strings"

	"github.com/viant/agently-core/workspace"
	wscfg "github.com/viant/agently-core/workspace/config"
)

func resolveArtifactCohortConfig(root, explicit, environment string, config *wscfg.Root) (string, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		return path, nil
	}
	if path := strings.TrimSpace(environment); path != "" {
		return path, nil
	}
	if config == nil {
		return "", nil
	}
	value, exists := config.Raw["artifactCohortConfig"]
	if !exists {
		return "", nil
	}
	reference, ok := value.(string)
	if !ok || strings.TrimSpace(reference) == "" {
		return "", fmt.Errorf("artifactCohortConfig must name a workspace configuration file")
	}
	return workspace.ResolveChildPath(root, strings.TrimSpace(reference))
}
