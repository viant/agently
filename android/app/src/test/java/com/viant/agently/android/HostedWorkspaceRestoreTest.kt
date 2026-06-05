package com.viant.agently.android

import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.ExecutionPageState
import com.viant.agentlysdk.ExecutionState
import com.viant.agentlysdk.ToolStepState
import com.viant.agentlysdk.TurnState
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.agentlysdk.stream.LiveExecutionGroup
import com.viant.agentlysdk.stream.LiveToolStepState
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class HostedWorkspaceRestoreTest {
    private val json = Json { ignoreUnknownKeys = true }

    @Test
    fun `deriveHostedWorkspaceRestoreState restores hosted window from ui view open tool step`() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        execution = com.viant.agentlysdk.ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-1",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-1",
                                            toolName = "ui/view:open",
                                            status = "completed",
                                            requestPayload = json.parseToJsonElement(
                                                """{"id":"reportWindow","parameters":{"entity_id":[7203973]}}"""
                                            ),
                                            responsePayload = json.parseToJsonElement(
                                                """{"windowId":"reportWindow__conv-1","conversationId":"conv-1","windowKey":"reportWindow","windowTitle":"Report Review","presentation":"hosted","region":"chat.top","parentKey":"chat/new"}"""
                                            )
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(state)

        assertNotNull(restore)
        assertEquals("reportWindow__conv-1", restore?.selectedWindowId)
        assertEquals("reportWindow", restore?.windows?.singleOrNull()?.windowKey)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState restores hosted window from live stream snapshot`() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-live",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = mapOf(
                "assistant-1" to LiveExecutionGroup(
                    pageId = "page-live",
                    assistantMessageId = "assistant-1",
                    turnId = "turn-live",
                    toolSteps = listOf(
                        LiveToolStepState(
                            toolCallId = "tool-1",
                            toolName = "ui/view/open",
                            status = "completed",
                            responsePayload = json.parseToJsonElement(
                                """{"windowId":"reportWindow__conv-1","conversationId":"conv-1","windowKey":"reportWindow","windowTitle":"Report Review","presentation":"hosted","region":"chat.top","parentKey":"chat/new"}"""
                            )
                        )
                    )
                )
            )
        )

        val restore = filterAgentlyHostedWorkspaceRestoreState(
            com.viant.agentlysdk.deriveHostedWorkspaceRestoreState(snapshot)
        )

        assertNotNull(restore)
        assertEquals("reportWindow__conv-1", restore?.selectedWindowId)
        assertEquals("reportWindow", restore?.windows?.singleOrNull()?.windowKey)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState does not fall back to stale state during live turn`() {
        val staleState = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-old",
                        execution = ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-old",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-old",
                                            toolName = "ui/window/list",
                                            status = "completed",
                                            responsePayload = json.parseToJsonElement(
                                                """{"items":[{"windowId":"record_legacy","conversationId":"conv-1","windowKey":"record","windowTitle":"Record Detail","presentation":"hosted","region":"chat.top","parentKey":"chat/new"}]}"""
                                            )
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )
        val liveSnapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-live",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = emptyMap()
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(staleState, liveSnapshot)

        assertNull(restore)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState ignores non hosted transcript turns`() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        execution = com.viant.agentlysdk.ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-1",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-1",
                                            toolName = "system/exec:start",
                                            status = "completed"
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(state)

        assertNull(restore)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState filters generic windows outside hosted chat placement`() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        execution = com.viant.agentlysdk.ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-1",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-1",
                                            toolName = "ui/view/open",
                                            status = "completed",
                                            responsePayload = json.parseToJsonElement(
                                                """{"windowId":"generic__conv-1","windowKey":"generic-report","windowTitle":"Generic Report"}"""
                                            )
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(state)

        assertNull(restore)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState restores from the last turn only`() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        execution = ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-1",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-1",
                                            toolName = "ui/window/list",
                                            status = "completed",
                                            responsePayload = json.parseToJsonElement(
                                                """{"items":[{"windowId":"record_legacy","conversationId":"conv-1","windowKey":"record","windowTitle":"Record Detail","presentation":"hosted","region":"chat.top","parentKey":"chat/new","inTab":true,"parameters":{"RecordId":[111]}}]}"""
                                            )
                                        )
                                    )
                                )
                            )
                        )
                    ),
                    TurnState(
                        turnId = "turn-2",
                        execution = ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-2",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-2",
                                            toolName = "message/reply",
                                            status = "completed",
                                            responsePayload = json.parseToJsonElement("""{"ok":true}""")
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(state)

        assertNull(restore)
    }

    @Test
    fun `deriveHostedWorkspaceRestoreState uses tool content when ui view open response payload is gzip envelope`() {
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(
                    TurnState(
                        turnId = "turn-1",
                        execution = ExecutionState(
                            pages = listOf(
                                ExecutionPageState(
                                    pageId = "page-1",
                                    toolSteps = listOf(
                                        ToolStepState(
                                            toolCallId = "tool-1",
                                            toolName = "ui/view/open",
                                            status = "completed",
                                            requestPayload = json.parseToJsonElement(
                                                """{"InlineBody":"{\"id\":\"record\",\"parameters\":{\"RecordId\":[2673453]}}","Compression":"none"}"""
                                            ),
                                            responsePayload = json.parseToJsonElement(
                                                """{"InlineBody":"\u0001\u0002garbled","Compression":"gzip"}"""
                                            ),
                                            content = """{"conversationId":"conv-1","items":[{"conversationId":"conv-1","parameters":{"RecordId":[2673453]},"parentKey":"chat/new","presentation":"hosted","region":"chat.top","windowId":"record_2345888602__conv-1","windowKey":"record","windowTitle":"Record Detail","workspaceSharePct":72,"workspaceMinHeight":500}],"ok":true,"parentKey":"chat/new","presentation":"hosted","region":"chat.top","selectedWindowId":"record_2345888602__conv-1","windowId":"record_2345888602__conv-1","windowKey":"record","windowTitle":"Record Detail"}"""
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(state)

        assertNotNull(restore)
        assertEquals("record_2345888602__conv-1", restore?.selectedWindowId)
        assertEquals("record", restore?.windows?.singleOrNull()?.windowKey)
        assertEquals("[2673453]", (restore?.windows?.singleOrNull()?.parameters?.get("RecordId") as? JsonArray)?.toString())
        assertEquals(72, restore?.windows?.singleOrNull()?.workspaceSharePct)
        assertEquals(500, restore?.windows?.singleOrNull()?.workspaceMinHeight)
    }
}
