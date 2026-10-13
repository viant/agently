package agently

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	identity "github.com/viant/agently-core/protocol/resource"
	catalogruntime "github.com/viant/agently-core/service/reporting/catalog"
	resources "github.com/viant/agently-core/service/resource"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/forge/backend/reporting/registry"
)

// workspaceReportResourceProviders binds an explicit authored workspace source.
// Other storage sources require an injected provider; no provenance fallback is used.
func workspaceReportResourceProviders(ctx context.Context, raw json.RawMessage, root string, access HostResourceAccess) (ReportResourceProviders, error) {
	var config struct {
		Source         string            `json:"source"`
		ReportingRoot  string            `json:"reportingRoot,omitempty"`
		BuilderWindows map[string]string `json:"builderWindows"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return ReportResourceProviders{}, err
	}
	if decoder.Decode(new(any)) != io.EOF || config.Source != "workspace" || len(config.BuilderWindows) == 0 {
		return ReportResourceProviders{}, fmt.Errorf("explicit workspace report source and builder bindings required")
	}
	options := catalogruntime.ReportResourceOptions{Options: registry.Options{WorkspaceRoot: root, ReportingRoot: config.ReportingRoot}, BuilderWindows: config.BuilderWindows, DataSources: func(ctx context.Context, builder *registry.Asset) (map[string]json.RawMessage, error) {
		key := config.BuilderWindows[builder.ID]
		if key == "" {
			return nil, fmt.Errorf("report builder lacks a trusted window binding")
		}
		return windowloader.LoadWorkspaceDatasourceDescriptorsAt(ctx, root, key, nil)
	}}
	source, err := catalogruntime.NewReportResourceSource(options)
	if err != nil {
		return ReportResourceProviders{}, err
	}
	if access.ProviderIdentity != "" {
		return snapshotWorkspaceReports(ctx, root, options, source, access)
	}
	return ReportResourceProviders{BuilderWindows: config.BuilderWindows, Source: func(_ context.Context, actor identity.VerifiedActor) (identity.ResourceSource, error) {
		return source, nil
	}, Inventories: []catalogruntime.ReportInventory{catalogruntime.AuthoredReportInventory{Source: source}}}, nil
}

func snapshotWorkspaceReports(ctx context.Context, root string, options catalogruntime.ReportResourceOptions, source *catalogruntime.ReportResourceSource, access HostResourceAccess) (ReportResourceProviders, error) {
	if access.Actor == nil || access.Resolver == nil || access.ProviderIdentity == "" {
		return ReportResourceProviders{}, fmt.Errorf("native reports require explicit host authority")
	}
	// Inventory and materialization are content-only startup operations. No
	// principal, profile or grant is requested until a real user operation.
	rows, err := source.List(ctx)
	if err != nil {
		return ReportResourceProviders{}, err
	}
	discovered, err := registry.Discover(ctx, options.Options)
	if err != nil {
		return ReportResourceProviders{}, err
	}
	files := map[string]string{}
	for _, asset := range discovered.Reports {
		uri := asset.ResourceURI
		if uri == "" {
			name, namespace := asset.Name, asset.Namespace
			if name == "" {
				name = asset.ID
			}
			if namespace == "" {
				namespace = options.DefaultNamespace
			}
			uri = "report://" + namespace + "/" + name
		}
		file := asset.SourcePath
		if filepath.IsAbs(file) {
			file, err = filepath.Rel(root, file)
			if err != nil {
				return ReportResourceProviders{}, err
			}
		}
		files[uri] = filepath.ToSlash(file)
	}
	definitions := make([]resources.NativeSnapshotDefinition, 0, len(rows))
	metadata := make([]catalogruntime.ReportCatalogCandidate, 0, len(rows))
	for _, row := range rows {
		if files[row.URI] == "" {
			return ReportResourceProviders{}, fmt.Errorf("report source file missing for %s", row.URI)
		}
		definitions = append(definitions, resources.NativeSnapshotDefinition{URI: row.URI, Title: row.Title, FormatVersion: 1, File: files[row.URI], Load: resources.NativeReportLoader(row.URI, options, nil)})
		metadata = append(metadata, catalogruntime.ReportCatalogCandidate{URI: row.URI, Title: row.Title, Description: row.Description, OwnerID: row.OwnerID, BuilderRef: row.BuilderRef, BuilderWindow: row.BuilderWindow, ReportID: row.ReportID})
	}
	snapshot, err := resources.NewNativeAssetSnapshot(ctx, root, definitions, resources.NativeSnapshotOptions{})
	if err != nil {
		return ReportResourceProviders{}, err
	}
	bindings := make([]resources.LocalResourceBinding, 0, len(rows))
	for _, row := range rows {
		binding := resources.LocalResourceBinding{URI: row.URI, Title: row.Title, Description: row.Description, FormatVersion: 1, Resolver: func(ctx context.Context, _ identity.VerifiedActor, _ string) (*identity.ResourceResolver, error) {
			return access.Resolver(ctx, "report.retrieve", snapshot)
		}}
		bindings = append(bindings, binding)
	}
	return ReportResourceProviders{LocalBindings: bindings, BuilderWindows: options.BuilderWindows, Source: func(context.Context, identity.VerifiedActor) (identity.ResourceSource, error) { return snapshot, nil }, Inventories: []catalogruntime.ReportInventory{catalogruntime.ReportInventoryFunc(func(context.Context, identity.VerifiedActor) ([]catalogruntime.ReportCatalogCandidate, error) {
		return append([]catalogruntime.ReportCatalogCandidate(nil), metadata...), nil
	})}}, nil
}
