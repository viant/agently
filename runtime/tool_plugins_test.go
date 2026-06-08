package runtime

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/app/executor/config"
	"github.com/viant/agently-core/app/store/data"
	"github.com/viant/agently-core/genai/llm"
	convw "github.com/viant/agently-core/pkg/agently/conversation/write"
	resourcesvc "github.com/viant/agently-core/protocol/tool/service/resources"
	goalsvc "github.com/viant/agently-core/protocol/tool/service/system/goal"
	templatesvc "github.com/viant/agently-core/protocol/tool/service/template"
	runtimerequestctx "github.com/viant/agently-core/runtime/requestctx"
	"github.com/viant/agently-core/service/agent"
	"github.com/viant/agently-core/service/augmenter"
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

func TestInternalServiceFactoryResourcesUsesRuntimeAugmenter(t *testing.T) {
	tempDir := t.TempDir()
	runtime := &executor.Runtime{
		Defaults: &config.Defaults{
			Model:    "openai_gpt4o_mini",
			Embedder: "openai_text",
		},
		Store:     fsstore.New(tempDir),
		Augmenter: augmenter.New(nil),
	}

	service := internalServiceFactory(runtime, tempDir, "resources")
	if service == nil {
		t.Fatalf("expected resources service")
	}
	exec, err := service.Method("match")
	if err != nil {
		t.Fatalf("Method(match) error = %v", err)
	}
	out := &resourcesvc.MatchOutput{}
	err = exec(context.Background(), &resourcesvc.MatchInput{Query: "planner"}, out)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "augmenter service is not configured") {
		t.Fatalf("expected runtime resources service to use configured augmenter, got %v", err)
	}
}

func TestInternalServiceFactoryTemplateUsesRuntimeStore(t *testing.T) {
	tempDir := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(tempDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	write("templates/dashboard.yaml", "id: dashboard\nname: dashboard\ndescription: Dashboard template\n")
	write("templates/bundles/steward-output.yaml", "id: steward-output\ntemplates:\n  - dashboard\n")

	runtime := &executor.Runtime{
		Defaults: &config.Defaults{
			Model:    "openai_gpt4o_mini",
			Embedder: "openai_text",
		},
		Store: fsstore.New(tempDir),
	}

	service := internalServiceFactory(runtime, tempDir, "template")
	if service == nil {
		t.Fatalf("expected template service")
	}
	exec, err := service.Method("list")
	if err != nil {
		t.Fatalf("Method(list) error = %v", err)
	}
	out := &templatesvc.ListOutput{}
	if err := exec(context.Background(), &templatesvc.ListInput{}, out); err != nil {
		t.Fatalf("template list error = %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Name != "dashboard" {
		t.Fatalf("unexpected templates: %+v", out.Items)
	}
}

func TestInternalServiceFactorySystemGoalExecutesAgainstConversationScopedStore(t *testing.T) {
	ctx := context.Background()
	dataSvc, err := data.NewThinServiceInMemory(ctx)
	if err != nil {
		t.Fatalf("NewThinServiceInMemory: %v", err)
	}
	if _, err := dataSvc.PatchConversations(ctx, []*convw.Conversation{
		convw.NewMutableConversationView(convw.WithConversationID("conv-goal")),
	}); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}

	runtime := &executor.Runtime{Data: dataSvc}
	service := internalServiceFactory(runtime, t.TempDir(), "system/goal")
	if service == nil {
		t.Fatalf("expected system/goal service")
	}

	toolCtx := runtimerequestctx.WithConversationID(ctx, "conv-goal")

	createExec, err := service.Method("create")
	if err != nil {
		t.Fatalf("system/goal create method: %v", err)
	}
	createOut := &goalsvc.CreateOutput{}
	if err := createExec(toolCtx, &goalsvc.CreateInput{Objective: "finish parser cleanup"}, createOut); err != nil {
		t.Fatalf("system/goal create exec: %v", err)
	}
	if createOut.Goal == nil || createOut.Goal.Objective != "finish parser cleanup" {
		t.Fatalf("unexpected create output: %#v", createOut.Goal)
	}

	getExec, err := service.Method("get")
	if err != nil {
		t.Fatalf("system/goal get method: %v", err)
	}
	getOut := &goalsvc.GetOutput{}
	if err := getExec(toolCtx, &goalsvc.GetInput{}, getOut); err != nil {
		t.Fatalf("system/goal get exec: %v", err)
	}
	if getOut.Goal == nil || getOut.Goal.Objective != "finish parser cleanup" {
		t.Fatalf("unexpected get output: %#v", getOut.Goal)
	}

	updateExec, err := service.Method("update")
	if err != nil {
		t.Fatalf("system/goal update method: %v", err)
	}
	updateOut := &goalsvc.UpdateOutput{}
	if err := updateExec(toolCtx, &goalsvc.UpdateInput{Status: "blocked", Reason: "waiting for review"}, updateOut); err != nil {
		t.Fatalf("system/goal update exec: %v", err)
	}
	if updateOut.Goal == nil || updateOut.Goal.Status != "blocked" || updateOut.Goal.StatusReason == nil || *updateOut.Goal.StatusReason != "waiting for review" {
		t.Fatalf("unexpected update output: %#v", updateOut.Goal)
	}
}

type staticModelFinder struct{}

func (staticModelFinder) Find(_ context.Context, _ string) (llm.Model, error) { return nil, nil }
