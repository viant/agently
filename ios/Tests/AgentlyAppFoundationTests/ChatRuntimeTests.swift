import XCTest
import AgentlySDK
@testable import AgentlyAppFoundation

final class ChatRuntimeTests: XCTestCase {
    @MainActor
    func testApplyStreamingPrefersLiveExecutionGroups() {
        let runtime = ChatRuntime()
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-buffered",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "buffered fallback",
                    narration: "buffered narration",
                    status: "running"
                )
            ],
            liveExecutionGroupsByID: [
                "msg-live": LiveExecutionGroup(
                    pageID: "msg-live",
                    assistantMessageID: "msg-live",
                    turnID: "turn-1",
                    sequence: 2,
                    narration: "live narration",
                    content: "live content",
                    status: "running",
                    createdAt: "2026-04-15T22:00:00Z"
                )
            ]
        )

        runtime.applyStreaming(snapshot: snapshot)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertEqual(runtime.transcript.first?.id, "msg-live")
        XCTAssertEqual(runtime.transcript.first?.markdown, "live narration\n\nlive content")
        XCTAssertEqual(runtime.transcript.first?.statusLabel, "Streaming")
    }

    @MainActor
    func testApplyStreamingFallsBackToBufferedMessages() {
        let runtime = ChatRuntime()
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-buffered",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "buffered content",
                    narration: "buffered narration",
                    status: "completed"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        runtime.applyStreaming(snapshot: snapshot)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertEqual(runtime.transcript.first?.id, "msg-buffered")
        XCTAssertEqual(runtime.transcript.first?.markdown, "buffered narration\n\nbuffered content")
        XCTAssertNil(runtime.transcript.first?.statusLabel)
    }

    func testParseTranscriptContentPartsExtractsForgeUIBlocks() {
        let content = [
            "Intro text",
            "```forge-data",
            #"{"version":1,"id":"summary_metrics","data":[{"spend":1316.86,"pacing_ratio":0.17,"win_rate":4.02}]}"#,
            "```",
            "```forge-ui",
            #"{"version":1,"title":"Ad order 2639076","subtitle":"Agency 4257","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["spend","pacing_ratio","win_rate"]}]}"#,
            "```"
        ].joined(separator: "\n")

        let parts = parseTranscriptContentParts(content)

        XCTAssertEqual(parts.count, 2)
        if case .markdown(let intro) = parts[0] {
            XCTAssertTrue(intro.contains("Intro text"))
        } else {
            XCTFail("Expected leading markdown part")
        }
        if case .forgeUI(let payload, let dataStore) = parts[1] {
            XCTAssertEqual(payload.title, "Ad order 2639076")
            XCTAssertEqual(dataStore.keys.sorted(), ["summary_metrics"])
        } else {
            XCTFail("Expected forge-ui part")
        }
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsDashboardSummaryBlock() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "Ad order 2639076",
            subtitle: "Agency 4257",
            blocks: [
                .object([
                    "id": .string("summary"),
                    "kind": .string("dashboard.summary"),
                    "dataSourceRef": .string("summary_metrics"),
                    "metrics": .array([
                        .string("spend"),
                        .string("pacing_ratio"),
                        .string("win_rate")
                    ])
                ])
            ]
        )

        let metadata = try buildTranscriptForgeWindowMetadata(
            payload: payload,
            dataStore: [
                "summary_metrics": MaterializedForgeDataBlock(
                    id: "summary_metrics",
                    rows: .array([
                        .object([
                            "spend": .number(1316.86),
                            "pacing_ratio": .number(0.17),
                            "win_rate": .number(4.02)
                        ])
                    ])
                )
            ]
        )

        let root = try XCTUnwrap(metadata.view?.content?.containers.first)
        let summary = try XCTUnwrap(root.containers.first)
        XCTAssertEqual(root.title, "Ad order 2639076")
        XCTAssertEqual(summary.kind, "dashboard.summary")
        XCTAssertEqual(summary.dataSourceRef, "summary_metrics")
        XCTAssertEqual(summary.metrics.map(\.id), ["spend", "pacing_ratio", "win_rate"])
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsDashboardTimelineBlock() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "iOS dashboard verification",
            subtitle: "Agency 4257",
            blocks: [
                .object([
                    "id": .string("spend-trend"),
                    "kind": .string("dashboard.timeline"),
                    "title": .string("Spend trend"),
                    "dataSourceRef": .string("metrics_data"),
                    "chartType": .string("bar"),
                    "dateField": .string("channel"),
                    "series": .array([.string("spend")])
                ])
            ]
        )

        let metadata = try buildTranscriptForgeWindowMetadata(
            payload: payload,
            dataStore: [
                "metrics_data": MaterializedForgeDataBlock(
                    id: "metrics_data",
                    rows: .array([
                        .object([
                            "channel": .string("Alpha"),
                            "spend": .number(1316.86)
                        ]),
                        .object([
                            "channel": .string("Beta"),
                            "spend": .number(842.10)
                        ])
                    ])
                )
            ]
        )

        let root = try XCTUnwrap(metadata.view?.content?.containers.first)
        let chart = try XCTUnwrap(root.containers.first)
        XCTAssertEqual(chart.dataSourceRef, "metrics_data")
        XCTAssertEqual(chart.chart?.type, "bar")
        XCTAssertEqual(chart.chart?.xKey, "channel")
        XCTAssertEqual(chart.chart?.series, ["spend"])
    }
}
