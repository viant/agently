import SwiftUI
import AgentlySDK

public struct AuthRequiredScreen: View {
    @ObservedObject private var authRuntime: AuthRuntime
    @ObservedObject private var settingsRuntime: SettingsRuntime
    @State private var didAttemptAutoOOBSignIn = false
    private let baseURL: String
    private let statusMessage: String?
    private let onOpenSettings: () -> Void
    private let onLoginSuccess: () -> Void

    public init(
        authRuntime: AuthRuntime,
        settingsRuntime: SettingsRuntime,
        baseURL: String = "",
        statusMessage: String? = nil,
        onOpenSettings: @escaping () -> Void = {},
        onLoginSuccess: @escaping () -> Void = {}
    ) {
        self.authRuntime = authRuntime
        self.settingsRuntime = settingsRuntime
        self.baseURL = baseURL
        self.statusMessage = statusMessage
        self.onOpenSettings = onOpenSettings
        self.onLoginSuccess = onLoginSuccess
    }

    public var body: some View {
        let developerAuthEnabled = developerAuthFeaturesEnabled()
        VStack(alignment: .leading, spacing: 16) {
            Text("Sign In Required")
                .font(.title2.weight(.semibold))
            if !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                LabeledContent("API Base URL", value: baseURL)
                    .font(.footnote)
            }
            if let statusMessage, !statusMessage.isEmpty {
                Text(statusMessage)
                    .foregroundStyle(.secondary)
                    .font(.footnote)
            }
            if authRuntime.isSubmittingOAuthLogin {
                ProgressView("Starting secure sign-in…")
                    .font(.footnote)
            }
            if authRuntime.isRefreshingContext {
                ProgressView("Checking authentication options…")
                    .font(.footnote)
            }
            if let probeMessage = authRuntime.probeMessage, !probeMessage.isEmpty {
                Text(probeMessage)
                    .foregroundStyle(.secondary)
                    .font(.footnote)
            }
            if let currentUser = authRuntime.currentUser {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Current Session")
                        .font(.footnote.weight(.semibold))
                    Text(currentUser.displayName ?? currentUser.email ?? currentUser.id ?? "Signed in")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            if !authRuntime.authProviders.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Available Providers")
                        .font(.footnote.weight(.semibold))
                    FlexibleProviderList(providers: authRuntime.authProviders)
                }
            }
            if !authRuntime.oauthScopes.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("OAuth Scopes")
                        .font(.footnote.weight(.semibold))
                    Text(authRuntime.oauthScopes.joined(separator: ", "))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            Text("Use the workspace sign-in flow. OAuth client setup is discovered from the connected workspace.")
                .foregroundStyle(.secondary)
                .font(.footnote)
            Button("Continue with Workspace Sign-In") {
                Task {
                    if await authRuntime.beginOAuthWebAuthenticationSessionLogin() {
                        onLoginSuccess()
                    }
                }
            }
            .disabled(authRuntime.isSubmittingOAuthLogin)
            .buttonStyle(.borderedProminent)
            if developerAuthEnabled,
               !settingsRuntime.oobSecretReference.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                Button("Use Saved OOB Sign-In") {
                    Task {
                        if await authRuntime.beginOOBLogin(secretsURL: settingsRuntime.oobSecretReference) {
                            onLoginSuccess()
                        }
                    }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.bordered)
            } else if developerAuthEnabled {
                Text("Add an OOB secret reference in Settings to use out-of-band sign-in.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if authRuntime.currentUser != nil {
                Button("Sign Out") {
                    Task { _ = await authRuntime.logoutCurrentSession() }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.bordered)
            }
            Button("Refresh Auth Options") {
                Task { await authRuntime.refreshConnectionContext() }
            }
            .disabled(
                authRuntime.isRefreshingContext ||
                authRuntime.isSubmittingOAuthLogin
            )
            .buttonStyle(.bordered)
            Button("Edit Connection Settings", action: onOpenSettings)
                .buttonStyle(.bordered)
            if let probeError = authRuntime.probeError, !probeError.isEmpty {
                Text(probeError)
                    .foregroundStyle(.orange)
                    .font(.footnote)
            }
            if let lastError = authRuntime.lastError {
                Text(lastError)
                    .foregroundStyle(.red)
                    .font(.footnote)
            }
        }
        .padding()
        .task {
            guard authRuntime.shouldAutoRefreshAuthContext else { return }
            await authRuntime.refreshConnectionContext()
        }
        .task {
            guard developerAuthFeaturesEnabled() else { return }
            guard resolvedBootstrapAutoOOBSignIn(
                environmentValue: ProcessInfo.processInfo.environment["AGENTLY_IOS_AUTO_OOB_SIGN_IN"],
                launchArguments: CommandLine.arguments
            ) else { return }
            guard !didAttemptAutoOOBSignIn else { return }
            let secret = settingsRuntime.oobSecretReference.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !secret.isEmpty else { return }
            didAttemptAutoOOBSignIn = true
            if await authRuntime.beginOOBLogin(secretsURL: secret) {
                onLoginSuccess()
            }
        }
    }
}

private struct FlexibleProviderList: View {
    let providers: [AuthProvider]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(providers) { provider in
                HStack(spacing: 8) {
                    Text(provider.name ?? provider.type)
                        .font(.footnote.weight(.medium))
                    Text(provider.type)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 6)
                .background(.secondary.opacity(0.12), in: Capsule())
            }
        }
    }
}
