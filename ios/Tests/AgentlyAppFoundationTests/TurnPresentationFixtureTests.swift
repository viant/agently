import Foundation
import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class TurnPresentationFixtureTests: XCTestCase {
    func testSharedProgressFixtures() throws {
        let fixture = try loadFixture()
        for testCase in fixture.progressCases {
            let input = testCase.input
            let expected = testCase.expected
            let turnID = input.turnId
            let terminal = expected == nil
            let groups = input.groups.enumerated().map { index, group in
                LiveExecutionGroup(
                    pageID: "fixture-page-\(index)",
                    assistantMessageID: "fixture-message-\(index)",
                    turnID: turnID,
                    status: input.status,
                    toolSteps: group.toolSteps.map {
                        LiveToolStepState(toolCallID: $0.toolCallId, toolName: $0.toolName, status: $0.status)
                    },
                    toolCallsPlanned: group.toolCallsPlanned.map {
                        PlannedToolCall(toolCallID: $0.toolCallId, toolName: $0.toolName)
                    }
                )
            }
            let planner: [String: PlannerState]
            if input.phase?.lowercased().contains("plan") == true {
                planner = [turnID: try JSONDecoder().decode(PlannerState.self, from: Data(#"{"status":"running"}"#.utf8))]
            } else {
                planner = [:]
            }
            let snapshot = terminal ? nil : ConversationStreamSnapshot(
                conversationID: "fixture",
                activeTurnID: turnID,
                liveExecutionGroupsByID: Dictionary(uniqueKeysWithValues: groups.map { ($0.assistantMessageID, $0) }),
                plannerByTurnID: planner
            )
            let conversationState = terminal ? nil : ConversationStateResponse(
                conversation: ConversationState(
                    conversationID: "fixture",
                    turns: [TurnState(turnID: turnID, status: input.status)]
                )
            )
            let actual = turnProgressPresentation(
                isSending: !terminal,
                activeTurnID: terminal ? nil : turnID,
                isStoppingTurn: false,
                conversationState: conversationState,
                streamSnapshot: snapshot
            )

            if expected == nil {
                XCTAssertNil(actual, testCase.name)
                continue
            }
            XCTAssertEqual(actual?.activity, expectedActivity(expected!), testCase.name)
            XCTAssertEqual(actual?.toolProgress, expectedToolProgress(expected!), testCase.name)
            if let canStop = expected?.canStop {
                XCTAssertEqual(actual?.canStop, canStop, testCase.name)
            }
        }
    }

    @MainActor
    func testSharedNarrationFixtures() throws {
        let fixture = try loadFixture()
        for testCase in fixture.narrationCases {
            let input = testCase.input
            let messageID = input.narrationMessageId ?? "\(input.turnId):narration"
            let snapshot = ConversationStreamSnapshot(
                conversationID: "fixture",
                activeTurnID: input.turnId,
                bufferedMessages: [
                    BufferedStreamMessage(
                        id: messageID,
                        turnID: input.turnId,
                        narration: input.candidates.first ?? "",
                        status: "running"
                    )
                ]
            )
            let actual = ChatRuntime().transcriptWithActiveAssistant(snapshot: snapshot).last
            XCTAssertEqual(actual?.markdown, testCase.expected?.content, testCase.name)
            if let expected = testCase.expected {
                XCTAssertEqual(actual?.id, expected.messageId, testCase.name)
            } else {
                XCTAssertNil(actual, testCase.name)
            }
        }
    }

    private func loadFixture() throws -> TurnPresentationFixture {
        var viantRoot = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        for _ in 0..<4 { viantRoot.deleteLastPathComponent() }
        let url = viantRoot.appendingPathComponent("agently-core/sdk/fixtures/turn_presentation.json")
        return try JSONDecoder().decode(TurnPresentationFixture.self, from: Data(contentsOf: url))
    }

    private func expectedActivity(_ expected: FixtureProgressExpected) -> String {
        switch expected.activity?.kind {
        case "connecting": return "Connecting"
        case "planning": return "Planning"
        case "stopping": return "Stopping"
        case "tool": return expected.activity?.label ?? "Using tool"
        case "tools": return "Calling tools"
        case "waiting_for_user": return "Needs your input"
        case "writing": return "Writing response"
        default: return "Thinking"
        }
    }

    private func expectedToolProgress(_ expected: FixtureProgressExpected) -> String? {
        guard expected.identityComplete != false, (expected.totalToolCount ?? 0) > 0 else { return nil }
        var parts = ["\(expected.completedToolCount ?? 0)/\(expected.totalToolCount ?? 0) done"]
        if (expected.activeToolCount ?? 0) > 0 { parts.append("\(expected.activeToolCount!) active") }
        if (expected.queuedToolCount ?? 0) > 0 { parts.append("\(expected.queuedToolCount!) queued") }
        if (expected.failedToolCount ?? 0) > 0 { parts.append("\(expected.failedToolCount!) failed") }
        return parts.joined(separator: " · ")
    }
}

private struct TurnPresentationFixture: Decodable {
    let progressCases: [FixtureProgressCase]
    let narrationCases: [FixtureNarrationCase]
}

private struct FixtureProgressCase: Decodable {
    let name: String
    let input: FixtureProgressInput
    let expected: FixtureProgressExpected?
}

private struct FixtureProgressInput: Decodable {
    let turnId: String
    let status: String
    let phase: String?
    let groups: [FixtureGroup]

    enum CodingKeys: String, CodingKey { case turnId, status, phase, groups }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        turnId = try values.decode(String.self, forKey: .turnId)
        status = try values.decode(String.self, forKey: .status)
        phase = try values.decodeIfPresent(String.self, forKey: .phase)
        groups = try values.decodeIfPresent([FixtureGroup].self, forKey: .groups) ?? []
    }
}

private struct FixtureGroup: Decodable {
    let toolSteps: [FixtureTool]
    let toolCallsPlanned: [FixtureTool]
    enum CodingKeys: String, CodingKey { case toolSteps, toolCallsPlanned }
    init(from decoder: Decoder) throws {
        let values = try decoder.container(keyedBy: CodingKeys.self)
        toolSteps = try values.decodeIfPresent([FixtureTool].self, forKey: .toolSteps) ?? []
        toolCallsPlanned = try values.decodeIfPresent([FixtureTool].self, forKey: .toolCallsPlanned) ?? []
    }
}

private struct FixtureTool: Decodable {
    let toolCallId: String?
    let toolName: String?
    let status: String?
}

private struct FixtureProgressExpected: Decodable {
    let activity: FixtureActivity?
    let completedToolCount: Int?
    let activeToolCount: Int?
    let queuedToolCount: Int?
    let failedToolCount: Int?
    let totalToolCount: Int?
    let identityComplete: Bool?
    let canStop: Bool?
}

private struct FixtureActivity: Decodable { let kind: String; let label: String? }

private struct FixtureNarrationCase: Decodable {
    let name: String
    let input: FixtureNarrationInput
    let expected: FixtureNarrationExpected?
}

private struct FixtureNarrationInput: Decodable {
    let turnId: String
    let narrationMessageId: String?
    let candidates: [String]
}

private struct FixtureNarrationExpected: Decodable {
    let messageId: String
    let content: String
}
