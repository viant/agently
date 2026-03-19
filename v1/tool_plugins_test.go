package v1

import (
	"testing"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
)

func TestInternalServiceFactoryAppOwnedServices(t *testing.T) {
	runtime := &executor.Runtime{
		Defaults: &config.Defaults{
			Model:    "openai_gpt4o_mini",
			Embedder: "openai_text",
		},
	}

	testCases := []struct {
		name        string
		serviceName string
		expect      string
	}{
		{name: "resources", serviceName: "resources", expect: "resources"},
		{name: "internal message", serviceName: "internal/message", expect: "internal/message"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := internalServiceFactory(runtime, "/tmp/workspace", tc.serviceName)
			if service == nil {
				t.Fatalf("expected service for %s", tc.serviceName)
			}
			if service.Name() != tc.expect {
				t.Fatalf("unexpected service name: got %s want %s", service.Name(), tc.expect)
			}
		})
	}
}
