import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ForgeAgentlyDataSourceLoaderTests: XCTestCase {
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
    func testLookupDatasourceLoaderSendsFlatFilterInputsAndPagingDefaults() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let loader = makeForgeAgentlyDataSourceLoader(client: client)

        URLProtocolStub.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/v1/api/datasources/advertiser_lookup/fetch")
            let body = try XCTUnwrap(request.httpBody)
            let payload = try JSONSerialization.jsonObject(with: body) as? [String: Any]
            let inputs = payload?["inputs"] as? [String: Any]
            XCTAssertEqual(inputs?["AdvertiserId"] as? Double, 13579)
            XCTAssertEqual(inputs?["Page"] as? Double, 1)
            XCTAssertEqual(inputs?["Limit"] as? Double, 20)
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let bodyData = #"{"rows":[{"advertiserId":13579,"advertiserName":"Acme"}],"dataInfo":{"recordCount":1,"pageCount":1},"metrics":{"recordCount":1,"selectedCount":1}}"#.data(using: .utf8)!
            return (response, bodyData)
        }

        let result = try await loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowID: "w1",
                dataSourceRef: "advertiser_lookup",
                dataSource: DataSourceDef(
                    service: DataSourceServiceDef(endpoint: "agentlyAPI", uri: "/v1/api/datasources/advertiser_lookup/fetch", method: "POST"),
                    paging: DataSourcePagingDef(
                        size: 20,
                        enabled: true,
                        parameters: ["page": "Page", "size": "Limit"]
                    )
                ),
                input: InputState(
                    filter: ["AdvertiserId": .number(13579)],
                    parameters: [:],
                    page: nil,
                    fetch: true,
                    refresh: false
                )
            )
        )

        XCTAssertEqual(result?.rows.first?["advertiserId"], .number(13579))
        XCTAssertEqual(result?.metrics["recordCount"], .number(1))
        XCTAssertEqual(result?.metrics["selectedCount"], .number(1))
        URLProtocolStub.requestHandler = nil
    }

    @MainActor
    func testReportDatasourceLoaderPreservesNestedInputQueryShape() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let loader = makeForgeAgentlyDataSourceLoader(client: client)

        URLProtocolStub.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/v1/api/datasources/metrics_ad_cube_report/fetch")
            let body = try XCTUnwrap(request.httpBody)
            let payload = try JSONSerialization.jsonObject(with: body) as? [String: Any]
            let inputs = payload?["inputs"] as? [String: Any]
            let input = inputs?["input"] as? [String: Any]
            let query = input?["query"] as? [String: Any]
            let measures = query?["measures"] as? [String: Any]
            let filters = query?["filters"] as? [String: Any]
            XCTAssertEqual(measures?["totalSpend"] as? Bool, true)
            XCTAssertEqual(filters?["advertiserId"] as? Double, 7)
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let bodyData = #"{"rows":[],"dataInfo":{"recordCount":0,"pageCount":0},"metrics":{"totalSpend":42}}"#.data(using: .utf8)!
            return (response, bodyData)
        }

        _ = try await loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowID: "w1",
                dataSourceRef: "metrics_ad_cube_report",
                dataSource: DataSourceDef(
                    service: DataSourceServiceDef(endpoint: "agentlyAPI", uri: "/v1/api/datasources/metrics_ad_cube_report/fetch", method: "POST")
                ),
                input: InputState(
                    filter: ["filters": .object(["advertiserId": .number(7)])],
                    parameters: [
                        "input": .object([
                            "query": .object([
                                "measures": .object(["totalSpend": .bool(true)])
                            ])
                        ])
                    ],
                    page: nil,
                    fetch: true,
                    refresh: false
                )
            )
        )

        URLProtocolStub.requestHandler = nil
    }

    @MainActor
    func testDatasourceLoaderStartsFromResolvedInputsAndPreservesNestedInputShape() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let loader = makeForgeAgentlyDataSourceLoader(client: client)

        URLProtocolStub.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/v1/api/datasources/order_performance_period_today/fetch")
            let body = try XCTUnwrap(request.httpBody)
            let payload = try JSONSerialization.jsonObject(with: body) as? [String: Any]
            let inputs = payload?["inputs"] as? [String: Any]
            XCTAssertEqual(inputs?["order_id"] as? Double, 2673453)
            XCTAssertEqual(inputs?["period"] as? String, "today")
            XCTAssertEqual(inputs?["granularity"] as? String, "day")
            let input = inputs?["input"] as? [String: Any]
            let query = input?["query"] as? [String: Any]
            XCTAssertEqual(query?["preserve"] as? String, "yes")
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let bodyData = #"{"rows":[],"dataInfo":{"recordCount":0,"pageCount":0},"metrics":{"dailyBudget":180,"lifetimePacingIndex":26}}"#.data(using: .utf8)!
            return (response, bodyData)
        }

        let result = try await loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowID: "w1",
                dataSourceRef: "order_performance_period_today",
                dataSource: DataSourceDef(
                    service: DataSourceServiceDef(endpoint: "agentlyAPI", uri: "/v1/api/datasources/order_performance_period_today/fetch", method: "POST")
                ),
                input: InputState(
                    filter: [:],
                    parameters: [
                        "input": .object([
                            "query": .object([
                                "preserve": .string("yes")
                            ])
                        ])
                    ],
                    page: nil,
                    fetch: true,
                    refresh: false
                ),
                resolvedInputs: [
                    "order_id": .number(2673453),
                    "period": .string("today"),
                    "granularity": .string("day")
                ]
            )
        )

        XCTAssertEqual(result?.metrics["dailyBudget"], .number(180))
        XCTAssertEqual(result?.metrics["lifetimePacingIndex"], .number(26))
        URLProtocolStub.requestHandler = nil
    }

    @MainActor
    func testDatasourceLoaderDoesNotTreatDataInfoAsMetricsWhenMetricsAreMissing() async throws {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [URLProtocolStub.self]
        let session = URLSession(configuration: configuration)
        let endpoint = EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8585")))
        let client = AgentlyClient(endpoints: ["appAPI": endpoint], session: session)
        let loader = makeForgeAgentlyDataSourceLoader(client: client)

        URLProtocolStub.requestHandler = { request in
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let bodyData = #"{"rows":[],"dataInfo":{"recordCount":3,"pageCount":1}}"#.data(using: .utf8)!
            return (response, bodyData)
        }

        let result = try await loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowID: "w1",
                dataSourceRef: "advertiser_lookup",
                dataSource: DataSourceDef(
                    service: DataSourceServiceDef(endpoint: "agentlyAPI", uri: "/v1/api/datasources/advertiser_lookup/fetch", method: "POST")
                ),
                input: InputState(fetch: true)
            )
        )

        XCTAssertTrue(result?.metrics.isEmpty == true)
        URLProtocolStub.requestHandler = nil
    }
}
