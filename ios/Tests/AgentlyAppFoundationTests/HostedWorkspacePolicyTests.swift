import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class HostedWorkspacePolicyTests: XCTestCase {
    func testFilterAgentlyHostedWorkspaceRestoreStateSeedsParametersIntoWindowForm() {
        let restore = filterAgentlyHostedWorkspaceRestoreState(
            HostedWorkspaceRestoreState(
                windows: [
                    WorkspaceWindowSnapshot(
                        windowId: "builder__conv-1",
                        conversationId: "conv-1",
                        windowKey: "genericBuilder",
                        windowTitle: "Generic Builder",
                        presentation: "hosted",
                        region: "chat.top",
                        parentKey: "chat/new",
                        parameters: [
                            "prefill": .object([
                                "recordId": .number(12345),
                                "targetKey": .string("record:12345")
                            ])
                        ],
                        windowForm: [
                            "prefill": .object([
                                "targetKey": .string("override:12345"),
                                "source": .string("transcript")
                            ])
                        ]
                    )
                ],
                selectedWindowId: "builder__conv-1"
            )
        )

        let prefill = restore?.windows.single?.windowForm?["prefill"]?.objectValue
        XCTAssertEqual(prefill?["recordId"], .number(12345))
        XCTAssertEqual(prefill?["targetKey"], .string("override:12345"))
        XCTAssertEqual(prefill?["source"], .string("transcript"))
    }

    func testHostedWorkspaceEventNoticeDescribesInvalidWorkspaceIDEvent() {
        let notice = hostedWorkspaceEventNotice(
            from: [
                UIEvent(
                    seq: 7,
                    kind: "error",
                    detail: [
                        "payload": .object([
                            "invalidWorkspaceId": .string("missingView"),
                            "availableWorkspaceIds": .array([
                                .string("summary"),
                                .string("builder")
                            ])
                        ])
                    ]
                )
            ]
        )

        XCTAssertEqual(notice?.invalidWorkspaceID, "missingView")
        XCTAssertEqual(notice?.availableWorkspaceIDs, ["summary", "builder"])
        XCTAssertEqual(
            notice?.message,
            "Workspace view \"missingView\" is not published for this workspace. Available views: summary, builder."
        )
    }

    func testShouldApplyHostedWorkspaceNoticeRequiresMatchingActiveConversationID() {
        XCTAssertTrue(shouldApplyHostedWorkspaceNotice(activeConversationID: " conv-1 ", targetConversationID: "conv-1"))
        XCTAssertFalse(shouldApplyHostedWorkspaceNotice(activeConversationID: "conv-2", targetConversationID: "conv-1"))
        XCTAssertFalse(shouldApplyHostedWorkspaceNotice(activeConversationID: nil, targetConversationID: "conv-1"))
        XCTAssertFalse(shouldApplyHostedWorkspaceNotice(activeConversationID: "conv-1", targetConversationID: " "))
    }

    func testHostedWorkspaceLoaderReusesMatchingExistingWindow() {
        let existing = ForgeRuntime.WindowState(
            id: "order__conv-1",
            key: "order",
            title: "Order Summary",
            metadata: WindowMetadata(view: ViewDef(content: ContentDef())),
            conversationID: "conv-1",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )
        let selected = WorkspaceWindowSnapshot(
            windowId: "order__conv-1",
            conversationId: "conv-1",
            windowKey: "order",
            windowTitle: "Order Summary",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )

        XCTAssertTrue(shouldReuseExistingHostedWorkspaceWindow(existing, for: selected))
    }

    func testHostedWorkspaceLoaderDoesNotReuseDifferentWindowKey() {
        let existing = ForgeRuntime.WindowState(
            id: "order__conv-1",
            key: "order",
            title: "Order Summary",
            metadata: WindowMetadata(view: ViewDef(content: ContentDef())),
            conversationID: "conv-1",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )
        let selected = WorkspaceWindowSnapshot(
            windowId: "order__conv-1",
            conversationId: "conv-1",
            windowKey: "campaign",
            windowTitle: "Campaign",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )

        XCTAssertFalse(shouldReuseExistingHostedWorkspaceWindow(existing, for: selected))
    }

    func testHostedWorkspaceLoaderDoesNotReuseWindowWithoutMetadata() {
        let existing = ForgeRuntime.WindowState(
            id: "line__conv-1",
            key: "line",
            title: "Line Summary",
            metadata: nil,
            conversationID: "conv-1",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )
        let selected = WorkspaceWindowSnapshot(
            windowId: "line__conv-1",
            conversationId: "conv-1",
            windowKey: "line",
            windowTitle: "Line Summary",
            presentation: "hosted",
            region: "chat.top",
            parentKey: "chat/new"
        )

        XCTAssertFalse(shouldReuseExistingHostedWorkspaceWindow(existing, for: selected))
    }
}

private extension Array {
    var single: Element? {
        count == 1 ? first : nil
    }
}

private extension AgentlySDK.JSONValue {
    var objectValue: [String: AgentlySDK.JSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }
}
