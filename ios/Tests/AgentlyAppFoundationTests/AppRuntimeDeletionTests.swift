import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class AppRuntimeDeletionTests: XCTestCase {
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
    func testDeleteConversationClearsActiveSelectionAndRefreshesList() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let defaults = UserDefaults(suiteName: #function)!
        defaults.removePersistentDomain(forName: #function)
        let store = AppSettingsStore(defaults: defaults)
        let runtime = AppRuntime(
            client: client,
            startupBaseURL: "http://localhost:8585",
            settingsStore: store,
            clientFactory: { _ in client }
        )

        runtime.state.activeConversationID = "conv-1"
        store.saveActiveConversationID("conv-1")

        URLProtocolStub.requestHandler = { request in
            let url = try XCTUnwrap(request.url)
            let response = HTTPURLResponse(
                url: url,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body: String
            switch (request.httpMethod ?? "", url.path) {
            case ("DELETE", "/v1/conversations/conv-1"):
                body = #"{}"#
            case ("GET", "/v1/conversations"):
                body = #"{"Rows":[{"Id":"conv-2","Title":"Remaining"}],"HasMore":false}"#
            default:
                XCTFail("unexpected request \(request.httpMethod ?? "") \(url.path)")
                body = #"{}"#
            }
            return (response, body.data(using: .utf8)!)
        }

        await runtime.deleteConversation(conversationID: "conv-1")

        XCTAssertNil(runtime.state.activeConversationID)
        XCTAssertEqual(runtime.state.conversations.map(\.id), ["conv-2"])
        XCTAssertEqual(store.loadActiveConversationID(), "")
        XCTAssertNil(runtime.state.bootstrapErrorMessage)
        URLProtocolStub.requestHandler = nil
    }
}
