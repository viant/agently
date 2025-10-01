package executor

import (
	"context"
	"time"

	"github.com/viant/agently/internal/hotswap"

	agentfinder "github.com/viant/agently/internal/finder/agent"
	embedfinder "github.com/viant/agently/internal/finder/embedder"
	extfinder "github.com/viant/agently/internal/finder/extension"
	modelfinder "github.com/viant/agently/internal/finder/model"

	agentloader "github.com/viant/agently/internal/loader/agent"
	embedloader "github.com/viant/agently/internal/loader/embedder"
	fsloader "github.com/viant/agently/internal/loader/fs"
	modelload "github.com/viant/agently/internal/loader/model"

	embedprovider "github.com/viant/agently/genai/embedder/provider"
	llmprovider "github.com/viant/agently/genai/llm/provider"

	"github.com/viant/afs"
	extrepo "github.com/viant/agently/internal/repository/extension"
	workflowrepo "github.com/viant/agently/internal/repository/workflow"
	"github.com/viant/agently/internal/workspace"

	"github.com/viant/fluxor"

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

	aLoader := agentloader.New(agentloader.WithMetaService(metaSvc))
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

	// Workflow adaptor ---------------------------------------------------
	if rt := e.Runtime(); rt != nil {
		repo := workflowrepo.New(afs.New())

		loadRaw := func(ctx context.Context, name string) ([]byte, error) {
			return repo.GetRaw(ctx, name)
		}

		absFn := func(name string) string { return hotswap.ResolveWorkflowPath(name) }

		refreshFn := func(location string) error { return rt.RefreshWorkflow(location) }

		// UpsertDefinition may not exist in earlier fluxor versions. Use
		// reflection to check.
		var upsertFn func(string, []byte) error
		if upsert, ok := runtimeUpsertFunc(rt); ok {
			upsertFn = upsert
		}

		mgr.Register(workspace.KindWorkflow,
			hotswap.NewWorkflowAdaptor(loadRaw, absFn, refreshFn, upsertFn))
	}

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
// older fluxor versions that do not yet expose the API.
func runtimeUpsertFunc(rt *fluxor.Runtime) (func(string, []byte) error, bool) {
	if rt == nil {
		return nil, false
	}
	v := reflect.ValueOf(rt)
	m := v.MethodByName("UpsertDefinition")
	if !m.IsValid() {
		return nil, false
	}
	// Expect signature func(string, []byte) error
	wrapper := func(location string, data []byte) error {
		res := m.Call([]reflect.Value{reflect.ValueOf(location), reflect.ValueOf(data)})
		if len(res) == 1 && !res[0].IsNil() {
			return res[0].Interface().(error)
		}
		return nil
	}
	return wrapper, true
}
