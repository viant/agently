import Foundation
import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ResourceQueryTests: XCTestCase {
    final class QueryProtocol: URLProtocol {
        static var captured: URLRequest?
        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            Self.captured = request
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil,
                                           headerFields: ["Content-Type": "application/json"])!
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: Data(#"{"content":"done"}"#.utf8))
            client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }
    @MainActor
    func testQueryRuntimePassesResourcesAndLegacyAttachments() async throws {
        defer { QueryProtocol.captured = nil }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [QueryProtocol.self]
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "http://query.test")!)],
                                   session: URLSession(configuration: configuration))
        let runtime = QueryRuntime(client: client)
        let output = await runtime.send(conversationID: "conv", agentID: "agent", query: "Read uploads",
            attachments: [QueryAttachment(name: "old.csv", uri: "/v1/files/old")],
            resourceURIs: ["scratchpad://artifact/new"])
        XCTAssertEqual(output?.content, "done")
        let request = try XCTUnwrap(QueryProtocol.captured)
        var data = request.httpBody ?? Data()
        if data.isEmpty, let stream = request.httpBodyStream {
            stream.open(); defer { stream.close() }
            var buffer = [UInt8](repeating: 0, count: 4096)
            while stream.hasBytesAvailable {
                let count = stream.read(&buffer, maxLength: buffer.count)
                if count <= 0 { break }
                data.append(contentsOf: buffer.prefix(count))
            }
        }
        let json = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
        XCTAssertEqual(json["resourceURIs"] as? [String], ["scratchpad://artifact/new"])
        XCTAssertEqual((json["attachments"] as? [[String: Any]])?.first?["uri"] as? String, "/v1/files/old")
    }
}
