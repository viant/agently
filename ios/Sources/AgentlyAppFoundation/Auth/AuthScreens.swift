import SwiftUI
import AgentlySDK

public struct AuthRequiredScreen: View {
    @ObservedObject private var authRuntime: AuthRuntime
    @ObservedObject private var settingsRuntime: SettingsRuntime
    private let onOpenSettings: () -> Void
    private let onLoginSuccess: () -> Void

    public init(
        authRuntime: AuthRuntime,
        settingsRuntime: SettingsRuntime,
        onOpenSettings: @escaping () -> Void = {},
        onLoginSuccess: @escaping () -> Void = {}
    ) {
        self.authRuntime = authRuntime
        self.settingsRuntime = settingsRuntime
        self.onOpenSettings = onOpenSettings
        self.onLoginSuccess = onLoginSuccess
    }

    public var body: some View {
        let developerAuthEnabled = developerAuthFeaturesEnabled()
        let oobSecretReference = settingsRuntime.oobSecretReference.trimmingCharacters(in: .whitespacesAndNewlines)
        let hasOOBSecret = developerAuthEnabled && !oobSecretReference.isEmpty
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .firstTextBaseline) {
                Text("This workspace requires authorization.")
                    .font(.title2.weight(.semibold))
                Spacer()
                Button(action: onOpenSettings) {
                    Image(systemName: "gearshape")
                }
                .accessibilityLabel("Workspace settings")
                .buttonStyle(.borderless)
            }
            Button("Sign in") {
                Task {
                    if await authRuntime.beginOAuthWebAuthenticationSessionLogin() {
                        onLoginSuccess()
                    }
                }
            }
            .disabled(authRuntime.isSubmittingOAuthLogin || authRuntime.isRefreshingContext)
            .buttonStyle(.borderedProminent)

            if hasOOBSecret {
                Button("Developer OOB sign-in") {
                    Task {
                        if await authRuntime.beginOOBLogin(secretsURL: oobSecretReference) {
                            onLoginSuccess()
                        }
                    }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.bordered)
            }
        }
        .padding()
        .task {
            guard authRuntime.shouldAutoRefreshAuthContext else { return }
            await authRuntime.refreshConnectionContext()
        }
    }
}
