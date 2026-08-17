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
            VStack(alignment: .leading, spacing: 10) {
                Button {
                    Task {
                        if await authRuntime.beginOAuthWebAuthenticationSessionLogin() {
                            onLoginSuccess()
                        }
                    }
                } label: {
                    HStack(spacing: 8) {
                        if authRuntime.isSubmittingOAuthLogin {
                            ProgressView()
                                .controlSize(.small)
                                .tint(.white)
                        }
                        Text(authRuntime.isSubmittingOAuthLogin ? "Opening sign-in…" : "Sign in")
                    }
                }
                .disabled(authRuntime.isSubmittingOAuthLogin)
                .buttonStyle(.borderedProminent)

                if let error = authRuntime.lastError,
                   !error.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .accessibilityIdentifier("sign-in-error")
                }
            }
        }
        .padding()
        .task {
            guard authRuntime.shouldAutoRefreshAuthContext else { return }
            await authRuntime.refreshConnectionContext()
        }
    }
}
