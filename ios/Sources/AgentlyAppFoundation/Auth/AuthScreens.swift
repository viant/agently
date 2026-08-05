import SwiftUI
import AgentlySDK

public struct AuthRequiredScreen: View {
    @ObservedObject private var authRuntime: AuthRuntime
    private let onOpenSettings: () -> Void
    private let onLoginSuccess: () -> Void

    public init(
        authRuntime: AuthRuntime,
        onOpenSettings: @escaping () -> Void = {},
        onLoginSuccess: @escaping () -> Void = {}
    ) {
        self.authRuntime = authRuntime
        self.onOpenSettings = onOpenSettings
        self.onLoginSuccess = onLoginSuccess
    }

    public var body: some View {
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
        }
        .padding()
        .task {
            guard authRuntime.shouldAutoRefreshAuthContext else { return }
            await authRuntime.refreshConnectionContext()
        }
    }
}
