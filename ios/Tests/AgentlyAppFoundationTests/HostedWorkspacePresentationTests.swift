import XCTest
@testable import AgentlyAppFoundation
import AgentlySDK

final class HostedWorkspacePresentationTests: XCTestCase {
    func testResolveHostedWorkspacePresentationUsesTitleWhenAvailable() {
        let window = WorkspaceWindowSnapshot(
            windowId: "line_1",
            windowKey: "line",
            windowTitle: "OLV_BAU_AUS_Media.net PMP"
        )

        let presentation = resolveHostedWorkspacePresentation(window: window)

        XCTAssertEqual(presentation?.badgeLabel, "Line")
        XCTAssertEqual(presentation?.title, "OLV_BAU_AUS_Media.net PMP")
        XCTAssertNil(presentation?.subtitle)
    }

    func testResolveHostedWorkspacePresentationFallsBackToHumanizedWindowKey() {
        let window = WorkspaceWindowSnapshot(
            windowId: "order_1",
            windowKey: "order",
            windowTitle: "order"
        )

        let presentation = resolveHostedWorkspacePresentation(window: window)

        XCTAssertEqual(presentation?.badgeLabel, "Order")
        XCTAssertEqual(presentation?.title, "Order")
        XCTAssertNil(presentation?.subtitle)
    }
}
