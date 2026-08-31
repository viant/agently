import XCTest
import Foundation
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ReportRuntimeExportHandlerTests: XCTestCase {
    func testBuildFeedReportExportRequestCompilesCurrentForgeSnapshot() async throws {
        let runtime = ForgeRuntime()
        let metadata = WindowMetadata(
            view: ViewDef(content: ContentDef(containers: [
                ContainerDef(
                    id: "summary",
                    title: "Summary",
                    kind: "dashboard.summary",
                    dataSourceRef: "catalog"
                )
            ])),
            dataSources: ["catalog": DataSourceDef(autoFetch: false)]
        )
        let window = await runtime.openWindowInline(
            key: "feed-catalog-conversation-1",
            title: "Catalog",
            metadata: metadata,
            conversationID: "conversation-1"
        )
        await runtime.setDataSourceForm(
            windowID: window.id,
            dataSourceRef: "catalog",
            values: ["budget": .number(250_000)]
        )

        let request = try await buildFeedReportExportRequest(
            forgeRuntime: runtime,
            windowID: window.id
        )

        XCTAssertEqual(request["format"], .string("pdf"))
        XCTAssertEqual(request["artifactRef"], .string("feed://catalog"))
        let startFence = try XCTUnwrap(request["fences"]?.arrayValue?.first?.objectValue?["payload"]?.objectValue)
        XCTAssertEqual(startFence["grammar"], .string("report-document-v1"))
        XCTAssertEqual(startFence["scope"], .string("feed_catalog"))
        XCTAssertEqual(request["reportSpec"]?.objectValue?["version"], .number(1))
        XCTAssertEqual(request["reportFill"]?.objectValue?["version"], .number(1))
        XCTAssertEqual(request["reportFill"]?.objectValue?["specVersion"], .number(1))
        XCTAssertEqual(request["reportPrint"]?.objectValue?["kind"], .string("reportPrint"))
        XCTAssertEqual(request["reportPrint"]?.objectValue?["version"], .number(1))
        XCTAssertEqual(request["reportPrint"]?.objectValue?["specVersion"], .number(1))
        let datasets = try XCTUnwrap(request["reportFill"]?.objectValue?["datasets"]?.arrayValue)
        XCTAssertEqual(datasets.first?.objectValue?["rows"]?.arrayValue?.first?.objectValue?["budget"], .number(250_000))
    }

    func testReportArtifactIDRecognizesAuthenticatedArtifactLinks() throws {
        XCTAssertEqual(
            reportArtifactID(from: try XCTUnwrap(URL(string: "scratchpad://artifact/3cf51e86-ba08-41be-9bf8-83b27ab9a275"))),
            "3cf51e86-ba08-41be-9bf8-83b27ab9a275"
        )
        XCTAssertEqual(
            reportArtifactID(from: try XCTUnwrap(URL(string: "https://workspace.example.com/__report_artifact__/artifact-2"))),
            "artifact-2"
        )
        XCTAssertNil(reportArtifactID(from: try XCTUnwrap(URL(string: "https://example.com/report.pdf"))))
    }

    func testReportRuntimeExportErrorHidesTransportURL() {
        let error = NSError(
            domain: "test",
            code: 400,
            userInfo: [NSLocalizedDescriptionKey:
                "POST https://example.invalid/v1/tools/reporting failed: 400: " +
                #"{"error":"reporting export: invalid reportSpec: missing version"}"#]
        )

        XCTAssertEqual(
            reportRuntimeExportErrorMessage(error),
            "Unable to create the report PDF: invalid reportSpec: missing version"
        )
    }

    func testReportRuntimeExportErrorExplainsTemporaryStorageFailure() {
        let error = NSError(
            domain: "test",
            code: 500,
            userInfo: [NSLocalizedDescriptionKey:
                #"request failed: 500: {"error":"reporting scratchpad publish: unable to generate access token"}"#]
        )

        XCTAssertEqual(
            reportRuntimeExportErrorMessage(error),
            "The PDF was created, but report storage is temporarily unavailable. Please try again."
        )
    }

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

    func testExportReportRuntimePDFSubmitsPollsAndDownloadsArtifact() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:9191")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let pdfBytes = Data("%PDF-1.7\nreport".utf8)
        let encodedPDF = pdfBytes.base64EncodedString()
        var paths: [String] = []
        var responses = [
            #"{"result":"{\"jobId\":\"job-1\",\"status\":\"queued\"}"}"#,
            #"{"result":"{\"jobId\":\"job-1\",\"status\":\"succeeded\",\"artifactId\":\"artifact-1\",\"artifactRef\":\"report://runtime/performance\"}"}"#,
            #"{"result":"{\"artifactId\":\"artifact-1\",\"name\":\"performance.pdf\",\"contentType\":\"application/pdf\",\"data\":\"\#(encodedPDF)\"}"}"#
        ]
        URLProtocolStub.requestHandler = { request in
            paths.append(request.url?.path(percentEncoded: true) ?? "")
            let body = responses.removeFirst()
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(body.utf8))
        }
        defer { URLProtocolStub.requestHandler = nil }

        let exported = try await exportReportRuntimePDF(
            client: client,
            exportRequest: [
                "artifactRef": .string("report://runtime/performance"),
                "title": .string("Performance"),
                "reportSpec": .object(["kind": .string("reportSpec")]),
                "reportFill": .object(["kind": .string("reportFill")]),
                "reportPrint": .object(["kind": .string("reportPrint")])
            ],
            conversationID: "conversation-1"
        )

        XCTAssertEqual(exported.id, "artifact-1")
        XCTAssertEqual(exported.name, "performance.pdf")
        XCTAssertEqual(exported.contentType, "application/pdf")
        XCTAssertEqual(exported.data, pdfBytes)
        XCTAssertEqual(paths, [
            "/v1/tools/reporting%3Asubmit_export/execute",
            "/v1/tools/reporting%3Aget_export_status/execute",
            "/v1/tools/reporting%3Aget_artifact/execute"
        ])
    }

    func testExportReportRuntimePDFCompilesCanonicalFencesOnGoBackend() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:9191")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let pdfBytes = Data("%PDF-1.7\ncanonical".utf8)
        let encodedPDF = pdfBytes.base64EncodedString()
        var paths: [String] = []
        var responses = [
            #"{"result":"{\"job\":{\"jobId\":\"job-2\",\"status\":\"succeeded\",\"artifactId\":\"artifact-2\"},\"artifact\":{\"artifactId\":\"artifact-2\"}}"}"#,
            #"{"result":"{\"artifactId\":\"artifact-2\",\"name\":\"canonical.pdf\",\"contentType\":\"application/pdf\",\"data\":\"\#(encodedPDF)\"}"}"#
        ]
        URLProtocolStub.requestHandler = { request in
            paths.append(request.url?.path(percentEncoded: true) ?? "")
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(responses.removeFirst().utf8))
        }
        defer { URLProtocolStub.requestHandler = nil }

        let exported = try await exportReportRuntimePDF(
            client: client,
            exportRequest: [
                "title": .string("Canonical"),
                "reportId": .string("canonical-report"),
                "fences": .array([.object([
                    "kind": .string("forge-report"),
                    "index": .number(0),
                    "payload": .object(["version": .number(1), "mode": .string("start")])
                ])])
            ],
            conversationID: "conversation-2"
        )

        XCTAssertEqual(exported.id, "artifact-2")
        XCTAssertEqual(exported.data, pdfBytes)
        XCTAssertEqual(paths, [
            "/v1/tools/reporting%3Acompile_and_export_fenced_report/execute",
            "/v1/tools/reporting%3Aget_artifact/execute"
        ])
    }
}
