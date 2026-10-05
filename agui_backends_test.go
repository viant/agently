package agently

import (
	"testing"

	"github.com/stretchr/testify/require"
	wscfg "github.com/viant/agently-core/workspace/config"
)

func TestConfiguredAGUIDemoBackends(t *testing.T) {
	backends, err := configuredAGUIDemoBackends(nil)
	require.NoError(t, err)
	require.Empty(t, backends)
	backends, err = configuredAGUIDemoBackends(&wscfg.Root{Raw: map[string]interface{}{"agUI": map[string]interface{}{"demoBackends": []interface{}{
		map[string]interface{}{"id": "langgraph", "label": "Public LangGraph demo", "url": "https://demo.example/agent/chat", "allowedSubjects": []interface{}{"owner"}},
	}}}})
	require.NoError(t, err)
	require.Len(t, backends, 1)
	require.Equal(t, "https://demo.example/agent/chat", backends[0].URL)
	require.Equal(t, []string{"owner"}, backends[0].AllowedSubjects)
	_, err = configuredAGUIDemoBackends(&wscfg.Root{Raw: map[string]interface{}{"agUI": map[string]interface{}{"demoBackends": []interface{}{
		map[string]interface{}{"id": "bad", "credentialsRef": "not-implemented"},
	}}}})
	require.Error(t, err, "unimplemented authentication must not silently degrade to anonymous")
}
