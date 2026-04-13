import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ApprovalMetadataSupportTests: XCTestCase {
    func testExtractToolApprovalMetaReadsRichHiddenApprovalMeta() {
        let schema = AppJSONValue.object([
            "type": .string("object"),
            "properties": .object([
                "_approvalMeta": .object([
                    "const": .string("""
                    {
                      "type":"tool_approval",
                      "title":"Deploy approval",
                      "toolName":"deploy",
                      "acceptLabel":"Approve",
                      "rejectLabel":"Reject",
                      "cancelLabel":"Cancel",
                      "editors":[
                        {
                          "name":"env",
                          "kind":"radio_list",
                          "label":"Environment",
                          "options":[
                            {"id":"prod","label":"Production","selected":true}
                          ]
                        }
                      ]
                    }
                    """)
                ])
            ])
        ])

        guard case .object(let object) = schema else {
            return XCTFail("expected object schema")
        }

        let meta = ApprovalMetadataSupport.extractToolApprovalMeta(object)
        XCTAssertEqual(meta?.type, "tool_approval")
        XCTAssertEqual(meta?.title, "Deploy approval")
        XCTAssertEqual(meta?.toolName, "deploy")
        XCTAssertEqual(meta?.acceptLabel, "Approve")
        XCTAssertEqual(meta?.editors?.first?.name, "env")
    }

    func testExtractToolApprovalMetaReadsLegacyHiddenFields() {
        let schema = AppJSONValue.object([
            "type": .string("object"),
            "properties": .object([
                "_type": .object(["const": .string("tool_approval")]),
                "_title": .object(["const": .string("Approval Required")]),
                "_toolName": .object(["const": .string("shell.exec")]),
                "_acceptLabel": .object(["const": .string("Allow")]),
                "_rejectLabel": .object(["const": .string("Deny")]),
                "_cancelLabel": .object(["const": .string("Back")])
            ])
        ])

        guard case .object(let object) = schema else {
            return XCTFail("expected object schema")
        }

        let meta = ApprovalMetadataSupport.extractToolApprovalMeta(object)
        XCTAssertEqual(meta?.type, "tool_approval")
        XCTAssertEqual(meta?.title, "Approval Required")
        XCTAssertEqual(meta?.toolName, "shell.exec")
        XCTAssertEqual(meta?.acceptLabel, "Allow")
        XCTAssertEqual(meta?.rejectLabel, "Deny")
        XCTAssertEqual(meta?.cancelLabel, "Back")
    }

    func testParsedApprovalMetaReadsWrappedApprovalObjectAndForgeFields() {
        let approvalValue = AppJSONValue.object([
            "id": .string("approval-1"),
            "conversationId": .string("conversation-1"),
            "messageId": .string("message-1"),
            "toolName": .string("deploy"),
            "title": .string("Deploy approval"),
            "status": .string("pending"),
            "metadata": .object([
                "approval": AppJSONValue.object([
                    "type": .string("tool_approval"),
                    "toolName": .string("deploy"),
                    "title": .string("Deploy approval"),
                    "acceptLabel": .string("Ship"),
                    "rejectLabel": .string("Stop"),
                    "cancelLabel": .string("Later"),
                    "forge": AppJSONValue.object([
                        "windowRef": .string("chat/new/dialog/approval_editor"),
                        "containerRef": .string("approvalEditor"),
                        "dataSource": .string("approvalEditorForm")
                    ]),
                    "editors": AppJSONValue.array([
                        AppJSONValue.object([
                            "name": .string("env"),
                            "kind": .string("radio_list"),
                            "label": .string("Environment"),
                            "options": AppJSONValue.array([
                                AppJSONValue.object([
                                    "id": .string("prod"),
                                    "label": .string("Production"),
                                    "selected": .bool(true)
                                ])
                            ])
                        ])
                    ])
                ])
            ])
        ])

        let data = try! JSONEncoder.agently().encode(approvalValue)
        let approval = try! JSONDecoder.agently().decode(PendingToolApproval.self, from: data)

        let meta = ApprovalMetadataSupport.parsedApprovalMeta(approval)
        XCTAssertEqual(meta?.acceptLabel, "Ship")
        XCTAssertEqual(meta?.rejectLabel, "Stop")
        XCTAssertEqual(meta?.cancelLabel, "Later")
        XCTAssertEqual(meta?.forge?.windowRef, "chat/new/dialog/approval_editor")
        XCTAssertEqual(meta?.forge?.containerRef, "approvalEditor")
        XCTAssertEqual(meta?.forge?.dataSource, "approvalEditorForm")
        XCTAssertEqual(meta?.editors?.first?.kind, "radio_list")
    }
}
