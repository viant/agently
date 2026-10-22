package agently

import (
	"testing"

	"github.com/viant/agently-core/app/executor"
	"github.com/viant/agently-core/protocol/mcp/manager"
	"github.com/viant/agently-core/protocol/tool"
	bridge "github.com/viant/agently-core/service/primitiveprovider"
	fsstore "github.com/viant/agently-core/workspace/store/fs"
)

func TestRegisterSchedulerUIServices(t *testing.T) {
	mgr, err := manager.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tool.NewDefaultRegistry(mgr)
	if err != nil {
		t.Fatal(err)
	}
	rt := &executor.Runtime{Registry: registry, UIBridge: bridge.NewService(&bridge.Config{}), Store: fsstore.New(t.TempDir())}
	if err := registerSchedulerUIServices(rt); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"ui/view:open", "ui/metadata:list", "ui/metadata:get", "ui/window:list", "ui/control:setValue", "ui/feed:get",
		"ui/datasource:peek", "ui/context:get", "ui/events:list", "ui/report:run",
	} {
		if _, ok := registry.GetDefinition(name); !ok {
			t.Errorf("scheduler tool definition %q is missing", name)
		}
	}
}
