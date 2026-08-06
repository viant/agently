import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class AuthRuntimeTests: XCTestCase {
    final class URLProtocolStub: URLProtocol {
        static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

        override func startLoading() {
            guard let handler = Self.requestHandler else {
                XCTFail("URLProtocolStub.requestHandler was not set")
                return
            }
            do {
                let (response, data) = try handler(request)
                client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
                client?.urlProtocol(self, didLoad: data)
                client?.urlProtocolDidFinishLoading(self)
            } catch {
                client?.urlProtocol(self, didFailWithError: error)
            }
        }

        override func stopLoading() {}
    }

    @MainActor
    func testBeginOOBLoginRefreshesSignedInContext() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let runtime = AuthRuntime(client: client)

        URLProtocolStub.requestHandler = { request in
            let url = try XCTUnwrap(request.url)
            let response = HTTPURLResponse(
                url: url,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body: String
            switch url.path {
            case "/v1/api/auth/oob":
                body = #"{"sessionId":"sess-1","status":"ok","username":"test-user"}"#
            case "/v1/api/auth/providers":
                body = #"[]"#
            case "/v1/api/auth/me":
                body = #"{"id":"user-1","email":"test-user@example.com","displayName":"Test User"}"#
            default:
                XCTFail("unexpected path \(url.path)")
                body = #"{}"#
            }
            return (response, body.data(using: .utf8)!)
        }

        let success = await runtime.beginOOBLogin(secretsURL: "app-oob-reference|blowfish://default")

        XCTAssertTrue(success)
        XCTAssertEqual(runtime.currentUser?.displayName, "Test User")
        XCTAssertNil(runtime.lastError)
        XCTAssertEqual(runtime.probeMessage, "Connected as Test User.")
        URLProtocolStub.requestHandler = nil
    }

    @MainActor
    func testBeginOOBLoginRejectsMissingSecretReference() async {
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "http://localhost:8585")!)])
        let runtime = AuthRuntime(client: client)

        let success = await runtime.beginOOBLogin(secretsURL: "   ")

        XCTAssertFalse(success)
        XCTAssertEqual(runtime.lastError, "Add an OOB secret reference before starting out-of-band sign-in.")
    }

    @MainActor
    func testBeginOOBLoginDoesNotStartWhileAnotherSignInIsActive() async {
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "http://localhost:8585")!)])
        let runtime = AuthRuntime(client: client)
        runtime.isSubmittingOAuthLogin = true

        let success = await runtime.beginOOBLogin(secretsURL: "app-oob-reference|blowfish://default")

        XCTAssertFalse(success)
        XCTAssertTrue(runtime.isSubmittingOAuthLogin)
    }

    @MainActor
    func testBeginOOBLoginDoesNotExposeTransportDetails() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let runtime = AuthRuntime(client: client)

        URLProtocolStub.requestHandler = { _ in
            throw URLError(.timedOut)
        }
        defer { URLProtocolStub.requestHandler = nil }

        let success = await runtime.beginOOBLogin(secretsURL: "app-oob-reference|blowfish://default")

        XCTAssertFalse(success)
        XCTAssertEqual(runtime.lastError, "Saved sign-in could not be completed. Check your connection and try again.")
        XCTAssertFalse(runtime.lastError?.contains("secret") ?? true)
        XCTAssertFalse(runtime.lastError?.contains("http") ?? true)
    }

    @MainActor
    func testBeginOAuthLoginHidesCallbackConfigurationDetails() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let runtime = AuthRuntime(client: client)

        URLProtocolStub.requestHandler = { request in
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"{"authURL":"https://idp.example.test/authorize?redirect_uri=https%3A%2F%2Fworkspace.example.test%2Fcallback"}"#
            return (response, body.data(using: .utf8)!)
        }
        defer { URLProtocolStub.requestHandler = nil }

        let url = await runtime.beginOAuthLogin()

        XCTAssertNil(url)
        XCTAssertEqual(runtime.lastError, "This workspace is not configured for mobile sign-in. Contact the workspace administrator.")
        XCTAssertFalse(runtime.lastError?.contains("agently-ios://") ?? true)
        XCTAssertFalse(runtime.lastError?.contains("workspace.example.test") ?? true)
    }

    @MainActor
    func testHandleOAuthCallbackDoesNotStartWhileAnotherSignInIsActive() async throws {
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))])
        let runtime = AuthRuntime(client: client)
        runtime.isSubmittingOAuthLogin = true

        let success = await runtime.handleOAuthCallback(
            try XCTUnwrap(URL(string: "agently-ios://oauth/callback?code=code&state=state"))
        )

        XCTAssertFalse(success)
        XCTAssertTrue(runtime.isSubmittingOAuthLogin)
    }

    @MainActor
    func testBeginDeveloperSessionLoginAttachesCookieSession() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let runtime = AuthRuntime(client: client)

        var seen: [String] = []
        URLProtocolStub.requestHandler = { request in
            let url = try XCTUnwrap(request.url)
            seen.append("\(request.httpMethod ?? "") \(url.path)")
            let response = HTTPURLResponse(
                url: url,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body: String
            switch url.path {
            case "/v1/api/auth/session/attach":
                body = #"{"status":"ok","sessionId":"sess-1","username":"test-user"}"#
            case "/v1/api/auth/providers":
                body = #"[]"#
            case "/v1/api/auth/me":
                body = #"{"id":"user-1","email":"test-user@example.com","displayName":"Test User"}"#
            default:
                XCTFail("unexpected path \(url.path)")
                body = #"{}"#
            }
            return (response, body.data(using: .utf8)!)
        }

        let success = await runtime.beginDeveloperSessionLogin(
            rawCredential: "Cookie: agently_session=sess-1; Path=/"
        )

        XCTAssertTrue(success)
        XCTAssertEqual(runtime.lastAuthSessionID, "sess-1")
        XCTAssertEqual(runtime.currentUser?.displayName, "Test User")
        XCTAssertNil(runtime.lastError)
        XCTAssertEqual(seen, [
            "POST /v1/api/auth/session/attach",
            "GET /v1/api/auth/providers",
            "GET /v1/api/auth/me"
        ])
        URLProtocolStub.requestHandler = nil
    }

    @MainActor
    func testNormalizedDeveloperSessionCredentialAcceptsCommonPasteFormats() {
        XCTAssertEqual(
            AuthRuntime.normalizedDeveloperSessionCredential("Cookie: agently_session=sess-1; Path=/"),
            "sess-1"
        )
        XCTAssertEqual(
            AuthRuntime.normalizedDeveloperSessionCredential(#"{"sessionId":"sess-json"}"#),
            "sess-json"
        )
        XCTAssertEqual(
            AuthRuntime.normalizedDeveloperSessionCredential("Authorization: Bearer token-1"),
            "token-1"
        )
        XCTAssertEqual(
            AuthRuntime.normalizedDeveloperSessionCredential("'bare-session'"),
            "bare-session"
        )
    }

    @MainActor
    func testSettingsRuntimePersistsOOBSecretReference() {
        let defaults = UserDefaults(suiteName: #function)!
        defaults.removePersistentDomain(forName: #function)
        let store = AppSettingsStore(defaults: defaults)
        let runtime = SettingsRuntime(store: store)

        runtime.apiBaseURL = "http://localhost:9292/v1"
        runtime.preferredAgentID = "agent-1"
        runtime.oobSecretReference = "app-oob-reference|blowfish://default"
        runtime.save()

        XCTAssertEqual(store.loadAPIBaseURL(), "http://localhost:9292")
        XCTAssertEqual(store.loadPreferredAgentID(), "agent-1")
        XCTAssertEqual(store.loadOOBSecretReference(), "app-oob-reference|blowfish://default")
    }

    @MainActor
    func testSettingsRuntimeStoresWorkspaceEndpointSelection() {
        let defaults = UserDefaults(suiteName: #function)!
        defaults.removePersistentDomain(forName: #function)
        let store = AppSettingsStore(defaults: defaults)
        let runtime = SettingsRuntime(store: store)
        let local = SettingsRuntime.workspacePresets[0]

        runtime.selectWorkspaceEndpoint(local)

        XCTAssertTrue(runtime.hasWorkspaceEndpointSelection)
        XCTAssertEqual(runtime.normalizedAPIBaseURL, local.value)
        XCTAssertEqual(store.loadAPIBaseURL(), local.value)

        let restored = SettingsRuntime(store: store)
        XCTAssertTrue(restored.hasWorkspaceEndpointSelection)
        XCTAssertEqual(restored.selectedWorkspacePreset, local)
    }

    func testConfiguredWorkspaceEndpointOptionsDeduplicatesStewardDefault() {
        let options = mergeWorkspaceEndpointOptions(
            parseWorkspaceEndpointOptions(
                """
                [
                  {
                    "title": "Steward",
                    "subtitle": "Viant Steward workspace",
                    "value": "https://steward.agently.viantinc.com/v1/api/"
                  }
                ]
                """
            )
        )

        XCTAssertEqual(
            options.first,
            WorkspaceEndpointOption(
                title: "Steward",
                subtitle: "Viant Steward workspace",
                value: "https://steward.agently.viantinc.com"
            )
        )
        XCTAssertEqual(options.count { $0.value == "https://steward.agently.viantinc.com" }, 1)
        XCTAssertFalse(options.contains { $0.value == "http://localhost:9292" })
        XCTAssertTrue(SettingsRuntime.workspacePresets.contains { $0.value == "https://steward.agently.viantinc.com" })
    }

    @MainActor
    func testWorkspaceEndpointOptionsDefaultToPublicStewardOnly() {
        XCTAssertEqual(SettingsRuntime.workspacePresets.first?.title, "Steward")
        XCTAssertEqual(SettingsRuntime.workspacePresets.first?.value, "https://steward.agently.viantinc.com")
        XCTAssertFalse(SettingsRuntime.workspacePresets.contains { $0.value == "http://localhost:9292" })
    }

    @MainActor
    func testSettingsRuntimeCanSelectConfiguredLocalhostWorkspaceEndpoint() {
        let defaults = UserDefaults(suiteName: #function)!
        defaults.removePersistentDomain(forName: #function)
        let store = AppSettingsStore(defaults: defaults)
        let runtime = SettingsRuntime(store: store)
        let options = mergeWorkspaceEndpointOptions(
            SettingsRuntime.localWorkspacePresets,
            defaults: SettingsRuntime.defaultWorkspacePresets
        )
        let local = options.first { $0.value == "http://localhost:9292" }

        runtime.selectWorkspaceEndpoint(try! XCTUnwrap(local))

        XCTAssertTrue(runtime.hasWorkspaceEndpointSelection)
        XCTAssertEqual(runtime.normalizedAPIBaseURL, "http://localhost:9292")
        XCTAssertEqual(store.loadAPIBaseURL(), "http://localhost:9292")
        XCTAssertNil(runtime.selectedWorkspacePreset)
    }

    @MainActor
    func testAuthURLUsesMobileRedirectRejectsWebCallback() throws {
        let mobileURL = try XCTUnwrap(URL(string: "https://idp.viantinc.com/v1/api/oauth2/authorize?redirect_uri=agently-ios%3A%2F%2Foauth%2Fcallback"))
        let webURL = try XCTUnwrap(URL(string: "https://idp.viantinc.com/v1/api/oauth2/authorize?redirect_uri=https%3A%2F%2Fsteward.agently.viantinc.com%2Fv1%2Fapi%2Fauth%2Foauth%2Fcallback"))

        XCTAssertTrue(AuthRuntime.authURLUsesMobileRedirect(mobileURL, expectedRedirectURI: AuthRuntime.mobileOAuthRedirectURI))
        XCTAssertFalse(AuthRuntime.authURLUsesMobileRedirect(webURL, expectedRedirectURI: AuthRuntime.mobileOAuthRedirectURI))
    }
}
