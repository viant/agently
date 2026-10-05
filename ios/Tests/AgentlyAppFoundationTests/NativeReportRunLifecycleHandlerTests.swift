import Foundation
import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class NativeReportRunLifecycleHandlerTests: XCTestCase {
    private final class Stub: URLProtocol {
        static var handler: ((URLRequest) throws -> (Int, [String: Any]))?
        override class func canInit(with request: URLRequest) -> Bool { true }
        override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
        override func startLoading() {
            do {
                let (status, value) = try Self.handler!(request)
                let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: nil, headerFields: ["Content-Type": "application/json"])!
                client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
                client?.urlProtocol(self, didLoad: try JSONSerialization.data(withJSONObject: value))
                client?.urlProtocolDidFinishLoading(self)
            } catch { client?.urlProtocol(self, didFailWithError: error) }
        }
        override func stopLoading() {}
    }
    private func body(_ request: URLRequest) throws -> [String: Any] {
        var data = request.httpBody ?? Data()
        if data.isEmpty, let stream = request.httpBodyStream {
            stream.open(); defer { stream.close() }
            var buffer = [UInt8](repeating: 0, count: 4096)
            while stream.hasBytesAvailable { let count = stream.read(&buffer, maxLength: buffer.count); if count <= 0 { break }; data.append(buffer, count: count) }
        }
        return data.isEmpty ? [:] : (try JSONSerialization.jsonObject(with: data) as! [String: Any])
    }
    private func exercise(priorRevision: Int64?, activation: String) async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [Stub.self]
        let session = URLSession(configuration: configuration)
        defer { session.invalidateAndCancel(); Stub.handler = nil }
        let client = AgentlyClient(endpoints: ["appAPI": EndpointConfig(baseURL: URL(string: "https://fixture.invalid")!)], session: session)
        let handler = NativeReportRunLifecycleHandler(client: client)
        let identity = PreparedReportIdentity(windowId: "window", builderRef: "builder", formRevision: .number(1), stateRevision: .number(1))
        let query: [String: ForgeIOSRuntime.JSONValue] = ["filters": .object(["orderIds": .array([.number(7)])]), "limit": .number(50), "offset": .number(0)]
        let packet = PreparedReportRequest(identity: identity, status: "ready", hookStatus: "completed", dataSourceRef: "cube", request: query)
        let admission = NativeReportAdmission(preparation: packet, conversationID: "conversation", stateKey: "state", document: ["blocks": .array([])], datasets: [.init(id: "summary", dataSourceRef: "cube", request: query)], authoredConfiguration: ["request": .object(["autoFetch": .bool(false)])], authorState: ["opaque": .string("retained")])
        let queryObject = try JSONSerialization.jsonObject(with: JSONEncoder().encode(query))
        var run: [String: Any] = ["reportRunId": "run", "ownerId": "owner", "conversationId": "conversation", "status": "running", "revision": 1, "builderRef": "builder", "effectiveParams": queryObject]
        var phases: [String] = []
        func context(_ id: String, _ revision: Int64) -> [String: Any] { ["ownerId": "owner", "conversationId": "conversation", "activeReportRunId": id, "revision": revision] }
        Stub.handler = { request in
            let path = request.url!.path
            let body = try self.body(request)
            if path.hasSuffix("/begin") {
                phases.append("begin")
                XCTAssertEqual(body["origin"] as? String, "prompt")
                let requested = body["requestedParams"] as! [String: Any]
                XCTAssertNotNil(requested[nativeReportAdmissionKey])
                XCTAssertNil((body["effectiveParams"] as? [String: Any])?[nativeReportAdmissionKey])
                var result: [String: Any] = ["run": run]
                if let priorRevision { result["context"] = context("previous-run", priorRevision) }
                return (200, result)
            }
            if path.contains("compile_fenced_report") {
                phases.append("compile")
                let spec = body["invocation"] as! [String: Any]
                let datasets = (spec["datasets"] as! [[String: Any]]).map { item -> [String: Any] in var result = item; result["rows"] = [["spend": 309]]; return result }
                let artifacts: [String: Any] = ["reportSpec": spec, "reportFill": ["source": spec["source"]!, "datasets": datasets], "reportPrint": ["source": spec["source"]!, "pages": []]]
                return (200, ["result": String(decoding: try JSONSerialization.data(withJSONObject: artifacts), as: UTF8.self)])
            }
            if path.hasSuffix("/complete") {
                phases.append("complete")
                XCTAssertEqual((body["expectedRevision"] as? NSNumber)?.intValue, 1)
                run["status"] = "completed"; run["revision"] = 2
                for key in ["reportSpec", "reportFill", "reportPrint"] { run[key] = body[key] }
                return (200, run)
            }
            if path.hasSuffix("/activate") {
                phases.append("activate")
                XCTAssertEqual((body["expectedRunRevision"] as? NSNumber)?.intValue, 2)
                XCTAssertEqual((body["expectedContextRevision"] as? NSNumber)?.int64Value, priorRevision ?? 0)
                if activation == "normal" { return (200, context("run", (priorRevision ?? 0) + 1)) }
                if activation == "conflict" { return (409, ["error": "revision mismatch"]) }
                throw URLError(.networkConnectionLost)
            }
            if path.contains("/context/") {
                phases.append("context")
                if activation == "lost" { return (200, context("run", (priorRevision ?? 0) + 1)) }
                if activation == "conflict" { return (200, context("newer-run", (priorRevision ?? 0) + 1)) }
                return (404, ["error": "not found"])
            }
            XCTFail("Unexpected lifecycle path: \(path)")
            return (500, ["error": "unexpected"])
        }
        let begun = try await handler.begin(admission: admission, uiRunRequestID: "request", origin: "ui.report.run")
        XCTAssertEqual(begun.expectedContextRevision, priorRevision ?? 0)
        XCTAssertEqual(begun.ownerID, "owner")
        let completed = try await handler.complete(handle: begun, rows: ["summary": [["spend": .number(309)]]], current: { true })
        XCTAssertEqual(completed.reportRunID, "run")
        XCTAssertEqual(completed.revision, 2)
        XCTAssertEqual(Array(phases.prefix(4)), ["begin", "compile", "complete", "activate"])
        XCTAssertEqual(phases.filter { $0 == "activate" }.count, 1, "Never retry using a newer context revision")
        if activation == "normal" || activation == "lost" { XCTAssertEqual(completed.contextStatus, "active"); XCTAssertEqual(completed.active, true) }
        else if activation == "conflict" { XCTAssertEqual(completed.contextStatus, "superseded"); XCTAssertEqual(completed.active, false) }
        else { XCTAssertEqual(completed.contextStatus, "unconfirmed"); XCTAssertNil(completed.active) }
    }
    func testBeginWithoutContextCompletesThenActivatesRevisionZero() async throws { try await exercise(priorRevision: nil, activation: "normal") }
    func testBeginWithPreviousContextUsesCapturedRevision() async throws { try await exercise(priorRevision: 7, activation: "normal") }
    func testLostActivationResponseReconcilesSameRun() async throws { try await exercise(priorRevision: nil, activation: "lost") }
    func testConflictPreservesNewerActiveRunAndCompletedArtifact() async throws { try await exercise(priorRevision: 7, activation: "conflict") }
    func testUnavailableContextLeavesCompletionUnconfirmedNotFailed() async throws { try await exercise(priorRevision: nil, activation: "unknown") }
}
