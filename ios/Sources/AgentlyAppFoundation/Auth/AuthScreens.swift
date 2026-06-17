import SwiftUI
import AgentlySDK

public struct AuthRequiredScreen: View {
    @ObservedObject private var authRuntime: AuthRuntime
    @ObservedObject private var settingsRuntime: SettingsRuntime
    @State private var didAttemptAutoOOBSignIn = false
    @State private var didAttemptAutoWorkspaceSignIn = false
    @State private var developerSessionCredential = ""
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
        let oobSecretReference = settingsRuntime.oobSecretReference.trimmingCharacters(in: .whitespacesAndNewlines)
        let hasOOBSecret = !oobSecretReference.isEmpty
        let showDeveloperSessionRecovery = developerAuthEnabled && authRuntime.shouldOfferDeveloperSessionRecovery
        VStack(alignment: .leading, spacing: 16) {
            Text("Sign in to \(workspaceDisplayName)")
                .font(.title2.weight(.semibold))
            if developerAuthEnabled,
               !baseURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                LabeledContent("API Base URL", value: baseURL)
                    .font(.footnote)
            }
            if developerAuthEnabled,
               let statusMessage,
               !statusMessage.isEmpty {
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
            if developerAuthEnabled,
               let probeMessage = authRuntime.probeMessage,
               !probeMessage.isEmpty {
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
            if developerAuthEnabled,
               let sessionID = authRuntime.lastAuthSessionID,
               !sessionID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                LabeledContent("Session ID", value: sessionID)
                    .font(.footnote)
            }
            if developerAuthEnabled, !authRuntime.authProviders.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Available Providers")
                        .font(.footnote.weight(.semibold))
                    FlexibleProviderList(providers: authRuntime.authProviders)
                }
            }
            if developerAuthEnabled, !authRuntime.oauthScopes.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text("OAuth Scopes")
                        .font(.footnote.weight(.semibold))
                    Text(authRuntime.oauthScopes.joined(separator: ", "))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            Text("Continue with your workspace account.")
                .foregroundStyle(.secondary)
                .font(.footnote)
            if hasOOBSecret {
                Button("Sign In") {
                    Task {
                        if await authRuntime.beginOOBLogin(secretsURL: oobSecretReference) {
                            onLoginSuccess()
                        }
                    }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.borderedProminent)
            } else {
                Button("Sign In") {
                    Task {
                        if await authRuntime.beginOAuthWebAuthenticationSessionLogin() {
                            onLoginSuccess()
                        }
                    }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin || authRuntime.isRefreshingContext)
                .buttonStyle(.borderedProminent)
            }
            if authRuntime.currentUser != nil {
                Button("Sign Out") {
                    Task { _ = await authRuntime.logoutCurrentSession() }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.bordered)
            }
            if developerAuthEnabled {
                Button("Refresh Auth Options") {
                    Task { await authRuntime.refreshConnectionContext() }
                }
                .disabled(
                    authRuntime.isRefreshingContext ||
                    authRuntime.isSubmittingOAuthLogin
                )
                .buttonStyle(.bordered)
            }
            Button(developerAuthEnabled ? "Edit Connection Settings" : "Connection Settings", action: onOpenSettings)
                .buttonStyle(.bordered)
            if let probeError = authRuntime.probeError,
               !probeError.isEmpty {
                Text(probeError)
                    .foregroundStyle(.orange)
                    .font(.footnote)
            }
            if let lastError = authRuntime.lastError {
                Text(lastError)
                    .foregroundStyle(.red)
                    .font(.footnote)
            }
            if showDeveloperSessionRecovery {
                VStack(alignment: .leading, spacing: 10) {
                    developerSessionCredentialField
                    Button("Use Session") {
                        Task {
                            if await authRuntime.beginDeveloperSessionLogin(rawCredential: developerSessionCredential) {
                                developerSessionCredential = ""
                                onLoginSuccess()
                            }
                        }
                    }
                    .disabled(
                        authRuntime.isSubmittingOAuthLogin ||
                        developerSessionCredential.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                    )
                    .buttonStyle(.bordered)
                }
            }
        }
        .padding()
        .task {
            guard authRuntime.shouldAutoRefreshAuthContext else { return }
            await authRuntime.refreshConnectionContext()
            await autoStartWorkspaceSignIn(hasOOBSecret: hasOOBSecret)
        }
        .task(id: autoWorkspaceSignInSignature(hasOOBSecret: hasOOBSecret)) {
            await autoStartWorkspaceSignIn(hasOOBSecret: hasOOBSecret)
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

    private var workspaceDisplayName: String {
        settingsRuntime.selectedWorkspacePreset?.title ?? "Workspace"
    }

    @ViewBuilder
    private var developerSessionCredentialField: some View {
        #if os(iOS)
        SecureField("Session ID or token", text: $developerSessionCredential)
            .textFieldStyle(.roundedBorder)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
        #else
        SecureField("Session ID or token", text: $developerSessionCredential)
            .textFieldStyle(.roundedBorder)
            .autocorrectionDisabled()
        #endif
    }

    private func autoWorkspaceSignInSignature(hasOOBSecret: Bool) -> String {
        let providerTypes = authRuntime.authProviders
            .map { $0.type.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }
            .sorted()
            .joined(separator: ",")
        return [
            hasOOBSecret ? "oob" : "oauth",
            providerTypes,
            authRuntime.currentUser == nil ? "signed-out" : "signed-in",
            authRuntime.isRefreshingContext ? "refreshing" : "ready"
        ].joined(separator: "|")
    }

    private var hasWorkspaceOAuthProvider: Bool {
        authRuntime.authProviders.contains { provider in
            let type = provider.type.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            return type == "oauth" || type == "bff" || type == "oidc" || type == "jwt"
        }
    }

    private func autoStartWorkspaceSignIn(hasOOBSecret: Bool) async {
        guard !hasOOBSecret else { return }
        guard !didAttemptAutoWorkspaceSignIn else { return }
        guard hasWorkspaceOAuthProvider else { return }
        guard authRuntime.currentUser == nil else { return }
        guard !authRuntime.isRefreshingContext, !authRuntime.isSubmittingOAuthLogin else { return }
        didAttemptAutoWorkspaceSignIn = true
        if await authRuntime.beginOAuthWebAuthenticationSessionLogin() {
            onLoginSuccess()
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
