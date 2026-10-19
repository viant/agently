package agently

import (
	"fmt"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/protocol/tool"
	toolservice "github.com/viant/agently-core/protocol/tool/service"
	uicontext "github.com/viant/agently-core/protocol/tool/service/ui/context"
	uicontrol "github.com/viant/agently-core/protocol/tool/service/ui/control"
	uidatasource "github.com/viant/agently-core/protocol/tool/service/ui/datasource"
	uievents "github.com/viant/agently-core/protocol/tool/service/ui/events"
	uifeed "github.com/viant/agently-core/protocol/tool/service/ui/feed"
	uireport "github.com/viant/agently-core/protocol/tool/service/ui/report"
	uiview "github.com/viant/agently-core/protocol/tool/service/ui/view"
	uiwindow "github.com/viant/agently-core/protocol/tool/service/ui/window"
	forgewindowrepo "github.com/viant/agently-core/workspace/repository/forgewindow"
)

// Scheduled agents use the same workspace profile as interactive agents. Keep
// UI tool definitions available even though no browser is attached to a run.
func registerSchedulerUIServices(rt *executor.Runtime) error {
	if rt == nil || rt.Registry == nil || rt.UIBridge == nil {
		return fmt.Errorf("scheduler runtime is missing UI registry or bridge")
	}
	services := []toolservice.Service{
		uiview.New(forgewindowrepo.NewWithStore(rt.Store), rt.UIBridge),
		uiwindow.New(rt.UIBridge),
		uicontrol.New(rt.UIBridge),
		uifeed.New(rt.UIBridge),
		uidatasource.New(rt.UIBridge),
		uicontext.New(rt.UIBridge),
		uievents.New(rt.UIBridge),
		uireport.New(rt.UIBridge),
	}
	for _, service := range services {
		if err := tool.AddInternalService(rt.Registry, service); err != nil {
			return fmt.Errorf("register %T: %w", service, err)
		}
	}
	return nil
}
