import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ApprovalComponentsSupportTests: XCTestCase {
    func testBuildApprovalEditorSeedPreservesOriginalArgsPayload() {
        let meta = ApprovalMeta(
            toolName: "deploy",
            title: "Deploy approval",
            forge: ApprovalForgeView(containerRef: "approvalEnvPicker"),
            editors: [
                ApprovalEditor(
                    name: "envNames",
                    kind: "multi_list",
                    label: "Environments",
                    options: [
                        ApprovalOption(id: "dev", label: "Dev", selected: true),
                        ApprovalOption(id: "prod", label: "Prod")
                    ]
                )
            ]
        )
        let approvalValue = AppJSONValue.object([
            "id": .string("approval-1"),
            "conversationId": .string("conversation-1"),
            "messageId": .string("message-1"),
            "toolName": .string("deploy"),
            "title": .string("Deploy approval"),
            "status": .string("pending"),
            "arguments": .object([
                "envNames": .array([.string("dev"), .string("prod")]),
                "mode": .string("safe")
            ])
        ])
        let data = try! JSONEncoder.agently().encode(approvalValue)
        let approval = try! JSONDecoder.agently().decode(PendingToolApproval.self, from: data)

        let seed = buildApprovalEditorSeed(meta: meta, approval: approval, editors: meta.editors ?? [])

        XCTAssertNotNil(seed["approvalSchemaJSON"]?.stringValue)
        XCTAssertEqual(seed["envNames"], ForgeJSONValue.array([.string("dev"), .string("prod")]))
        XCTAssertEqual(seed["editedFields"]?.objectValue?["envNames"], ForgeJSONValue.array([.string("dev"), .string("prod")]))
        XCTAssertEqual(seed["originalArgs"]?.objectValue?["mode"], ForgeJSONValue.string("safe"))
        XCTAssertEqual(seed["approval"]?.objectValue?["toolName"], ForgeJSONValue.string("deploy"))
    }

    func testExtractApprovalEditedFieldsPrefersLiveFieldsOverSeededEditedFields() {
        let edited = extractApprovalEditedFields(from: [
            "approval": .object(["toolName": .string("deploy")]),
            "editedFields": .object([
                "envNames": .array([.string("prod")])
            ]),
            "envNames": .array([.string("dev")])
        ])

        XCTAssertEqual(edited, [
            "envNames": .array([.string("dev")])
        ])
    }

    func testExtractApprovalEditedFieldsFallsBackToSeededEditedFieldsWhenNoLiveFieldsExist() {
        let edited = extractApprovalEditedFields(from: [
            "approvalSchemaJSON": .string("{\"type\":\"object\"}"),
            "editedFields": .object([
                "envNames": .array([.string("prod")])
            ])
        ])

        XCTAssertEqual(edited, [
            "envNames": .array([.string("prod")])
        ])
    }

    func testApprovalArgumentsPreviewHidesTechnicalFieldsAndBuildsRows() {
        let preview = buildApprovalArgumentsPreview(.object([
            "Recommendation": .object([
                "audience_id": .number(7_314_989),
                "change_summary": .string("Exclude low-performing publisher paths."),
                "selector_direction": .string("EXCLUDE")
            ]),
            "rows": .array([
                .object([
                    "publisher": .string("37/3713495849"),
                    "recommendation": .string("EXCLUDE"),
                    "rationale": .string("Low click-through rate."),
                    "selected": .bool(true)
                ])
            ]),
            "timeoutMs": .number(600_000)
        ]))

        XCTAssertEqual(preview.rowsLabel, "Rows")
        XCTAssertEqual(preview.totalRows, 1)
        XCTAssertTrue(preview.summary.contains(ApprovalPreviewField(label: "Audience Id", value: "7314989")))
        XCTAssertFalse(preview.summary.contains { $0.label.contains("Timeout") })
        XCTAssertFalse(preview.rows.flatMap { $0 }.contains { $0.label == "Selected" })
    }

    func testApprovalFailureMessageExplainsEOF() {
        XCTAssertEqual(
            approvalFailureMessage("java.io.EOFException: source exhausted prematurely"),
            "The connection ended before the server confirmed approval. Your selection was kept; try again."
        )
    }
}
