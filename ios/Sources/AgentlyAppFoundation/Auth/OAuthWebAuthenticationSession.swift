import Foundation
#if canImport(AuthenticationServices)
import AuthenticationServices
#endif

@MainActor
final class OAuthWebAuthenticationSession {
    #if canImport(AuthenticationServices)
    private var session: ASWebAuthenticationSession?
    #endif

    func authenticate(url: URL, callbackScheme: String) async throws -> URL {
        #if canImport(AuthenticationServices)
        try await withCheckedThrowingContinuation { continuation in
            let session = ASWebAuthenticationSession(
                url: url,
                callbackURLScheme: callbackScheme
            ) { callbackURL, error in
                self.session = nil
                if let callbackURL {
                    continuation.resume(returning: callbackURL)
                } else if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume(throwing: OAuthWebAuthenticationError.missingCallback)
                }
            }
            session.prefersEphemeralWebBrowserSession = true
            self.session = session
            if !session.start() {
                self.session = nil
                continuation.resume(throwing: OAuthWebAuthenticationError.unableToStart)
            }
        }
        #else
        throw OAuthWebAuthenticationError.unavailable
        #endif
    }
}

enum OAuthWebAuthenticationError: LocalizedError {
    case unavailable
    case unableToStart
    case missingCallback

    var errorDescription: String? {
        switch self {
        case .unavailable:
            return "Secure OAuth sign-in is not available on this platform."
        case .unableToStart:
            return "Could not start the secure OAuth sign-in session."
        case .missingCallback:
            return "OAuth sign-in finished without a callback URL."
        }
    }
}
