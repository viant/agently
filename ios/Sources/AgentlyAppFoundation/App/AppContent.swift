import SwiftUI

public struct AppContent: View {
    @ObservedObject private var runtime: AppRuntime
    @State private var isShowingSettings = false

    public init(runtime: AppRuntime) {
        self.runtime = runtime
    }

    public var body: some View {
        NavigationStack {
            switch runtime.state.authState {
            case .checking:
                LaunchingScreen(
                    baseURL: runtime.state.bootstrapBaseURL,
                    onRetry: {
                        Task { await runtime.bootstrap() }
                    },
                    onOpenSettings: {
                        isShowingSettings = true
                    }
                )
            case .required:
                AuthRequiredScreen(
                    authRuntime: runtime.authRuntime,
                    baseURL: runtime.state.bootstrapBaseURL,
                    statusMessage: runtime.state.bootstrapErrorMessage,
                    onOpenSettings: {
                        isShowingSettings = true
                    },
                    onLoginSuccess: {
                        Task { await runtime.bootstrap() }
                    }
                )
            case .connectionFailed:
                ConnectionFailureScreen(
                    baseURL: runtime.state.bootstrapBaseURL,
                    errorMessage: runtime.state.bootstrapErrorMessage,
                    onRetry: {
                        Task { await runtime.bootstrap() }
                    },
                    onOpenSettings: {
                        isShowingSettings = true
                    }
                )
            case .signedIn:
                AppShellView(runtime: runtime)
            }
        }
        .sheet(isPresented: $isShowingSettings) {
            NavigationStack {
                SettingsScreen(
                    runtime: runtime.settingsRuntime,
                    workspaceRoot: runtime.state.workspaceMetadata?.workspaceRoot,
                    workspaceDefaultAgentID: runtime.state.workspaceMetadata?.defaultAgent,
                    availableAgents: runtime.availableAgentOptions,
                    agentAutoSelectionEnabled: runtime.state.workspaceMetadata?.capabilities?.agentAutoSelection == true,
                    oauthProviderLabels: runtime.authRuntime.authProviders.map { ($0.name ?? $0.type).trimmingCharacters(in: .whitespacesAndNewlines) }.filter { !$0.isEmpty },
                    oauthScopes: runtime.authRuntime.oauthScopes
                ) {
                    Task {
                        isShowingSettings = false
                        await runtime.applySettingsAndReload()
                    }
                }
            }
        }
    }
}

private struct LaunchingScreen: View {
    let baseURL: String
    let onRetry: () -> Void
    let onOpenSettings: () -> Void
    @State private var didTimeout = false

    var body: some View {
        VStack(spacing: 16) {
            ProgressView()
                .controlSize(.large)
            Text("Connecting to workspace")
                .font(.title3.weight(.semibold))
            if !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Text(baseURL)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if didTimeout {
                Text("Still waiting for the workspace service.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                HStack(spacing: 12) {
                    Button("Retry", action: onRetry)
                        .buttonStyle(.borderedProminent)
                    Button("Edit Settings", action: onOpenSettings)
                        .buttonStyle(.bordered)
                }
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding()
        .task {
            guard !didTimeout else { return }
            do {
                try await Task.sleep(for: .seconds(10))
                didTimeout = true
            } catch {
                // Screen task was cancelled because navigation state changed.
            }
        }
    }
}

private struct ConnectionFailureScreen: View {
    let baseURL: String
    let errorMessage: String?
    let onRetry: () -> Void
    let onOpenSettings: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Text("Connection Problem")
                .font(.title2.weight(.semibold))
            if !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                LabeledContent("API Base URL", value: baseURL)
                    .font(.footnote)
            }
            if let errorMessage, !errorMessage.isEmpty {
                Text(errorMessage)
                    .foregroundStyle(.secondary)
            }
            HStack(spacing: 12) {
                Button("Retry", action: onRetry)
                    .buttonStyle(.borderedProminent)
                Button("Edit Settings", action: onOpenSettings)
                    .buttonStyle(.bordered)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .center)
        .padding()
        .navigationTitle("Agently")
    }
}
