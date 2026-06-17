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
    @Published public var lastAuthSessionID: String?

    private let client: AgentlyClient
    private let oauthSession = OAuthWebAuthenticationSession()
    public static let mobileOAuthRedirectURI = "agently-ios://oauth/callback"

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func beginOAuthLogin() async -> URL? {
        guard !isSubmittingOAuthLogin else { return nil }
        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Requesting OAuth initiation URL")
            let output = try await client.oauthMobileInitiate(
                OAuthInitiateInput(redirectURI: Self.mobileOAuthRedirectURI)
            )
            let rawValue = (output.authURL ?? output.authUrl)?
                .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            guard let url = URL(string: rawValue), !rawValue.isEmpty else {
                logger.error("OAuth initiation returned an invalid URL")
                lastError = "The workspace did not return a valid OAuth sign-in URL."
                return nil
            }
            guard Self.authURLUsesMobileRedirect(url, expectedRedirectURI: Self.mobileOAuthRedirectURI) else {
                logger.error("OAuth initiation returned a web callback instead of the mobile callback")
                lastError = "This workspace is returning a web sign-in callback. Mobile sign-in needs the workspace to allow \(Self.mobileOAuthRedirectURI)."
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

    public func beginOOBLogin(secretsURL: String) async -> Bool {
        let trimmedSecretsURL = secretsURL.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedSecretsURL.isEmpty else {
            lastError = "Add an OOB secret reference before starting out-of-band sign-in."
            return false
        }

        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Starting OOB sign-in flow")
            let output = try await client.oobLogin(OOBLoginInput(secretsURL: trimmedSecretsURL))
            let sessionID = output.sessionID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            lastAuthSessionID = sessionID.isEmpty ? nil : sessionID
            lastError = nil
            await refreshConnectionContext(expectSignedIn: true)
            logger.info("OOB sign-in completed successfully")
            return currentUser != nil
        } catch {
            logger.error("OOB sign-in failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
            return false
        }
    }

    public func beginDeveloperSessionLogin(rawCredential: String) async -> Bool {
        guard !isSubmittingOAuthLogin else { return false }
        let credential = Self.normalizedDeveloperSessionCredential(rawCredential)
        guard !credential.isEmpty else {
            lastError = "Paste a session ID, cookie, or token."
            return false
        }

        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }

        do {
            logger.info("Starting developer session attach")
            let output = try await client.attachAuthSession(AttachSessionInput(sessionID: credential))
            let sessionID = output.sessionID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? credential
            lastAuthSessionID = sessionID.isEmpty ? nil : sessionID
            lastError = nil
            await refreshConnectionContext(expectSignedIn: true)
            guard currentUser != nil else {
                lastError = "The session was attached, but the workspace still requires sign-in."
                return false
            }
            return true
        } catch {
            logger.info("Developer session attach failed; trying token-backed session creation")
        }

        do {
            return try await createDeveloperTokenSession(accessToken: credential, idToken: nil)
        } catch {
            logger.info("Developer access-token session creation failed; trying ID token")
        }

        do {
            return try await createDeveloperTokenSession(accessToken: nil, idToken: credential)
        } catch {
            logger.error("Developer session recovery failed: \(String(describing: error), privacy: .public)")
            lastAuthSessionID = nil
            lastError = "Could not use that session ID or token."
            return false
        }
    }

    public func logoutCurrentSession() async -> Bool {
        guard !isSubmittingOAuthLogin else { return false }
        isSubmittingOAuthLogin = true
        defer { isSubmittingOAuthLogin = false }
        do {
            logger.info("Logging out current session")
            try await client.logout()
            currentUser = nil
            lastAuthSessionID = nil
            await refreshConnectionContext()
            lastError = nil
            return true
        } catch {
            logger.error("Logout failed: \(String(describing: error), privacy: .public)")
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
            let output = try await client.oauthMobileCallback(
                OAuthCallbackInput(code: code, state: state)
            )
            let sessionID = output.sessionID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            lastAuthSessionID = sessionID.isEmpty ? nil : sessionID
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
            } else if !developerAuthFeaturesEnabled() {
                probeMessage = "Workspace is reachable."
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

    public var shouldOfferDeveloperSessionRecovery: Bool {
        currentUser == nil
            && (!(lastError?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
                || !(probeError?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true))
    }

    private func createDeveloperTokenSession(accessToken: String?, idToken: String?) async throws -> Bool {
        let output = try await client.createAuthSession(
            CreateSessionInput(accessToken: accessToken, idToken: idToken)
        )
        let sessionID = output.sessionID.trimmingCharacters(in: .whitespacesAndNewlines)
        lastAuthSessionID = sessionID.isEmpty ? nil : sessionID
        lastError = nil
        await refreshConnectionContext(expectSignedIn: true)
        guard currentUser != nil else {
            lastError = "The token created a session, but the workspace still requires sign-in."
            return false
        }
        return true
    }

    private func isAuthenticationError(_ error: Error) -> Bool {
        if case AgentlySDKError.httpStatus(let statusCode, _) = error {
            return statusCode == 401 || statusCode == 403
        }
        return false
    }

    static func authURLUsesMobileRedirect(_ url: URL, expectedRedirectURI: String) -> Bool {
        let expected = expectedRedirectURI.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !expected.isEmpty,
              let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let redirectURI = components.queryItems?.first(where: { $0.name == "redirect_uri" })?.value?
                .trimmingCharacters(in: .whitespacesAndNewlines) else {
            return false
        }
        return redirectURI == expected
    }

    static func normalizedDeveloperSessionCredential(_ rawValue: String) -> String {
        var value = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        if value.isEmpty {
            return ""
        }

        if let jsonValue = developerSessionCredentialFromJSON(value) {
            return jsonValue
        }

        value = stripSurroundingQuotes(value)
        if value.lowercased().hasPrefix("authorization:") {
            value = String(value.dropFirst("authorization:".count)).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        if value.lowercased().hasPrefix("cookie:") {
            value = String(value.dropFirst("cookie:".count)).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        if value.lowercased().hasPrefix("bearer ") {
            return String(value.dropFirst("bearer ".count)).trimmingCharacters(in: .whitespacesAndNewlines)
        }

        for cookieName in ["agently_session", "sessionId", "sessionID", "session_id"] {
            if let cookieValue = cookieValue(named: cookieName, in: value) {
                return cookieValue
            }
        }
        return stripSurroundingQuotes(value)
    }

    private static func developerSessionCredentialFromJSON(_ value: String) -> String? {
        guard value.first == "{",
              let data = value.data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else {
            return nil
        }
        for key in ["sessionId", "sessionID", "token", "accessToken", "idToken"] {
            if let raw = object[key] as? String {
                let trimmed = stripSurroundingQuotes(raw.trimmingCharacters(in: .whitespacesAndNewlines))
                if !trimmed.isEmpty {
                    return trimmed
                }
            }
        }
        return nil
    }

    private static func cookieValue(named cookieName: String, in value: String) -> String? {
        for part in value.split(separator: ";", omittingEmptySubsequences: true) {
            let pair = part.trimmingCharacters(in: .whitespacesAndNewlines)
            let prefix = "\(cookieName)="
            if pair.lowercased().hasPrefix(prefix.lowercased()) {
                let start = pair.index(pair.startIndex, offsetBy: prefix.count)
                let raw = String(pair[start...]).trimmingCharacters(in: .whitespacesAndNewlines)
                let trimmed = stripSurroundingQuotes(raw)
                return trimmed.isEmpty ? nil : trimmed
            }
        }
        return nil
    }

    private static func stripSurroundingQuotes(_ value: String) -> String {
        var result = value.trimmingCharacters(in: .whitespacesAndNewlines)
        while result.count >= 2,
              let first = result.first,
              let last = result.last,
              (first == "\"" && last == "\"" || first == "'" && last == "'") {
            result = String(result.dropFirst().dropLast()).trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return result
    }
}
