import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ChatRuntimeTests: XCTestCase {
    func testConversationUsageIncludesSidecarModels() {
        let state = ConversationStateResponse(
            conversation: ConversationState(conversationID: "conv-usage", turns: []),
            usage: UsageSummary(
                totalInputTokens: 1_000,
                totalOutputTokens: 100,
                totalCachedInputTokens: 0,
                totalReasoningTokens: 0,
                totalEmbeddingTokens: 0,
                totalTokens: 1_100,
                cost: nil,
                models: [
                    UsageModelSummary(provider: "openai", model: "gpt-5-mini", executionRole: "react", inputTokens: 800, outputTokens: 80, cachedInputTokens: 0, reasoningTokens: 0, totalTokens: 880, cost: nil),
                    UsageModelSummary(provider: "openai", model: "gpt-5-mini", executionRole: "intake", inputTokens: 200, outputTokens: 20, cachedInputTokens: 0, reasoningTokens: 0, totalTokens: 220, cost: nil)
                ]
            )
        )

        let presentation = turnProgressPresentation(
            isSending: true,
            activeTurnID: nil,
            isStoppingTurn: false,
            conversationState: state,
            streamSnapshot: nil
        )

        XCTAssertEqual(presentation?.tokenUsage, "1,100 total tokens")
        XCTAssertEqual(presentation?.tokenDetails?.models.map(\.label), ["openai/gpt-5-mini · react", "openai/gpt-5-mini · intake"])
    }
    func testTranscriptWindowStartsWithLatestFortyAndExpands() {
        XCTAssertEqual(transcriptWindowStart(totalItemCount: 100, visibleItemCount: 40), 60)
        XCTAssertEqual(transcriptWindowStart(totalItemCount: 32, visibleItemCount: 40), 0)
        XCTAssertEqual(transcriptWindowStart(totalItemCount: 80, visibleItemCount: 80), 0)
        XCTAssertEqual(transcriptWindowStart(totalItemCount: 12, visibleItemCount: -1), 12)
    }

    func testVisibleQueryErrorHidesBackendAndEndpointDetails() {
        XCTAssertEqual(
            visibleQueryError(AgentlySDKError.httpStatus(500, "failed to stream: API key is required")),
            "The workspace model is not configured. Ask an administrator to add the model API key, then try again."
        )
        XCTAssertEqual(
            visibleQueryError(AgentlySDKError.httpStatus(500, "POST /v1/agent/query failed")),
            "The assistant could not start this request. Try again, or contact the workspace administrator if it continues."
        )
        XCTAssertEqual(
            visibleQueryError(NSError(domain: NSCocoaErrorDomain, code: 0, userInfo: [NSLocalizedDescriptionKey: "EOFException"])),
            "The connection ended before the report finished loading. Refresh to try again."
        )
    }

    @MainActor
    func testLatestAssistantMarkdownPrefersNewestActiveAssistantMessage() {
        let runtime = ChatRuntime()
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-2",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-1",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "Earlier",
                    status: "completed"
                ),
                BufferedStreamMessage(
                    id: "msg-2",
                    conversationID: "conv-1",
                    turnID: "turn-2",
                    content: "Latest",
                    narration: "Heads up",
                    status: "running",
                    createdAt: "2026-04-15T22:00:00Z"
                ),
                BufferedStreamMessage(
                    id: "msg-3",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "Historical but newer",
                    status: "completed"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        XCTAssertEqual(runtime.latestAssistantMarkdown(snapshot: snapshot), "Latest")
    }

    @MainActor
    func testActiveAssistantSuppressesIncompleteReportTransportAndKeepsNarration() {
        let runtime = ChatRuntime()
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-1",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: """
                    ```forge-report
                    {"version":1,"scope":"delivery","id":"review","blocks":[
                    """,
                    narration: "Checking delivery evidence",
                    status: "running"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        XCTAssertEqual(runtime.latestAssistantMarkdown(snapshot: snapshot), "Checking delivery evidence")
    }

    @MainActor
    func testTranscriptWithActiveAssistantAppendsActiveEntryWithoutMutatingHistory() {
        let runtime = ChatRuntime()
        runtime.appendAssistantMessage("existing history")
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-history",
                    conversationID: "conv-1",
                    turnID: "turn-history",
                    content: "historical content",
                    narration: "historical narration",
                    status: "completed"
                ),
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

        let displayTranscript = runtime.transcriptWithActiveAssistant(snapshot: snapshot)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertEqual(runtime.transcript.first?.markdown, "existing history")
        XCTAssertEqual(displayTranscript.count, 2)
        XCTAssertEqual(displayTranscript.last?.id, "msg-buffered")
        XCTAssertEqual(displayTranscript.last?.markdown, "buffered content")
        XCTAssertEqual(displayTranscript.last?.statusLabel, "Streaming")
    }

    func testTurnProgressUsesLatestNarrationAndReportingTool() {
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "assistant-1",
                    turnID: "turn-1",
                    narration: "Checking delivery evidence.",
                    status: "running"
                )
            ],
            liveExecutionGroupsByID: [
                "assistant-1": LiveExecutionGroup(
                    pageID: "page-1",
                    assistantMessageID: "assistant-1",
                    turnID: "turn-1",
                    sequence: 1,
                    narration: "Validating performance metrics.",
                    status: "running",
                    toolSteps: [
                        LiveToolStepState(
                            toolCallID: "tool-1",
                            toolName: "reporting:run_report",
                            status: "running"
                        )
                    ]
                )
            ],
            usage: StreamUsageState(inputTokens: 120, outputTokens: 30, totalTokens: 150)
        )

        let presentation = turnProgressPresentation(
            isSending: true,
            activeTurnID: "turn-1",
            isStoppingTurn: false,
            conversationState: nil,
            streamSnapshot: snapshot
        )

        XCTAssertEqual(presentation?.title, "Working on your request")
        XCTAssertEqual(presentation?.detail, "Using a workspace tool.")
        XCTAssertEqual(presentation?.activity, "Reporting")
        XCTAssertEqual(presentation?.toolProgress, "0/1 done · 1 active")
        XCTAssertEqual(presentation?.toolDetails.map(\.name), ["reporting:run_report"])
        XCTAssertEqual(presentation?.toolDetails.map(\.status), ["running"])
        XCTAssertEqual(presentation?.tokenUsage, "150 total tokens")
        XCTAssertEqual(presentation?.canStop, true)
    }

    func testTurnProgressDistinguishesQueuedToolsAndWaitingForInput() {
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [TurnState(turnID: "turn-1", status: "waiting_for_user")]
            )
        )
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            liveExecutionGroupsByID: [
                "assistant-1": LiveExecutionGroup(
                    pageID: "page-1",
                    assistantMessageID: "assistant-1",
                    turnID: "turn-1",
                    status: "running",
                    toolSteps: [LiveToolStepState(toolCallID: "queued-1", toolName: "search_orders", status: "queued")]
                )
            ]
        )

        let presentation = turnProgressPresentation(
            isSending: true,
            activeTurnID: "turn-1",
            isStoppingTurn: false,
            conversationState: state,
            streamSnapshot: snapshot
        )

        XCTAssertEqual(presentation?.title, "Needs your input")
        XCTAssertEqual(presentation?.activity, "Needs your input")
        XCTAssertEqual(presentation?.toolProgress, "0/1 done · 1 queued")
        XCTAssertEqual(presentation?.toolDetails.first?.status, "queued")
        XCTAssertEqual(presentation?.isWaitingForUser, true)
        XCTAssertEqual(presentation?.canStop, false)
    }

    func testTurnProgressPrefersPerModelTurnTokenDetails() {
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            liveExecutionGroupsByID: [
                "assistant-1": LiveExecutionGroup(
                    pageID: "page-1",
                    assistantMessageID: "assistant-1",
                    turnID: "turn-1",
                    status: "running",
                    modelSteps: [
                        LiveModelStepState(
                            modelCallID: "model-1",
                            provider: "openai",
                            model: "gpt-5",
                            usage: ModelUsageState(inputTokens: 100, outputTokens: 30, cachedInputTokens: 20, reasoningTokens: 5, totalTokens: 130)
                        ),
                        LiveModelStepState(
                            modelCallID: "model-2",
                            provider: "openai",
                            model: "gpt-5-mini",
                            usage: ModelUsageState(inputTokens: 20, outputTokens: 10, totalTokens: 30)
                        )
                    ]
                )
            ],
            usage: StreamUsageState(inputTokens: 900, outputTokens: 100, totalTokens: 1_000)
        )

        let presentation = turnProgressPresentation(
            isSending: true,
            activeTurnID: "turn-1",
            isStoppingTurn: false,
            conversationState: nil,
            streamSnapshot: snapshot
        )

        XCTAssertEqual(presentation?.tokenUsage, "160 turn tokens")
        XCTAssertEqual(presentation?.tokenDetails?.scope, "turn")
        XCTAssertEqual(presentation?.tokenDetails?.cachedInput, 20)
        XCTAssertEqual(presentation?.tokenDetails?.reasoning, 5)
        XCTAssertEqual(presentation?.tokenDetails?.models.map(\.label), ["openai/gpt-5", "openai/gpt-5-mini"])
        XCTAssertEqual(presentation?.tokenDetails?.models.first?.input, 100)
        XCTAssertEqual(presentation?.tokenDetails?.models.first?.output, 30)
        XCTAssertEqual(presentation?.tokenDetails?.models.first?.cachedInput, 20)
        XCTAssertEqual(presentation?.tokenDetails?.models.first?.reasoning, 5)
        XCTAssertNil(presentation?.tokenDetails?.models.first?.embedding)
    }

    func testTurnProgressSurvivesStreamGapUsingPersistedPendingTurn() {
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [
                    TurnState(
                        turnID: "turn-1",
                        status: "running",
                        assistant: AssistantState(
                            narration: AssistantMessageState(
                                messageID: "n1",
                                content: "Checking delivery evidence."
                            )
                        )
                    )
                ]
            )
        )

        let presentation = turnProgressPresentation(
            isSending: false,
            activeTurnID: nil,
            isStoppingTurn: false,
            conversationState: state,
            streamSnapshot: nil
        )

        XCTAssertEqual(presentation?.title, "Working on your request")
        XCTAssertEqual(presentation?.detail, "Planning the next step.")
        XCTAssertEqual(presentation?.activity, "Thinking")
        XCTAssertEqual(presentation?.canStop, true)
    }

    @MainActor
    func testTranscriptWithActiveAssistantUsesSingleGlobalProgressInsteadOfWaitingPlaceholder() {
        let runtime = ChatRuntime()
        let optimistic = runtime.beginOptimisticTurn(query: "open report")
        runtime.markOptimisticTurnAccepted(optimistic)
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "assistant-real-1",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "Opening report.",
                    status: "running"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        let displayTranscript = runtime.transcriptWithActiveAssistant(snapshot: snapshot)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertFalse(runtime.transcript.contains { $0.id == optimistic.assistantEntryID })
        XCTAssertEqual(displayTranscript.count, 2)
        XCTAssertEqual(displayTranscript.first?.id, optimistic.userEntryID)
        XCTAssertEqual(displayTranscript.last?.id, "assistant-real-1")
        XCTAssertFalse(displayTranscript.contains { $0.id == optimistic.assistantEntryID })
    }

    @MainActor
    func testReplaceTranscriptStripsHiddenRouterPayloadSuffix() {
        let runtime = ChatRuntime()
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [
                    TurnState(
                        turnID: "turn-1",
                        user: UserMessageState(messageID: "u1", content: "troubleshoot order"),
                        assistant: AssistantState(
                            final: AssistantMessageState(
                                messageID: "a1",
                                content: """
                                I can troubleshoot the order, but I need the exact entity ID because “order” by itself is ambiguous. {
                                  "classification": { "intent": "troubleshoot_delivery_blocker" },
                                  "prompting": { "suggestedProfileId": "diagnostic_baseline" },
                                  "scope": { "values": { "entityType": "Entity" } }
                                }
                                """,
                                createdAt: "2026-06-03T21:00:00Z"
                            )
                        ),
                        createdAt: "2026-06-03T21:00:00Z"
                    )
                ]
            )
        )

        runtime.replaceTranscript(from: state)

        XCTAssertEqual(runtime.transcript.count, 2)
        XCTAssertEqual(
            runtime.transcript.last?.markdown,
            "I can troubleshoot the order, but I need the exact entity ID because “order” by itself is ambiguous."
        )
    }

    @MainActor
    func testReplaceTranscriptDoesNotDuplicateNarrationBesideFinalAnswer() {
        let runtime = ChatRuntime()
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [
                    TurnState(
                        turnID: "turn-1",
                        status: "completed",
                        assistant: AssistantState(
                            narration: AssistantMessageState(messageID: "n1", content: "Waiting for response"),
                            final: AssistantMessageState(messageID: "a1", content: "The report is ready.")
                        )
                    )
                ]
            )
        )

        runtime.replaceTranscript(from: state)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertEqual(runtime.transcript[0].markdown, "The report is ready.")
    }

    @MainActor
    func testReplaceTranscriptSeparatesNarrationFromFinalMarkdownHeading() throws {
        let narrationRendered = try JSONDecoder().decode(RenderedContent.self, from: Data(#"""
        {"schemaVersion":"1","parts":[{"kind":"markdown","text":"I’ll check delivery evidence."}]}
        """#.utf8))
        let finalRendered = try JSONDecoder().decode(RenderedContent.self, from: Data(#"""
        {"schemaVersion":"1","parts":[{"kind":"markdown","text":"### Key findings\n- **Primary blocker:** bid competitiveness."}]}
        """#.utf8))
        let runtime = ChatRuntime()
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [
                    TurnState(
                        turnID: "turn-1",
                        status: "completed",
                        assistant: AssistantState(
                            narration: AssistantMessageState(
                                messageID: "n1",
                                content: "I’ll check delivery evidence.",
                                renderedContent: narrationRendered
                            ),
                            final: AssistantMessageState(
                                messageID: "a1",
                                content: "### Key findings\n- **Primary blocker:** bid competitiveness.",
                                renderedContent: finalRendered
                            )
                        )
                    )
                ]
            )
        )

        runtime.replaceTranscript(from: state)

        let parts = try XCTUnwrap(runtime.transcript.first?.renderedParts)
        XCTAssertEqual(parts[0].text, "I’ll check delivery evidence.")
        XCTAssertEqual(parts[1].text, "\n\n### Key findings\n- **Primary blocker:** bid competitiveness.")
    }

    func testPendingInlineReportStatusIncludesDataSourcesAndBlocks() {
        let report = TranscriptCanonicalReport(
            scope: "order-1",
            id: "delivery",
            grammar: "report-document-v1",
            status: "rendering",
            source: .object([
                "title": .string("Delivery"),
                "blocks": .array([
                    .object(["id": .string("summary")]),
                    .object(["id": .string("details")])
                ])
            ]),
            dataSources: [
                "overview": TranscriptCanonicalData(id: "overview"),
                "daily": TranscriptCanonicalData(id: "daily")
            ]
        )

        XCTAssertTrue(isPendingInlineReport(report))
        XCTAssertEqual(inlineReportBuildStatus(report), "Building report · 2 data sources · 2 blocks")
    }

    @MainActor
    func testReplaceTranscriptUsesPendingNarrationAsAssistantBubble() {
        let runtime = ChatRuntime()
        let state = ConversationStateResponse(
            conversation: ConversationState(
                conversationID: "conv-1",
                turns: [
                    TurnState(
                        turnID: "turn-1",
                        status: "running",
                        assistant: AssistantState(
                            narration: AssistantMessageState(messageID: "n1", content: "Waiting for response")
                        )
                    )
                ]
            )
        )

        runtime.replaceTranscript(from: state)

        XCTAssertEqual(runtime.transcript.last?.markdown, "Waiting for response")
    }

    @MainActor
    func testTranscriptWithActiveAssistantStripsPureRouterPayloadMessage() {
        let runtime = ChatRuntime()
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-router",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: #"{"appendToolBundles":["analyst-baseline"],"suggestedProfileId":"diagnostic_baseline","scope":{"values":{"entityType":"Entity"}}}"#,
                    narration: nil,
                    status: "completed"
                ),
                BufferedStreamMessage(
                    id: "msg-human",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "I’m pulling the baseline delivery signals first.",
                    narration: nil,
                    status: "completed"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        let displayTranscript = runtime.transcriptWithActiveAssistant(snapshot: snapshot)

        XCTAssertTrue(runtime.transcript.isEmpty)
        XCTAssertEqual(displayTranscript.count, 1)
        XCTAssertEqual(displayTranscript[0].markdown, "I’m pulling the baseline delivery signals first.")
    }

    @MainActor
    func testTranscriptWithActiveAssistantIgnoresBufferedHistoryWithoutActiveTurn() {
        let runtime = ChatRuntime()
        runtime.appendAssistantMessage("existing history")
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: nil,
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "msg-history",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    content: "hydrated history",
                    narration: nil,
                    status: "completed"
                )
            ],
            liveExecutionGroupsByID: [:]
        )

        let displayTranscript = runtime.transcriptWithActiveAssistant(snapshot: snapshot)

        XCTAssertEqual(runtime.transcript.count, 1)
        XCTAssertEqual(runtime.transcript[0].markdown, "existing history")
        XCTAssertEqual(displayTranscript, runtime.transcript)
    }

    @MainActor
    func testTranscriptWithActiveAssistantPreservesReportOnlyStreamingResponse() throws {
        let runtime = ChatRuntime()
        let rendered = try JSONDecoder().decode(RenderedContent.self, from: Data(#"""
        {
          "schemaVersion": "1",
          "parts": [],
          "reports": [{
            "scope": "order-1",
            "id": "delivery",
            "grammar": "report-document-v1",
            "status": "committed",
            "resetVersion": 0,
            "source": {
              "title": "Delivery",
              "blocks": [{"id":"note","kind":"markdownBlock","markdown":"Ready"}]
            },
            "dataSources": {}
          }],
          "diagnostics": []
        }
        """#.utf8))
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            feeds: [],
            pendingElicitation: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "assistant-1",
                    conversationID: "conv-1",
                    turnID: "turn-1",
                    status: "running"
                )
            ],
            liveExecutionGroupsByID: [
                "assistant-1": LiveExecutionGroup(
                    pageID: "page-1",
                    assistantMessageID: "assistant-1",
                    turnID: "turn-1",
                    renderedContent: rendered
                )
            ]
        )

        let displayTranscript = runtime.transcriptWithActiveAssistant(snapshot: snapshot)

        XCTAssertEqual(displayTranscript.count, 1)
        XCTAssertEqual(displayTranscript[0].markdown, "")
        XCTAssertEqual(displayTranscript[0].renderedReports?.first?.id, "delivery")
    }

    @MainActor
    func testTranscriptWithActiveAssistantComposesProseReportAndLaterProse() throws {
        let runtime = ChatRuntime()
        let rendered = try JSONDecoder().decode(RenderedContent.self, from: Data(#"""
        {
          "schemaVersion": "1",
          "parts": [],
          "reports": [{
            "scope": "order-1",
            "id": "delivery",
            "grammar": "report-document-v1",
            "status": "building",
            "resetVersion": 0,
            "source": {"title":"Delivery","blocks":[]},
            "dataSources": {}
          }],
          "diagnostics": []
        }
        """#.utf8))
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: "turn-1",
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "assistant-prose-1",
                    turnID: "turn-1",
                    content: "### Key findings\n\nPrimary evidence."
                ),
                BufferedStreamMessage(id: "assistant-report", turnID: "turn-1"),
                BufferedStreamMessage(
                    id: "assistant-prose-2",
                    turnID: "turn-1",
                    content: "Follow-up interpretation."
                )
            ],
            liveExecutionGroupsByID: [
                "assistant-report": LiveExecutionGroup(
                    pageID: "page-1",
                    assistantMessageID: "assistant-report",
                    turnID: "turn-1",
                    renderedContent: rendered
                )
            ]
        )

        let entries = runtime.transcriptWithActiveAssistant(snapshot: snapshot)
        XCTAssertEqual(entries.count, 1)
        let entry = try XCTUnwrap(entries.first)

        XCTAssertEqual(entry.id, "assistant-prose-2")
        XCTAssertEqual(entry.turnID, "turn-1")
        XCTAssertEqual(entry.renderedParts?.compactMap(\.text), [
            "### Key findings\n\nPrimary evidence.",
            "\n\nFollow-up interpretation."
        ])
        XCTAssertEqual(entry.renderedReports?.map(\.id), ["delivery"])
    }

    @MainActor
    func testCommitAssistantTurnKeepsSSEContentAtCompletion() throws {
        let runtime = ChatRuntime()
        runtime.transcript = [
            ChatTranscriptEntry(id: "user-1", role: "user", markdown: "diagnose", turnID: "turn-1"),
            ChatTranscriptEntry(
                id: "assistant-pending",
                role: "assistant",
                markdown: "Working…",
                turnID: "turn-1",
                statusLabel: "Streaming"
            )
        ]
        let snapshot = ConversationStreamSnapshot(
            conversationID: "conv-1",
            activeTurnID: nil,
            bufferedMessages: [
                BufferedStreamMessage(
                    id: "assistant-final",
                    turnID: "turn-1",
                    content: "### Key findings\n\nThe report is ready.",
                    status: "completed",
                    interim: 0
                )
            ]
        )

        XCTAssertTrue(runtime.commitAssistantTurn(from: snapshot, turnID: "turn-1"))
        XCTAssertEqual(runtime.transcript.map(\.id), ["user-1", "assistant-final"])
        XCTAssertEqual(runtime.transcript.last?.markdown, "### Key findings\n\nThe report is ready.")
        XCTAssertNil(runtime.transcript.last?.statusLabel)
    }

    func testProgressStatusAttributedTextRemovesHeadingAndBoldMarkers() {
        let rendered = progressStatusAttributedText("""
        ### Key findings

        - **Primary blocker:** bid competitiveness
        """)

        XCTAssertEqual(String(rendered.characters), "Key findings\n• Primary blocker: bid competitiveness")
    }

    func testSanitizeVisibleAssistantTextStripsPureRouterPayload() {
        let raw = #"{"appendToolBundles":["analyst-baseline"],"suggestedProfileId":"diagnostic_baseline","scope":{"values":{"entityType":"Entity"}}}"#
        XCTAssertNil(sanitizeVisibleAssistantText(raw))
    }

    func testSanitizeVisibleAssistantTextPreservesHumanPrefixAndStripsRouterPayloadSuffix() {
        let raw = """
        I can troubleshoot the order, but I need the exact entity ID because “order” by itself is ambiguous. {
          "classification": { "intent": "troubleshoot_delivery_blocker" },
          "prompting": { "suggestedProfileId": "diagnostic_baseline" }
        }
        """
        XCTAssertEqual(
            sanitizeVisibleAssistantText(raw),
            "I can troubleshoot the order, but I need the exact entity ID because “order” by itself is ambiguous."
        )
    }

    func testParseTranscriptContentPartsExtractsForgeUIBlocks() {
        let content = [
            "Intro text",
            "```forge-data",
            #"{"version":1,"id":"summary_metrics","data":[{"spend":1316.86,"pacing_ratio":0.17,"win_rate":4.02}]}"#,
            "```",
            "```forge-ui",
            #"{"version":1,"title":"Entity 2639076","subtitle":"Group 4257","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["spend","pacing_ratio","win_rate"]}]}"#,
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
            XCTAssertEqual(payload.title, "Entity 2639076")
            XCTAssertEqual(dataStore.keys.sorted(), ["summary_metrics"])
        } else {
            XCTFail("Expected forge-ui part")
        }
    }

    func testParseTranscriptContentPartsLeavesMalformedLegacyMarkerPlusJSONFenceAsMarkdown() {
        let content = [
            "forge-data",
            "```json",
            #"{"version":1,"id":"summary_metrics","format":"json","mode":"replace","data":[{"spend":42}]}"#,
            "```",
            "forge-ui",
            "```json",
            #"{"version":1,"title":"Legacy dashboard","blocks":[{"id":"summary","kind":"dashboard.summary","dataSourceRef":"summary_metrics","metrics":["spend"]}]}"#,
            "```"
        ].joined(separator: "\n")

        let parts = parseTranscriptContentParts(content)

        XCTAssertEqual(parts.count, 1)
        if case .markdown(let text) = parts[0] {
            XCTAssertTrue(text.contains("forge-data"))
            XCTAssertTrue(text.contains("```json"))
        } else {
            XCTFail("Expected malformed legacy block to remain markdown")
        }
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsDashboardSummaryBlock() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "Entity 2639076",
            subtitle: "Group 4257",
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
        XCTAssertEqual(root.title, "Entity 2639076")
        XCTAssertEqual(summary.kind, "dashboard.summary")
        XCTAssertEqual(summary.dataSourceRef, "summary_metrics")
        XCTAssertEqual(summary.metrics.map(\.id), ["spend", "pacing_ratio", "win_rate"])
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsDashboardTimelineBlock() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "iOS dashboard verification",
            subtitle: "Group 4257",
            blocks: [
                .object([
                    "id": .string("spend-trend"),
                    "kind": .string("dashboard.timeline"),
                    "title": .string("Spend trend"),
                    "dataSourceRef": .string("metrics_data"),
                    "chartType": .string("bar"),
                    "dateField": .string("channel"),
                    "series": .array([
                        .object([
                            "key": .string("spend"),
                            "label": .string("Spend")
                        ])
                    ])
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

    func testBuildTranscriptForgeWindowMetadataAdaptsDashboardTableBlock() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "iOS dashboard verification",
            subtitle: "Group 4257",
            blocks: [
                .object([
                    "id": .string("primary-evidence"),
                    "kind": .string("dashboard.table"),
                    "title": .string("Primary evidence"),
                    "dataSourceRef": .string("summary_metrics"),
                    "columns": .array([
                        .object([
                            "key": .string("entity_name"),
                            "label": .string("Entity"),
                            "format": .string("currency"),
                            "type": .string("link"),
                            "link": .object([
                                "href": .string("ad_order_url")
                            ])
                        ]),
                        .object([
                            "key": .string("primary_blocker_family"),
                            "label": .string("Primary blocker"),
                            "format": .string("percentFraction")
                        ])
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
                            "entity_name": .string("CID-30432_DH_Retargeting"),
                            "primary_blocker_family": .string("Supply restriction")
                        ])
                    ])
                )
            ]
        )

        let root = try XCTUnwrap(metadata.view?.content?.containers.first)
        let table = try XCTUnwrap(root.containers.first)
        XCTAssertEqual(table.kind, "table")
        XCTAssertEqual(table.dataSourceRef, "summary_metrics")
        XCTAssertEqual(table.table?.columns.map(\.id), ["entity_name", "primary_blocker_family"])
        XCTAssertEqual(table.table?.columns.map(\.label), ["Entity", "Primary blocker"])
        XCTAssertEqual(table.table?.columns.map(\.format), ["currency", "percentFraction"])
        XCTAssertEqual(table.table?.columns.first?.type, "link")
        XCTAssertEqual(table.table?.columns.first?.link?.href, "ad_order_url")
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsSummaryItemsAndReportBlocks() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "Policy review review",
            subtitle: "Blocked before execution",
            blocks: [
                .object([
                    "kind": .string("dashboard.summary"),
                    "title": .string("Review summary"),
                    "items": .array([
                        .object([
                            "label": .string("Entity"),
                            "value": .string("Houston (Galleria) - Display (2657754)")
                        ]),
                        .object([
                            "label": .string("Submission status"),
                            "value": .string("Blocked")
                        ])
                    ])
                ]),
                .object([
                    "kind": .string("dashboard.report"),
                    "title": .string("Why this report item is not yet safe"),
                    "sections": .array([
                        .object([
                            "id": .string("interpretation"),
                            "title": .string("Interpretation"),
                            "body": .array([
                                .string("Block submission until current cap truth is confirmed.")
                            ])
                        ])
                    ])
                ]),
                .object([
                    "kind": .string("dashboard.kpiTable"),
                    "title": .string("Recent delivery posture"),
                    "dataSourceRef": .string("recent_delivery"),
                    "columns": .array([
                        .object([
                            "key": .string("total_spend"),
                            "label": .string("Spend")
                        ]),
                        .object([
                            "key": .string("flight_pacing_status"),
                            "label": .string("Flight pacing")
                        ])
                    ])
                ])
            ]
        )

        let metadata = try buildTranscriptForgeWindowMetadata(
            payload: payload,
            dataStore: [
                "recent_delivery": MaterializedForgeDataBlock(
                    id: "recent_delivery",
                    rows: .array([
                        .object([
                            "total_spend": .number(6061.727),
                            "flight_pacing_status": .string("behind")
                        ])
                    ])
                )
            ]
        )

        let containers = try XCTUnwrap(metadata.view?.content?.containers.first?.containers)
        XCTAssertEqual(containers[0].kind, "dashboard.summary")
        XCTAssertEqual(containers[0].metrics.map(\.label), ["Entity", "Submission status"])
        XCTAssertEqual(containers[1].kind, "dashboard.report")
        XCTAssertEqual(containers[1].sections.first?.title, "Interpretation")
        XCTAssertEqual(containers[2].table?.columns.map(\.id), ["total_spend", "flight_pacing_status"])
        XCTAssertEqual(containers[2].table?.columns.map(\.label), ["Spend", "Flight pacing"])
    }

    func testBuildTranscriptForgeWindowMetadataAdaptsDimensionsAndMessagesBlocks() throws {
        let payload = ForgeUIPayload(
            version: 1,
            title: "iOS dashboard verification",
            subtitle: "Group 4257",
            blocks: [
                .object([
                    "kind": .string("dashboard.dimensions"),
                    "title": .string("Publisher concentration"),
                    "dataSourceRef": .string("publisher_breakdown"),
                    "dimension": .object([
                        "key": .string("publisher_id"),
                        "label": .string("Publisher")
                    ]),
                    "metric": .object([
                        "key": .string("spend_share"),
                        "label": .string("Spend share"),
                        "format": .string("percentFraction")
                    ]),
                    "viewModes": .array([.string("chart"), .string("table")]),
                    "limit": .number(10)
                ]),
                .object([
                    "kind": .string("dashboard.messages"),
                    "title": .string("Next action"),
                    "items": .array([
                        .object([
                            "title": .string("Primary next step"),
                            "body": .string("Validate supply restriction next."),
                            "severity": .string("warning")
                        ])
                    ])
                ])
            ]
        )

        let metadata = try buildTranscriptForgeWindowMetadata(payload: payload, dataStore: [:])
        let containers = try XCTUnwrap(metadata.view?.content?.containers.first?.containers)

        XCTAssertEqual(containers[0].kind, "dashboard.dimensions")
        XCTAssertEqual(containers[0].dimension?.key, "publisher_id")
        XCTAssertEqual(containers[0].metric?.key, "spend_share")
        XCTAssertEqual(containers[0].viewModes, ["chart", "table"])
        XCTAssertEqual(containers[0].limit, 10)

        XCTAssertEqual(containers[1].kind, "dashboard.messages")
        XCTAssertEqual(containers[1].items.first?.title, "Primary next step")
        XCTAssertEqual(containers[1].items.first?.severity, "warning")
    }

    func testDeriveHostedWorkspaceRestoreStateRestoresHostedWindowFromViewOpen() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-1",
                          "toolName": "ui/view:open",
                          "status": "completed",
                          "requestPayload": {
                            "id": "reportWindow"
                          },
                          "responsePayload": {
                            "windowId": "reportWindow__conv-1",
                            "conversationId": "conv-1",
                            "windowKey": "reportWindow",
                            "windowTitle": "Report Review",
                            "presentation": "hosted",
                            "region": "chat.top",
                            "parentKey": "chat/new"
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )

        let restore = deriveHostedWorkspaceRestoreState(from: response)

        XCTAssertEqual(restore?.selectedWindowId, "reportWindow__conv-1")
        XCTAssertEqual(restore?.windows.first?.windowKey, "reportWindow")
    }

    func testDeriveHostedWorkspaceRestoreStatePreservesWorkspaceFromEarlierTurn() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-1",
                          "toolName": "ui/window/list",
                          "status": "completed",
                          "responsePayload": {
                            "items": [
                              {
                                "windowId": "order_legacy",
                                "conversationId": "conv-1",
                                "windowKey": "order",
                                "windowTitle": "Order Summary",
                                "presentation": "hosted",
                                "region": "chat.top",
                                "parentKey": "chat/new",
                                "inTab": true,
                                "parameters": {
                                  "EntityId": [111]
                                }
                              }
                            ]
                          }
                        }
                      ]
                    }
                  ]
                }
              },
              {
                "turnId": "turn-2",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-2",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-2",
                          "toolName": "message/reply",
                          "status": "completed",
                          "responsePayload": {
                            "ok": true
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )

        let restore = deriveHostedWorkspaceRestoreState(from: response)
        XCTAssertEqual(restore?.selectedWindowId, "order_legacy")
        XCTAssertEqual(restore?.windows.first?.windowKey, "order")
    }

    func testLatestTurnHostedWorkspaceRestoreStateIgnoresStoredStateWhenLatestTurnHasNoHostedWindow() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-1",
                          "toolName": "message/reply",
                          "status": "completed",
                          "responsePayload": {
                            "ok": true
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )
        let stored = HostedWorkspaceRestoreState(
            windows: [
                WorkspaceWindowSnapshot(
                    windowId: "order_legacy",
                    conversationId: "conv-1",
                    windowKey: "order",
                    windowTitle: "Order Summary",
                    presentation: "hosted",
                    region: "chat.top",
                    parentKey: "chat/new"
                )
            ],
            selectedWindowId: "order_legacy"
        )

        let restore = AppRuntime.latestTurnHostedWorkspaceRestoreState(
            transcriptState: response,
            stored: stored
        )

        XCTAssertNil(restore)
    }

    func testLatestTurnHostedWorkspaceRestoreStateFiltersGenericWindowsOutsideHostedPlacement() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-1",
                          "toolName": "ui/view/open",
                          "status": "completed",
                          "responsePayload": {
                            "windowId": "generic__conv-1",
                            "windowKey": "generic-report",
                            "windowTitle": "Generic Report"
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )

        let restore = AppRuntime.latestTurnHostedWorkspaceRestoreState(
            transcriptState: response,
            stored: nil
        )

        XCTAssertNil(restore)
    }

    func testLatestTurnHostedWorkspaceRestoreStateMergesStoredWindowFormWithTranscriptPrefill() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-open",
                          "toolName": "ui/view/open",
                          "status": "completed",
                          "responsePayload": {
                            "conversationId": "conv-1",
                            "items": [
                              {
                                "conversationId": "conv-1",
                                "parentKey": "chat/new",
                                "presentation": "hosted",
                                "region": "chat.top",
                                "windowId": "reportBuilder__conv-1",
                                "windowKey": "reportBuilder",
                                "windowTitle": "Report Builder"
                              }
                            ],
                            "selectedWindowId": "reportBuilder__conv-1"
                          }
                        },
                        {
                          "toolCallId": "tool-form",
                          "toolName": "ui/window/setFormData",
                          "status": "completed",
                          "requestPayload": {
                            "windowId": "reportBuilder__conv-1",
                            "values": {
                              "prefill": {
                                "country": ["US"],
                                "recordIds": [123]
                              },
                              "reportDefinition": {
                                "id": "delivery"
                              }
                            }
                          },
                          "responsePayload": {
                            "ok": true
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )
        let stored = HostedWorkspaceRestoreState(
            windows: [
                WorkspaceWindowSnapshot(
                    windowId: "reportBuilder__conv-1",
                    conversationId: "conv-1",
                    windowKey: "reportBuilder",
                    windowTitle: "Report Builder",
                    presentation: "hosted",
                    region: "chat.top",
                    parentKey: "chat/new",
                    windowForm: [
                        "reportBuilder": .object([
                            "viewMode": .string("table")
                        ]),
                        "reportBuilder:metricsCubeBuilder": .object([
                            "reportDocumentTitle": .string("Performance Delivery")
                        ]),
                        "reportMaterialization": .object([
                            "status": .string("running"),
                            "requestId": .string("orphaned-run")
                        ])
                    ]
                )
            ],
            selectedWindowId: "reportBuilder__conv-1"
        )

        let restore = AppRuntime.latestTurnHostedWorkspaceRestoreState(
            transcriptState: response,
            stored: stored
        )

        let windowForm = try XCTUnwrap(restore?.windows.first?.windowForm)
        XCTAssertEqual(windowForm["reportBuilder"]?.objectValue?["viewMode"], .string("table"))
        XCTAssertNil(windowForm["reportBuilder:metricsCubeBuilder"])
        XCTAssertNil(windowForm["reportMaterialization"])
        XCTAssertEqual(windowForm["reportDefinition"]?.objectValue?["id"], .string("delivery"))
        guard case .object(let prefill)? = windowForm["prefill"] else {
            XCTFail("Expected transcript prefill to survive stored window form merge")
            return
        }
        XCTAssertEqual(prefill["country"], .array([.string("US")]))
        XCTAssertEqual(prefill["recordIds"], .array([.number(123)]))
    }

    func testDeriveHostedWorkspaceRestoreStateUsesToolContentWhenViewOpenPayloadIsGzipEnvelope() throws {
        let json = """
        {
          "conversation": {
            "conversationId": "conv-1",
            "turns": [
              {
                "turnId": "turn-1",
                "execution": {
                  "pages": [
                    {
                      "pageId": "page-1",
                      "toolSteps": [
                        {
                          "toolCallId": "tool-1",
                          "toolName": "ui/view/open",
                          "status": "completed",
                          "content": "{\\"conversationId\\":\\"conv-1\\",\\"items\\":[{\\"conversationId\\":\\"conv-1\\",\\"parameters\\":{\\"EntityId\\":[2673453]},\\"parentKey\\":\\"chat/new\\",\\"presentation\\":\\"hosted\\",\\"region\\":\\"chat.top\\",\\"windowId\\":\\"order_2345888602__conv-1\\",\\"windowKey\\":\\"order\\",\\"windowTitle\\":\\"Order Summary\\",\\"workspaceSharePct\\":72,\\"workspaceMinHeight\\":500}],\\"ok\\":true,\\"parentKey\\":\\"chat/new\\",\\"presentation\\":\\"hosted\\",\\"region\\":\\"chat.top\\",\\"selectedWindowId\\":\\"order_2345888602__conv-1\\",\\"windowId\\":\\"order_2345888602__conv-1\\",\\"windowKey\\":\\"order\\",\\"windowTitle\\":\\"Order Summary\\"}",
                          "requestPayload": {
                            "InlineBody": "{\\"id\\":\\"order\\",\\"parameters\\":{\\"EntityId\\":[2673453]}}",
                            "Compression": "none"
                          },
                          "responsePayload": {
                            "InlineBody": "\\u0001\\u0002garbled",
                            "Compression": "gzip"
                          }
                        }
                      ]
                    }
                  ]
                }
              }
            ]
          }
        }
        """
        let response = try JSONDecoder.agently().decode(
            ConversationStateResponse.self,
            from: XCTUnwrap(json.data(using: .utf8))
        )

        let restore = deriveHostedWorkspaceRestoreState(from: response)

        XCTAssertEqual(restore?.selectedWindowId, "order_2345888602__conv-1")
        XCTAssertEqual(restore?.windows.first?.windowKey, "order")
        XCTAssertEqual(restore?.windows.first?.parameters?["EntityId"], .array([.number(2673453)]))
        XCTAssertEqual(restore?.windows.first?.workspaceSharePct, 72)
        XCTAssertEqual(restore?.windows.first?.workspaceMinHeight, 500)
    }
}
