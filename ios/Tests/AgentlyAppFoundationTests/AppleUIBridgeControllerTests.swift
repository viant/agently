import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class AppleUIBridgeControllerTests: XCTestCase {
    func testHandleSetFormDataMergesGenericWindowFormValues() async throws {
        let runtime = ForgeRuntime()
        let window = await runtime.openWindow(
            key: "generic/report",
            title: "Generic Report",
            parameters: [
                "prefill": .object([
                    "accountId": .number(7)
                ])
            ]
        )

        let result = try await handleAppleUIBridgeCommand(
            method: "ui.window.setFormData",
            params: [
                "windowId": .string(window.id),
                "values": .object([
                    "prefill": .object([
                        "recordId": .number(123)
                    ])
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )
        let windowForm = await runtime.windowFormJSONValue(windowID: window.id)
        let returnedPrefill = result["windowForm"]?.objectValue?["prefill"]?.objectValue

        XCTAssertEqual(result["ok"], .bool(true))
        XCTAssertEqual(result["windowId"], .string(window.id))
        XCTAssertEqual(windowForm["prefill"]?.objectValue?["accountId"], .number(7))
        XCTAssertEqual(windowForm["prefill"]?.objectValue?["recordId"], .number(123))
        XCTAssertEqual(returnedPrefill?["accountId"], .number(7))
        XCTAssertEqual(returnedPrefill?["recordId"], .number(123))
    }

    func testHandleSetFormDataPreservesForecastingPrefillContract() async throws {
        let runtime = ForgeRuntime()
        let openResult = try await handleAppleUIBridgeCommand(
            method: "ui.window.open",
            params: [
                "windowKey": .string("reportBuilder"),
                "windowTitle": .string("Forecasting"),
                "windowId": .string("forecastingCubeBuilder__conv-1"),
                "options": .object([
                    "conversationId": .string("conv-1"),
                    "presentation": .string("hosted"),
                    "region": .string("chat.top")
                ]),
                "parameters": .object([
                    "reportBuilderRef": .string("forecastingCubeBuilder")
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )

        XCTAssertEqual(openResult["ok"], .bool(true))
        XCTAssertEqual(openResult["windowId"], .string("forecastingCubeBuilder__conv-1"))

        let result = try await handleAppleUIBridgeCommand(
            method: "ui.window.setFormData",
            params: [
                "windowId": .string("forecastingCubeBuilder__conv-1"),
                "values": .object([
                    "prefill": .object([
                        "includeCountry": .array([.string("US")]),
                        "includeDealsPmp": .array([.number(90473), .number(90476)]),
                        "includePostalCodeList": .array([.number(70731)]),
                        "scope": .object([
                            "adOrderIds": .array([.number(2664518)]),
                            "audienceIds": .array([.number(7288336)]),
                            "targetKey": .string("audience:7288336")
                        ])
                    ])
                ])
            ],
            forgeRuntime: runtime,
            baseURL: "http://localhost"
        )

        XCTAssertEqual(result["ok"], .bool(true))
        let windowForm = await runtime.windowFormJSONValue(windowID: "forecastingCubeBuilder__conv-1")
        XCTAssertEqual(windowForm["reportBuilderRef"], .string("forecastingCubeBuilder"))
        let prefill = try XCTUnwrap(windowForm["prefill"]?.objectValue)
        XCTAssertEqual(prefill["includeCountry"], .array([.string("US")]))
        XCTAssertEqual(prefill["includeDealsPmp"], .array([.number(90473), .number(90476)]))
        XCTAssertEqual(prefill["includePostalCodeList"], .array([.number(70731)]))
        let scope = try XCTUnwrap(prefill["scope"]?.objectValue)
        XCTAssertEqual(scope["adOrderIds"], .array([.number(2664518)]))
        XCTAssertEqual(scope["audienceIds"], .array([.number(7288336)]))
        XCTAssertEqual(scope["targetKey"], .string("audience:7288336"))

        let returnedPrefill = result["windowForm"]?.objectValue?["prefill"]?.objectValue
        XCTAssertEqual(returnedPrefill?["includeCountry"], .array([.string("US")]))
        XCTAssertEqual(returnedPrefill?["includeDealsPmp"], .array([.number(90473), .number(90476)]))
        XCTAssertEqual(returnedPrefill?["includePostalCodeList"], .array([.number(70731)]))
    }
}

private extension AgentlySDK.JSONValue {
    var objectValue: [String: AgentlySDK.JSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }
}
