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
}
