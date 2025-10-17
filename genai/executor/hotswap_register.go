package executor

import (
	"context"
	"time"

	hotswap "github.com/viant/agently/internal/workspace/hotswap"
	"github.com/viant/agently/internal/workspace/loader/agent"
	embedloader "github.com/viant/agently/internal/workspace/loader/embedder"
	fsloader "github.com/viant/agently/internal/workspace/loader/fs"
	modelload "github.com/viant/agently/internal/workspace/loader/model"
	extrepo "github.com/viant/agently/internal/workspace/repository/extension"

	agentfinder "github.com/viant/agently/internal/finder/agent"
	embedfinder "github.com/viant/agently/internal/finder/embedder"
	extfinder "github.com/viant/agently/internal/finder/extension"
	modelfinder "github.com/viant/agently/internal/finder/model"

	"github.com/viant/afs"
	embedprovider "github.com/viant/agently/genai/embedder/provider"
	llmprovider "github.com/viant/agently/genai/llm/provider"
	// workflowrepo "github.com/viant/agently/internal/repository/workflow"
	"github.com/viant/agently/internal/workspace"

	"reflect"
	"unsafe"
)

// initialiseHotSwap sets up live reload when it is enabled. The method is
// invoked by Service.New after init(ctx) has succeeded.
func (e *Service) initialiseHotSwap() {
	mgr, err := hotswap.NewManager(workspace.Root(), 200*time.Millisecond)
	if err != nil {
		return // give up silently – feature is best-effort
	}

	// Resolve finders via reflection -----------------------------------
	aFinder := privateField[*agentfinder.Finder](e, "agentFinder")
	mFinder := privateField[*modelfinder.Finder](e, "modelFinder")
	emFinder := privateField[*embedfinder.Finder](e, "embedderFinder")

	// Build loaders with same meta service -----------------------------
	metaSvc := e.config.Meta()

	aLoader := agent.New(agent.WithMetaService(metaSvc))
	mLoader := modelload.New(fsloader.WithMetaService[llmprovider.Config](metaSvc))
	emLoader := embedloader.New(fsloader.WithMetaService[embedprovider.Config](metaSvc))

	if aFinder != nil {
		mgr.Register(workspace.KindAgent, hotswap.NewAgentAdaptor(aLoader, aFinder))
	}
	if mFinder != nil {
		mgr.Register(workspace.KindModel, hotswap.NewModelAdaptor(mLoader, mFinder))
	}
	if emFinder != nil {
		mgr.Register(workspace.KindEmbedder, hotswap.NewEmbedderAdaptor(emLoader, emFinder))
	}

	if e.config != nil && e.config.MCP != nil {
		mgr.Register(workspace.KindMCP, hotswap.NewMCPAdaptor(e.config.MCP))
	}

	// Extensions (Tool Metadata) adaptor -------------------------------
	{
		repo := extrepo.New(afs.New())
		finder := extfinder.New()
		// Preload existing definitions
		if names, err := repo.List(context.Background()); err == nil {
			for _, n := range names {
				if rec, err := repo.Load(context.Background(), n); err == nil && rec != nil {
					finder.Add(n, rec)
				}
			}
		}
		// Expose on executor for lookups
		e.extFinder = finder
		mgr.Register(workspace.KindFeeds, hotswap.NewExtensionAdaptor(repo, finder))
	}

	// Workflow adaptor disabled in decoupled mode

	_ = mgr.Start()
}

// privateField returns pointer to private struct field when type matches T.
func privateField[T any](svc *Service, name string) T {
	var zero T
	v := reflect.ValueOf(svc).Elem()
	f := v.FieldByName(name)
	if !f.IsValid() {
		return zero
	}
	ptr := unsafe.Pointer(f.UnsafeAddr())
	rv := reflect.NewAt(f.Type(), ptr)
	val, ok := rv.Elem().Interface().(T)
	if !ok {
		return zero
	}
	return val
}

// runtimeUpsertFunc returns a strongly typed closure bound to rt.UpsertDefinition
// when that method exists. It uses reflection so Agently can build against
// older runtime variants without the updated API.
func runtimeUpsertFunc(_ interface{}) (func(string, []byte) error, bool) { return nil, false }
