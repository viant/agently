import XCTest
@testable import AgentlyAppFoundation
import AgentlySDK

final class HostedWorkspacePresentationTests: XCTestCase {
    func testResolveHostedWorkspacePresentationUsesNavigationMetadata() {
        let window = WorkspaceWindowSnapshot(
            windowId: "report_1",
            windowKey: "reportBuilder",
            windowTitle: "Technical title",
            navigation: WorkspaceNavigation(label: "Reports", icon: "chart")
        )

        let presentation = resolveHostedWorkspacePresentation(window: window)

        XCTAssertEqual(presentation?.badgeLabel, "Reports")
        XCTAssertEqual(presentation?.badgeSymbolName, "chart.xyaxis.line")
    }

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

    func testResolveHostedWorkspacePresentationSplitsCamelCaseWindowKeys() {
        let reportBuilder = resolveHostedWorkspacePresentation(
            window: WorkspaceWindowSnapshot(
                windowId: "reportBuilder_1",
                windowKey: "reportBuilder",
                windowTitle: "reportBuilder"
            )
        )
        let forecasting = resolveHostedWorkspacePresentation(
            window: WorkspaceWindowSnapshot(
                windowId: "forecastingCubeBuilder_1",
                windowKey: "forecastingCubeBuilder",
                windowTitle: "forecastingCubeBuilder"
            )
        )

        XCTAssertEqual(reportBuilder?.badgeLabel, "Report Builder")
        XCTAssertEqual(reportBuilder?.title, "Report Builder")
        XCTAssertEqual(forecasting?.badgeLabel, "Forecasting Cube Builder")
        XCTAssertEqual(forecasting?.title, "Forecasting Cube Builder")
    }
}
