import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ApprovalCallbacksTests: XCTestCase {
    func testExecuteApprovalCallbacksFiltersSelectionsToOriginalOrder() async {
        let meta = ApprovalMeta(
            toolName: "deploy",
            forge: ApprovalForgeView(
                callbacks: [
                    ApprovalCallback(
                        event: "approve",
                        handler: "approval.filterEnvNames"
                    )
                ]
            )
        )
        let payload = ApprovalCallbackPayload(
            editedFields: [
                "names": .array([
                    .string("prod"),
                    .string("dev")
                ])
            ],
            originalArgs: [
                "names": .array([
                    .string("dev"),
                    .string("stage"),
                    .string("prod")
                ])
            ],
            action: "approve"
        )

        let result = await ApprovalCallbacks.execute(meta: meta, event: "approve", payload: payload)

        XCTAssertEqual(result.editedFields["names"], .array([.string("dev"), .string("prod")]))
        XCTAssertEqual(result.action, "approve")
    }

    func testExecuteApprovalCallbacksSkipsMismatchedEvent() async {
        let meta = ApprovalMeta(
            toolName: "deploy",
            forge: ApprovalForgeView(
                callbacks: [
                    ApprovalCallback(
                        event: "decline",
                        handler: "approval.filterEnvNames"
                    )
                ]
            )
        )
        let payload = ApprovalCallbackPayload(
            editedFields: [
                "names": .array([.string("prod"), .string("dev")])
            ],
            originalArgs: [
                "names": .array([.string("dev"), .string("prod")])
            ],
            action: "approve"
        )

        let result = await ApprovalCallbacks.execute(meta: meta, event: "approve", payload: payload)

        XCTAssertEqual(result.editedFields["names"], .array([.string("prod"), .string("dev")]))
    }

    func testBuildApprovalDecisionRequestAppliesMetadataCallbacks() async throws {
        let approval = try decodeApproval([
            "id": .string("approval-1"),
            "toolName": .string("deploy"),
            "status": .string("pending"),
            "arguments": .object([
                "names": .array([
                    .string("dev"),
                    .string("stage"),
                    .string("prod")
                ])
            ]),
            "metadata": .object([
                "toolName": .string("deploy"),
                "forge": .object([
                    "callbacks": .array([
                        .object([
                            "event": .string("approve"),
                            "handler": .string("approval.filterEnvNames")
                        ])
                    ])
                ])
            ])
        ])

        let request = await buildApprovalDecisionRequest(
            approval: approval,
            action: "approve",
            editedFields: [
                "names": .array([.string("prod"), .string("dev")])
            ]
        )

        XCTAssertEqual(request.id, "approval-1")
        XCTAssertEqual(request.action, "approve")
        XCTAssertEqual(request.editedFields["names"], .array([.string("dev"), .string("prod")]))
    }

    func testMergeCallbackResultAppliesCurrentEditedFieldsAndActionShape() {
        let payload = ApprovalCallbackPayload(
            editedFields: ["names": .array([.string("dev")])],
            originalArgs: [:],
            action: "approve"
        )

        let result = ApprovalCallbackResult(
            editedFields: ["names": .array([.string("prod")])],
            action: "decline"
        )
        let merged = ApprovalCallbacks.mergeCallbackResult(payload: payload, result: result)

        XCTAssertEqual(merged.action, "decline")
        XCTAssertEqual(merged.editedFields["names"], .array([.string("prod")]))
        XCTAssertNil(merged.editedFields["action"])
    }

    func testMergeCallbackResultTreatsLegacyPayloadActionAsControlMetadata() {
        let payload = ApprovalCallbackPayload(
            editedFields: ["mode": .string("safe")],
            originalArgs: [:],
            action: "approve"
        )

        let result = ApprovalCallbackResult(
            payload: [
                "action": .string("decline"),
                "names": .array([.string("prod")])
            ]
        )
        let merged = ApprovalCallbacks.mergeCallbackResult(payload: payload, result: result)

        XCTAssertEqual(merged.action, "decline")
        XCTAssertEqual(merged.editedFields["mode"], .string("safe"))
        XCTAssertEqual(merged.editedFields["names"], .array([.string("prod")]))
        XCTAssertNil(merged.editedFields["action"])
    }

    private func decodeApproval(_ object: [String: JSONValue]) throws -> PendingToolApproval {
        let data = try JSONEncoder.agently().encode(JSONValue.object(object))
        return try JSONDecoder.agently().decode(PendingToolApproval.self, from: data)
    }
}
