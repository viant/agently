import Foundation
import AgentlySDK
import OSLog

@MainActor
public final class AuthRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "AuthRuntime")
    public static let oauthCallbackScheme = "agently-ios"

    @Published public var authProviders: [AuthProvider] = []
    @Published public var currentUser: AuthUser?
    @Published public var probeMessage: String?
    @Published public var probeError: String?
    @Published public var oauthScopes: [String] = []
    @Published public var isRefreshingContext = false
    @Published public var isSubmittingOAuthLogin = false
    @Published public var lastError: String?

    private let client: AgentlyClient
    private let oauthSession = OAuthWebAuthenticationSession()

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func beginOAuthLogin() async -> URL? {
        guard !isSubmittingOAuthLogin else { return nil }
        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Requesting OAuth initiation URL")
            let output = try await client.oauthInitiate()
            let rawValue = (output.authURL ?? output.authUrl)?
                .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard let url = URL(string: rawValue), !rawValue.isEmpty else {
                logger.error("OAuth initiation returned an invalid URL")
                lastError = "The workspace did not return a valid OAuth sign-in URL."
                return nil
            }
            lastError = nil
            logger.info("OAuth initiation succeeded")
            return url
        } catch {
            logger.error("OAuth initiation failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
            return nil
        }
    }

    public func beginOAuthWebAuthenticationSessionLogin() async -> Bool {
        guard let authURL = await beginOAuthLogin() else {
            return false
        }

        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Starting ASWebAuthenticationSession sign-in")
            let callbackURL = try await oauthSession.authenticate(
                url: authURL,
                callbackScheme: Self.oauthCallbackScheme
            )
            return await handleOAuthCallback(callbackURL)
        } catch {
            logger.error("ASWebAuthenticationSession sign-in failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
            return false
        }
    }

    public func handleOAuthCallback(_ url: URL) async -> Bool {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme?.lowercased() == Self.oauthCallbackScheme,
              let code = components.queryItems?.first(where: { $0.name == "code" })?.value,
              let state = components.queryItems?.first(where: { $0.name == "state" })?.value,
              !code.isEmpty,
              !state.isEmpty else {
            logger.error("Rejected OAuth callback with missing code or state")
            return false
        }

        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Completing OAuth callback exchange")
            _ = try await client.oauthCallback(
                OAuthCallbackInput(code: code, state: state)
            )
            lastError = nil
            await refreshConnectionContext(expectSignedIn: true)
            logger.info("OAuth callback exchange completed successfully")
            return true
        } catch {
            logger.error("OAuth callback exchange failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
            return false
        }
    }

    public func refreshConnectionContext(expectSignedIn: Bool = false) async {
        guard !isRefreshingContext else { return }
        isRefreshingContext = true
        defer { isRefreshingContext = false }
        do {
            logger.info("Refreshing authentication context")
            let providers = try await client.authProviders()
            authProviders = providers
            probeError = nil
            oauthScopes = []

            if providers.contains(where: { provider in
                let type = provider.type.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
                return type == "oauth" || type == "bff"
            }) {
                do {
                    let oauthConfig = try await client.getOAuthConfig()
                    oauthScopes = oauthConfig.scopes
                } catch {
                    logger.error("OAuth config fetch failed: \(String(describing: error), privacy: .public)")
                    probeError = error.localizedDescription
                }
            }

            do {
                let user = try await client.authMe()
                currentUser = user
                let name = user.displayName ?? user.email ?? user.id ?? "current user"
                probeMessage = "Connected as \(name)."
                logger.info("Authentication context refreshed with signed-in user")
                return
            } catch {
                if isAuthenticationError(error) {
                    logger.info("Authentication context indicates sign-in is still required")
                } else {
                    logger.error("authMe probe failed unexpectedly: \(String(describing: error), privacy: .public)")
                    probeMessage = nil
                    probeError = error.localizedDescription
                    currentUser = nil
                    return
                }
            }

            currentUser = nil
            if expectSignedIn {
                probeMessage = "Workspace is reachable."
            } else if providers.isEmpty {
                probeMessage = "Workspace is reachable, but no auth providers were advertised."
            } else {
                probeMessage = "Workspace is reachable. Available sign-in options: \(providers.map { $0.name ?? $0.type }.joined(separator: ", "))."
            }
        } catch {
            logger.error("Authentication context refresh failed: \(String(describing: error), privacy: .public)")
            authProviders = []
            oauthScopes = []
            currentUser = nil
            probeMessage = nil
            probeError = error.localizedDescription
        }
    }

    public var shouldAutoRefreshAuthContext: Bool {
        authProviders.isEmpty
            && currentUser == nil
            && (probeMessage == nil || probeMessage?.isEmpty == true)
            && (probeError == nil || probeError?.isEmpty == true)
    }

    private func isAuthenticationError(_ error: Error) -> Bool {
        if case AgentlySDKError.httpStatus(let statusCode, _) = error {
            return statusCode == 401 || statusCode == 403
        }
        return false
    }
}
