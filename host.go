package agently

import (
	"context"
	hostruntime "github.com/viant/agently-core/app/host/runtime"
	identity "github.com/viant/agently-core/protocol/resource"
	forgeservice "github.com/viant/agently-core/service/primitiveprovider"
	windowloader "github.com/viant/agently-core/service/ui/window"
	"github.com/viant/agently/metadata"
	"github.com/viant/forge/backend/types"
)

// Compatibility aliases retain the application's public embedding API.
type AuthorizationProviders = hostruntime.AuthorizationProviders
type AuthorizationProviderFactory = hostruntime.AuthorizationProviderFactory
type HostAuthoredSource = hostruntime.HostAuthoredSource
type HostToolfeedSource = hostruntime.HostToolfeedSource
type HostNavigationSource = hostruntime.HostNavigationSource
type HostResourceAccess = hostruntime.HostResourceAccess
type ReportResourceProviders = hostruntime.ReportResourceProviders
type ReportResourceProviderFactory = hostruntime.ReportResourceProviderFactory
type ResourceSelectionProviderFactory = hostruntime.ResourceSelectionProviderFactory
type NativeWindowHostConfig = hostruntime.NativeWindowHostConfig
type NativeWindowExecutionBinding = hostruntime.NativeWindowExecutionBinding

func (o ServeOptions) hostOptions() hostruntime.Options {
	return hostruntime.Options{LinkedMetadata: &metadata.FS, NativeWindowRuntimeBinder: o.nativeWindowRuntimeBinder,
		InternalResourceProviderIdentity: o.InternalResourceProviderIdentity, NativeWindowConfigPath: o.NativeWindowConfigPath,
		WindowTargetProof: o.WindowTargetProof, WindowLegacyTargetSupport: o.WindowLegacyTargetSupport,
		ConfigureBuilder: o.ConfigureBuilder, AuthorizationProviderFactory: o.AuthorizationProviderFactory,
		ReportResourceProviderFactory: o.ReportResourceProviderFactory, ResourceSelectionProviderFactory: o.ResourceSelectionProviderFactory,
		AuthzConfigPath: o.AuthzConfigPath, WorkspacePath: o.WorkspacePath}
}
func ValidateHostAuthorizationConfiguration(options ServeOptions) error {
	return hostruntime.ValidateHostAuthorizationConfiguration(options.hostOptions())
}
func NewFrameworkWindowResourceSource(bindings []windowloader.ResourceBinding) (identity.ResourceSource, error) {
	return hostruntime.NewFrameworkWindowResourceSource(&metadata.FS, bindings)
}
func NewFrameworkWindowBundleSource(bindings []windowloader.ResourceBinding) (identity.ResourceSource, error) {
	return hostruntime.NewFrameworkWindowBundleSource(&metadata.FS, bindings)
}
func NewFrameworkLegacyTargetSupport(ctx context.Context, bindings []windowloader.ResourceBinding, targets []types.WindowTarget) (forgeservice.WindowLegacyTargetSupport, error) {
	return hostruntime.NewFrameworkLegacyTargetSupport(ctx, &metadata.FS, bindings, targets)
}
