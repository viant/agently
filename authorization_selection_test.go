package agently

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/viant/authz"
)

func TestHostResourceSelectionProviderDoesNotFallBackOrSnapshot(t *testing.T) {
	family := authz.ResourceFamily{Kind: "report", ID: "report://analytics/sales", Tenant: "tenant"}
	configured := &hostAuthorizationFile{ResourceSelectionProvider: json.RawMessage(`{"store":"resource-selection"}`)}
	if _, err := configureHostResourceSelections(ServeOptions{}, configured, "workspace", HostResourceAccess{}); err == nil {
		t.Fatal("missing mapping store accepted")
	}
	store := &hostSelectionStore{document: authz.SelectionDocument{Resource: family, Revision: 1, DefaultVersion: "1"}}
	factory := func(_ context.Context, raw json.RawMessage, root string, access HostResourceAccess) (authz.SelectionStore, error) {
		if string(raw) != string(configured.ResourceSelectionProvider) || root != "workspace" {
			t.Fatal("wrong operator mapping source")
		}
		return store, nil
	}
	selected, err := configureHostResourceSelections(ServeOptions{ResourceSelectionProviderFactory: factory}, configured, "workspace", HostResourceAccess{})
	if err != nil {
		t.Fatal(err)
	}
	store.document.Revision = 2
	store.document.DefaultVersion = "2"
	current, err := selected.GetSelection(context.Background(), family)
	if err != nil || current.Revision != 2 || current.DefaultVersion != "2" {
		t.Fatal("host froze persistent revision mapping at startup")
	}
	configured.ResourceSelections = []authz.SelectionDocument{{Resource: family, Revision: 1, DefaultVersion: "working"}}
	if _, err := configureHostResourceSelections(ServeOptions{ResourceSelectionProviderFactory: factory}, configured, "workspace", HostResourceAccess{}); err == nil {
		t.Fatal("ambiguous database/file authority accepted")
	}
	configured.ResourceSelections = nil
	unavailable := errors.New("mapping store unavailable")
	_, err = configureHostResourceSelections(ServeOptions{ResourceSelectionProviderFactory: func(context.Context, json.RawMessage, string, HostResourceAccess) (authz.SelectionStore, error) {
		return nil, unavailable
	}}, configured, "workspace", HostResourceAccess{})
	if !errors.Is(err, unavailable) {
		t.Fatal("mapping outage replaced by inline defaults")
	}
}

type hostSelectionStore struct{ document authz.SelectionDocument }

func (s *hostSelectionStore) GetSelection(context.Context, authz.ResourceFamily) (authz.SelectionDocument, error) {
	return s.document, nil
}
