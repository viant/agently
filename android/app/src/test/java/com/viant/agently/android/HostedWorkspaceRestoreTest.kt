package com.viant.agently.android

import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.ExecutionPageState
import com.viant.agentlysdk.ExecutionState
import com.viant.agentlysdk.ToolStepState
import com.viant.agentlysdk.TurnState
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
                                                """{"id":"recommendationList","parameters":{"audience_id":[7203973]}}"""
                                            ),
                                            responsePayload = json.parseToJsonElement(
                                                """{"windowId":"recommendationList__conv-1","conversationId":"conv-1","windowKey":"recommendationList","windowTitle":"Recommendation Review","presentation":"hosted","region":"chat.top","parentKey":"chat/new"}"""
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

        val restore = deriveHostedWorkspaceRestoreState(state)

        assertNotNull(restore)
        assertEquals("recommendationList__conv-1", restore?.selectedWindowId)
        assertEquals("recommendationList", restore?.windows?.singleOrNull()?.windowKey)
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

        val restore = deriveHostedWorkspaceRestoreState(state)

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
                                                """{"items":[{"windowId":"order_legacy","conversationId":"conv-1","windowKey":"order","windowTitle":"Order Summary","presentation":"hosted","region":"chat.top","parentKey":"chat/new","inTab":true,"parameters":{"AdOrderId":[111]}}]}"""
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

        val restore = deriveHostedWorkspaceRestoreState(state)

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
                                                """{"InlineBody":"{\"id\":\"order\",\"parameters\":{\"AdOrderId\":[2673453]}}","Compression":"none"}"""
                                            ),
                                            responsePayload = json.parseToJsonElement(
                                                """{"InlineBody":"\u0001\u0002garbled","Compression":"gzip"}"""
                                            ),
                                            content = """{"conversationId":"conv-1","items":[{"conversationId":"conv-1","parameters":{"AdOrderId":[2673453]},"parentKey":"chat/new","presentation":"hosted","region":"chat.top","windowId":"order_2345888602__conv-1","windowKey":"order","windowTitle":"Order Summary","workspaceSharePct":72,"workspaceMinHeight":500}],"ok":true,"parentKey":"chat/new","presentation":"hosted","region":"chat.top","selectedWindowId":"order_2345888602__conv-1","windowId":"order_2345888602__conv-1","windowKey":"order","windowTitle":"Order Summary"}"""
                                        )
                                    )
                                )
                            )
                        )
                    )
                )
            )
        )

        val restore = deriveHostedWorkspaceRestoreState(state)

        assertNotNull(restore)
        assertEquals("order_2345888602__conv-1", restore?.selectedWindowId)
        assertEquals("order", restore?.windows?.singleOrNull()?.windowKey)
        assertEquals("[2673453]", (restore?.windows?.singleOrNull()?.parameters?.get("AdOrderId") as? JsonArray)?.toString())
        assertEquals(72, restore?.windows?.singleOrNull()?.workspaceSharePct)
        assertEquals(500, restore?.windows?.singleOrNull()?.workspaceMinHeight)
    }
}
