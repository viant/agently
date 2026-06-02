package com.viant.agently.android

import com.viant.agentlysdk.ConversationState
import com.viant.agentlysdk.ConversationStateResponse
import com.viant.agentlysdk.ExecutionPageState
import com.viant.agentlysdk.ToolStepState
import com.viant.agentlysdk.TurnState
import kotlinx.serialization.json.Json
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
}
