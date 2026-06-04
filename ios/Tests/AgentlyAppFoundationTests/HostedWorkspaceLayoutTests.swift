import XCTest
import CoreGraphics
@testable import AgentlyAppFoundation
import AgentlySDK

final class HostedWorkspaceLayoutTests: XCTestCase {
    func testResolveDefaultHostedWorkspaceDisplayModeUsesExpandedForRegularWidth() {
        XCTAssertEqual(resolveDefaultHostedWorkspaceDisplayMode(isRegularWidth: true), .expanded)
        XCTAssertEqual(resolveDefaultHostedWorkspaceDisplayMode(isRegularWidth: false), .standard)
    }

    func testResolveActiveHostedWorkspaceWindowPrefersSelectedWindow() {
        let first = WorkspaceWindowSnapshot(windowId: "w1", windowKey: "order")
        let second = WorkspaceWindowSnapshot(windowId: "w2", windowKey: "order", workspaceSharePct: 72, workspaceMinHeight: 500)
        let restore = HostedWorkspaceRestoreState(windows: [first, second], selectedWindowId: "w2")

        let resolved = resolveActiveHostedWorkspaceWindow(restoreState: restore, conversationState: nil)

        XCTAssertEqual(resolved?.windowId, "w2")
        XCTAssertEqual(resolved?.workspaceSharePct, 72)
        XCTAssertEqual(resolved?.workspaceMinHeight, 500)
    }

    func testResolveHostedWorkspaceLayoutPlanHonorsLayoutHintsForExpandedMode() {
        let active = WorkspaceWindowSnapshot(
            windowId: "order_1",
            windowKey: "order",
            workspaceSharePct: 72,
            workspaceMinHeight: 500
        )

        let plan = resolveHostedWorkspaceLayoutPlan(
            availableHeight: 900,
            showsHostedWorkspace: true,
            displayMode: .expanded,
            isRegularWidth: true,
            transcriptMeasuredHeight: 90,
            hostedWorkspaceMeasuredHeight: 420,
            hostedWindowContentMeasuredHeight: 360,
            activeWindow: active
        )

        XCTAssertGreaterThanOrEqual(plan.workspaceHeight, 500)
        XCTAssertGreaterThanOrEqual(plan.transcriptHeight, 110)
    }

    func testResolveHostedWorkspaceLayoutPlanUsesContentMeasurementFallback() {
        let plan = resolveHostedWorkspaceLayoutPlan(
            availableHeight: 800,
            showsHostedWorkspace: true,
            displayMode: .standard,
            isRegularWidth: true,
            transcriptMeasuredHeight: 100,
            hostedWorkspaceMeasuredHeight: 0,
            hostedWindowContentMeasuredHeight: 300,
            activeWindow: nil
        )

        XCTAssertEqual(plan.workspaceHeight, 388)
    }

    func testResolveTranscriptHeightUsesMeasuredContentWithinBounds() {
        let compact = resolveTranscriptHeight(availableHeight: 900, isRegularWidth: true, measuredHeight: 80)
        XCTAssertEqual(compact, 110)

        let measured = resolveTranscriptHeight(availableHeight: 900, isRegularWidth: true, measuredHeight: 160)
        XCTAssertEqual(measured, 180)
    }
}
