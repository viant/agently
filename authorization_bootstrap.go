package agently

import (
	"fmt"
	"os"
	"strings"
)

// configuredWindowBootstrapOptions reads bindings only from the operator's
// authorization file. Browser parameters can select IDs, never providers.
func configuredWindowBootstrapOptions(options ServeOptions) (ServeOptions, error) {
	path := strings.TrimSpace(options.AuthzConfigPath)
	if path == "" {
		path = strings.TrimSpace(os.Getenv("AGENTLY_AUTHZ_CONFIG"))
	}
	if path == "" {
		return options, validateWindowBootstrapBindings(options.WindowBootstrapBindings)
	}
	config, err := readHostAuthorization(path)
	if err != nil {
		return options, err
	}
	if len(config.WindowBootstrapBindings) > 0 {
		if len(options.WindowBootstrapBindings) > 0 {
			return options, fmt.Errorf("window bootstrap bindings configured more than once")
		}
		options.WindowBootstrapBindings = config.WindowBootstrapBindings
	}
	return options, validateWindowBootstrapBindings(options.WindowBootstrapBindings)
}
