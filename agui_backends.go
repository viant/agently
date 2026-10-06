package agently

import (
	"bytes"
	"fmt"

	"github.com/viant/agently-core/sdk"
	wscfg "github.com/viant/agently-core/workspace/config"
	"gopkg.in/yaml.v3"
)

// Demo entries are operator-owned startup configuration, never browser-supplied
// URLs. Their anonymous upstream traffic does not carry the BFF session.
func configuredAGUIDemoBackends(root *wscfg.Root) ([]sdk.AGUIDemoBackend, error) {
	if root == nil || root.Raw["agUI"] == nil {
		return nil, nil
	}
	data, err := yaml.Marshal(root.Raw["agUI"])
	if err != nil {
		return nil, fmt.Errorf("encode AG-UI backend configuration: %w", err)
	}
	var config struct {
		DemoBackends []sdk.AGUIDemoBackend `yaml:"demoBackends"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode AG-UI backend configuration: %w", err)
	}
	return config.DemoBackends, nil
}
