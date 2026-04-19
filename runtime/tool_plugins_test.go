package runtime

import (
	"context"
	"reflect"
	"testing"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
	"github.com/viant/agently-core/genai/llm"
	"github.com/viant/agently-core/service/agent"
	core2 "github.com/viant/agently-core/service/core"
	fsstore "github.com/viant/agently-core/workspace/store/fs"
)

func TestInternalServiceFactoryAppOwnedServices(t *testing.T) {
	tempDir := t.TempDir()
	runtime := &executor.Runtime{
		Defaults: &config.Defaults{
			Model:    "openai_gpt4o_mini",
			Embedder: "openai_text",
		},
		Store: fsstore.New(tempDir),
	}

	testCases := []struct {
		name        string
		serviceName string
		expect      string
	}{
		{name: "resources", serviceName: "resources", expect: "resources"},
		{name: "message alias", serviceName: "message", expect: "message"},
		{name: "legacy internal message alias", serviceName: "internal/message", expect: "message"},
		{name: "system platform", serviceName: "system/platform", expect: "system/platform"},
		{name: "template", serviceName: "template", expect: "template"},
		{name: "prompt", serviceName: "prompt", expect: "prompt"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			service := internalServiceFactory(runtime, tempDir, tc.serviceName)
			if service == nil {
				t.Fatalf("expected service for %s", tc.serviceName)
			}
			if service.Name() != tc.expect {
				t.Fatalf("unexpected service name: got %s want %s", service.Name(), tc.expect)
			}
		})
	}
}

func TestInternalServiceFactoryLLMAgentsCarriesPromptProfileWiring(t *testing.T) {
	tempDir := t.TempDir()
	runtime := &executor.Runtime{
		Defaults: &config.Defaults{
			Model:    "openai_gpt4o_mini",
			Embedder: "openai_text",
		},
		Store: fsstore.New(tempDir),
		Agent: &agent.Service{},
		Core:  core2.New(staticModelFinder{}, nil, nil),
	}

	service := internalServiceFactory(runtime, tempDir, "llm/agents")
	if service == nil {
		t.Fatalf("expected llm/agents service")
	}
	if service.Name() != "llm/agents" {
		t.Fatalf("unexpected service name: got %s", service.Name())
	}

	value := reflect.ValueOf(service)
	if value.Kind() != reflect.Pointer {
		t.Fatalf("expected pointer service, got %s", value.Kind())
	}
	elem := value.Elem()

	promptRepo := elem.FieldByName("promptRepo")
	if !promptRepo.IsValid() || promptRepo.IsNil() {
		t.Fatalf("expected promptRepo to be wired")
	}
	modelFinder := elem.FieldByName("modelFinder")
	if !modelFinder.IsValid() || modelFinder.IsNil() {
		t.Fatalf("expected modelFinder to be wired")
	}
}

type staticModelFinder struct{}

func (staticModelFinder) Find(_ context.Context, _ string) (llm.Model, error) { return nil, nil }
