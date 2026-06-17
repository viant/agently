import Foundation
#if canImport(AuthenticationServices)
import AuthenticationServices
#endif
#if canImport(UIKit)
import UIKit
#endif
#if canImport(AppKit)
import AppKit
#endif

@MainActor
final class OAuthWebAuthenticationSession: NSObject {
    #if canImport(AuthenticationServices)
    private var session: ASWebAuthenticationSession?
    #endif

    func authenticate(url: URL, callbackScheme: String) async throws -> URL {
        #if canImport(AuthenticationServices)
        try await withCheckedThrowingContinuation { continuation in
            var didResume = false
            func resumeOnce(_ result: Result<URL, Error>) {
                guard !didResume else { return }
                didResume = true
                switch result {
                case .success(let url):
                    continuation.resume(returning: url)
                case .failure(let error):
                    continuation.resume(throwing: error)
                }
            }
            let session = ASWebAuthenticationSession(
                url: url,
                callbackURLScheme: callbackScheme
            ) { callbackURL, error in
                self.session = nil
                if let callbackURL {
                    resumeOnce(.success(callbackURL))
                } else if let error {
                    resumeOnce(.failure(error))
                } else {
                    resumeOnce(.failure(OAuthWebAuthenticationError.missingCallback))
                }
            }
            session.prefersEphemeralWebBrowserSession = true
            session.presentationContextProvider = self
            self.session = session
            if !session.start() {
                self.session = nil
                resumeOnce(.failure(OAuthWebAuthenticationError.unableToStart))
            }
        }
        #else
        throw OAuthWebAuthenticationError.unavailable
        #endif
    }
}

#if canImport(AuthenticationServices)
extension OAuthWebAuthenticationSession: ASWebAuthenticationPresentationContextProviding {
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        #if canImport(UIKit)
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        if let keyWindow = scenes
            .flatMap(\.windows)
            .first(where: { $0.isKeyWindow }) {
            return keyWindow
        }
        if let firstWindow = scenes.flatMap(\.windows).first {
            return firstWindow
        }
        return ASPresentationAnchor()
        #elseif canImport(AppKit)
        return NSApplication.shared.keyWindow ?? ASPresentationAnchor()
        #else
        return ASPresentationAnchor()
        #endif
    }
}
#endif

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
