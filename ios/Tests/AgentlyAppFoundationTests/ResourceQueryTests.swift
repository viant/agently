import Foundation
import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ResourceQueryTests: XCTestCase {
    final class QueryProtocol: URLProtocol {
        static var captured: URLRequest?
        static var capturedData: Data?
        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            if request.httpMethod == "GET" {
                let response=HTTPURLResponse(url:request.url!,statusCode:200,httpVersion:nil,headerFields:["Content-Type":"application/json"])!
                client?.urlProtocol(self,didReceive:response,cacheStoragePolicy:.notAllowed)
                client?.urlProtocol(self,didLoad:Data(#"{"id":"conv"}"#.utf8));client?.urlProtocolDidFinishLoading(self);return
            }
            var data=request.httpBody ?? Data()
            if data.isEmpty,let stream=request.httpBodyStream {stream.open();defer{stream.close()};var buffer=[UInt8](repeating:0,count:4096);while stream.hasBytesAvailable {let count=stream.read(&buffer,maxLength:buffer.count);if count<=0{break};data.append(contentsOf:buffer.prefix(count))}}
            let input=(try? JSONSerialization.jsonObject(with:data)) as? [String:Any] ?? [:]
            let extensionValue=(input["forwardedProps"] as? [String:Any])?["agently"] as? [String:Any] ?? [:]
            let thread=input["threadId"] as? String ?? "conv",run=input["runId"] as? String ?? ""
            var events:[[String:Any]]=[["type":"RUN_STARTED","threadId":thread,"runId":run,"metadata":["agently":["identityVersion":"1","nativeTurnId":"native-turn"]]]]
            if extensionValue["operation"] as? String=="conversation.bootstrap" {
                events.append(["type":"RUN_FINISHED","threadId":thread,"runId":run,"outcome":["type":"success"],"result":["version":"1","threadId":thread,"transcript":["schemaVersion":"2","conversation":["conversationId":"conv","turns":[]]],"messages":[],"state":[:],"runs":[],"projection":["lossless":true,"unavailableMessageIds":[]]]])
            }else{
                Self.captured=request
                Self.capturedData=data
                events.append(["type":"ACTIVITY_SNAPSHOT","activityType":"agently.turn","messageId":"turn","content":["version":"1","nativeTurnId":"native-turn","status":"running"]])
                events.append(["type":"RUN_FINISHED","threadId":thread,"runId":run,"outcome":["type":"success"]])
            }
            let payload=(try? events.map{"data: "+String(decoding:try JSONSerialization.data(withJSONObject:$0),as:UTF8.self)+"\n\n"}.joined()) ?? ""
            let response=HTTPURLResponse(url:request.url!,statusCode:200,httpVersion:nil,headerFields:["Content-Type":"text/event-stream"])!
            client?.urlProtocol(self,didReceive:response,cacheStoragePolicy:.notAllowed)
            client?.urlProtocol(self,didLoad:Data(payload.utf8));client?.urlProtocolDidFinishLoading(self)
        }
        override func stopLoading() {}
    }
    @MainActor
    func testQueryRuntimePassesResourcesAndAttachmentsThroughAgUi() async throws {
        defer { QueryProtocol.captured = nil;QueryProtocol.capturedData=nil }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [QueryProtocol.self]
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "http://query.test")!)],
                                   session: URLSession(configuration: configuration))
        let runtime = QueryRuntime(client: client)
        let output = await runtime.send(conversationID: "conv", messageID: "stable-client-request", agentID: "agent", query: "Read uploads",
            attachments: [QueryAttachment(name: "old.csv", uri: "/v1/files/old")],
            resourceURIs: ["scratchpad://artifact/new"])
        XCTAssertEqual(output?.conversationID,"conv")
        XCTAssertEqual(output?.messageID,"native-turn")
        let request = try XCTUnwrap(QueryProtocol.captured)
        var data = QueryProtocol.capturedData ?? request.httpBody ?? Data()
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
        let extensionValue=try XCTUnwrap((json["forwardedProps"] as? [String:Any])?["agently"] as? [String:Any])
        XCTAssertEqual(extensionValue["requestId"] as? String,"stable-client-request")
        let payload=try XCTUnwrap(extensionValue["payload"] as? [String:Any])
        XCTAssertEqual(payload["resourceURIs"] as? [String], ["scratchpad://artifact/new"])
        XCTAssertEqual((payload["attachments"] as? [[String: Any]])?.first?["uri"] as? String, "/v1/files/old")
    }
}
