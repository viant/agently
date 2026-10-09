package agently

import (
	"context"
	identity "github.com/viant/agently-core/protocol/resource"
	"github.com/viant/agently-core/service/ui/window"
	"github.com/viant/forge/backend/types"
	"testing"
)

func TestLinkedFrameworkLegacyTargetSupportRequiresExactByteEquivalence(t *testing.T) {
	bindings := []window.ResourceBinding{{WindowKey: "chat/new", URI: "window://system/chat"}}
	profiles := []types.WindowTarget{{Platform: "web", FormFactor: "desktop", Surface: "browser"}, {Platform: "ios", FormFactor: "phone", Surface: "app"}, {Platform: "android", FormFactor: "phone", Surface: "app"}}
	source, err := newEmbeddedWindowSource(bindings)
	if err != nil {
		t.Fatal(err)
	}
	uri, _ := identity.ParseResourceURI(bindings[0].URI)
	base, err := source.load(context.Background(), uri)
	if err != nil {
		t.Fatal(err)
	}
	support, err := NewFrameworkLegacyTargetSupport(context.Background(), bindings, profiles)
	if err != nil {
		t.Fatal(err)
	}
	pin := identity.ResolvedResource{URI: uri.String(), ResourceCandidate: identity.ResourceCandidate{Kind: identity.StampedCandidate, Revision: "1", ContentFingerprint: identity.ContentFingerprint(base)}}
	for _, profile := range profiles {
		raw, err := source.loadTarget(context.Background(), uri, &profile)
		if err != nil {
			t.Fatal(err)
		}
		same := identity.ContentFingerprint(raw) == identity.ContentFingerprint(base)
		if support(context.Background(), pin, profile) != same {
			t.Fatal("framework support guessed equivalence")
		}
	}
	changed := pin
	changed.ContentFingerprint = identity.ContentFingerprint([]byte("different historical bytes"))
	if support(context.Background(), changed, profiles[0]) {
		t.Fatal("current artifact substituted older historical content")
	}
	changed = pin
	changed.URI = "window://other/chat"
	if support(context.Background(), changed, profiles[0]) {
		t.Fatal("framework declaration applied to different canonical resource")
	}
}
