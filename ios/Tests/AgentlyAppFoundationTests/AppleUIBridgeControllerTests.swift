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
}

private extension AgentlySDK.JSONValue {
    var objectValue: [String: AgentlySDK.JSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }
}
