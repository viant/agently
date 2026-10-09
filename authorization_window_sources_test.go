package agently

import (
	"context"
	"encoding/json"
	"testing"

	windowloader "github.com/viant/agently-core/service/ui/window"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/forge/backend/types"
)

func TestEmbeddedChatIsAnExplicitCanonicalResource(t *testing.T) {
	source, err := newEmbeddedWindowSource([]windowloader.ResourceBinding{{WindowKey: "chat/new", URI: "window://agently/chat-new"}})
	if err != nil {
		t.Fatal(err)
	}
	uri, err := identity.ParseResourceURI("window://agently/chat-new")
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := source.Candidates(context.Background(), uri)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("embedded chat: %v", err)
	}
	raw, err := source.ReadCandidate(context.Background(), uri, candidates[0])
	if err != nil || identity.ContentFingerprint(raw) != candidates[0].ContentFingerprint {
		t.Fatalf("embedded content pin: %v", err)
	}
	var window types.Window
	if err := json.Unmarshal(raw, &window); err != nil || window.View.Content == nil {
		t.Fatalf("missing framework shell: %v", err)
	}
	unknown, _ := identity.ParseResourceURI("window://agently/mcp")
	if _, err := source.Candidates(context.Background(), unknown); err == nil {
		t.Fatal("undeclared framework resource inherited access")
	}
	for _, key := range []string{"../chat/new", "/chat/new", "chat/../../new", "chat/./new"} {
		if _, err := newEmbeddedWindowSource([]windowloader.ResourceBinding{{WindowKey: key, URI: uri.String()}}); err == nil {
			t.Fatalf("invalid embedded path accepted: %s", key)
		}
	}
}
