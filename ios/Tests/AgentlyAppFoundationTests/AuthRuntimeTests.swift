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

        let success = await runtime.beginOOBLogin(secretsURL: "~/.secret/app_oob.enc|blowfish://default")

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
    func testSettingsRuntimePersistsOOBSecretReference() {
        let defaults = UserDefaults(suiteName: #function)!
        defaults.removePersistentDomain(forName: #function)
        let store = AppSettingsStore(defaults: defaults)
        let runtime = SettingsRuntime(store: store)

        runtime.apiBaseURL = "http://localhost:9191/v1"
        runtime.preferredAgentID = "agent-1"
        runtime.oobSecretReference = "~/.secret/app_oob.enc|blowfish://default"
        runtime.save()

        XCTAssertEqual(store.loadAPIBaseURL(), "http://localhost:9191")
        XCTAssertEqual(store.loadPreferredAgentID(), "agent-1")
        XCTAssertEqual(store.loadOOBSecretReference(), "~/.secret/app_oob.enc|blowfish://default")
    }
}
