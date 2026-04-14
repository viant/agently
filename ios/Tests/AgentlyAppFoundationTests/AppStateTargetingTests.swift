import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class AppStateTargetingTests: XCTestCase {
    @MainActor
    func testAppStateSeedsSharedMetadataTargetContext() throws {
        let client = AgentlyClient(
            endpoints: ["appAPI": EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8181")))]
        )

        let state = AppState(client: client, bootstrapBaseURL: "http://localhost:8181")

        XCTAssertEqual(state.metadataTargetContext.platform, "ios")
        XCTAssertEqual(state.metadataTargetContext.surface, "app")
        XCTAssertTrue(state.metadataTargetContext.capabilities.contains("markdown"))
        XCTAssertTrue(state.metadataTargetContext.capabilities.contains("chart"))
    }

    @MainActor
    func testAppStateForgeRuntimeUsesExplicitIOSPlatformTargeting() throws {
        let client = AgentlyClient(
            endpoints: ["appAPI": EndpointConfig(baseURL: try XCTUnwrap(URL(string: "http://localhost:8181")))]
        )

        let state = AppState(client: client, bootstrapBaseURL: "http://localhost:8181")

        let runtime = state.forgeRuntime
        let mirror = Mirror(reflecting: runtime)
        let target = mirror.children.first { $0.label == "targetContext" }?.value as? ForgeTargetContext

        XCTAssertEqual(target?.platform, "ios")
        XCTAssertTrue((target?.formFactor ?? "").isEmpty == false)
        XCTAssertTrue(target?.capabilities.contains("markdown") == true)
    }

    @MainActor
    func testAppleTargetHelpersStayAligned() throws {
        XCTAssertEqual(buildAppleTargetCapabilities(), ["markdown", "chart", "attachments", "camera", "voice"])
        XCTAssertTrue(["phone", "tablet"].contains(detectAppleFormFactor()))
    }
}
