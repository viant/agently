import Foundation
import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ArtifactAvailabilityTests: XCTestCase {
    final class FixtureProtocol: URLProtocol {
        static var status = 500
        static var body = ""
        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            let response = HTTPURLResponse(url: request.url!, statusCode: Self.status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: Data(Self.body.utf8))
            client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }
    private func client() -> AgentlyClient {
        let config = URLSessionConfiguration.ephemeral; config.protocolClasses = [FixtureProtocol.self]
        return AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "http://fixture.invalid")!)], session: URLSession(configuration: config))
    }
    func testExplicitDisabledListingIsOptional() async throws {
        FixtureProtocol.status = 500; FixtureProtocol.body = #"{"error":"file listing is disabled"}"#
        let result = try await optionalConversationFileList(client: client(), input: ListFilesInput(conversationID: "thread"))
        XCTAssertTrue(result.files.isEmpty)
    }
    func testAuthorizationAndOtherFailuresAreNotSuppressed() async throws {
        for (status, body) in [(403, #"{"error":"file listing is disabled"}"#), (500, #"{"error":"permission denied"}"#), (502, #"{"error":"upstream unavailable"}"#)] {
            FixtureProtocol.status = status; FixtureProtocol.body = body
            do { _ = try await optionalConversationFileList(client: client(), input: ListFilesInput(conversationID: "thread")); XCTFail("Suppressed failure \(status)") }
            catch AgentlySDKError.httpStatus(let actual, _) { XCTAssertEqual(actual, status) }
        }
    }
}
