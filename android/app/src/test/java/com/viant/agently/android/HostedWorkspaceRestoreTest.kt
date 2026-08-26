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
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
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
    fun `deriveHostedWorkspaceRestoreState seeds parameters into window form`() {
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
                                            toolName = "ui/view:open",
                                            status = "completed",
                                            requestPayload = json.parseToJsonElement(
                                                """
                                                {
                                                  "id": "reportBuilder",
                                                  "parameters": {
                                                    "reportBuilderRef": "capacityBuilder",
                                                    "prefill": {
                                                      "recordIds": [12345],
                                                      "scope": {
                                                        "targetKey": "record:12345",
                                                        "source": "parameter"
                                                      }
                                                    }
                                                  }
                                                }
                                                """.trimIndent()
                                            ),
                                            responsePayload = json.parseToJsonElement(
                                                """
                                                {
                                                  "windowId": "reportBuilder__conv-1",
                                                  "conversationId": "conv-1",
                                                  "windowKey": "reportBuilder",
                                                  "windowTitle": "Capacity Builder",
                                                  "presentation": "hosted",
                                                  "region": "chat.top",
                                                  "parentKey": "chat/new",
                                                  "windowForm": {
                                                    "prefill": {
                                                      "scope": {
                                                        "source": "windowForm"
                                                      }
                                                    }
                                                  }
                                                }
                                                """.trimIndent()
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

        val windowForm = deriveAgentlyHostedWorkspaceRestoreState(state)
            ?.windows
            ?.singleOrNull()
            ?.windowForm

        assertNotNull(windowForm)
        assertEquals(JsonPrimitive("capacityBuilder"), windowForm?.get("reportBuilderRef"))
        val prefill = windowForm?.get("prefill") as? JsonObject
        assertEquals(JsonArray(listOf(JsonPrimitive(12345))), prefill?.get("recordIds"))
        val scope = prefill?.get("scope") as? JsonObject
        assertEquals(JsonPrimitive("record:12345"), scope?.get("targetKey"))
        assertEquals(JsonPrimitive("windowForm"), scope?.get("source"))
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
    fun `deriveHostedWorkspaceRestoreState preserves durable workspace during live stream gap`() {
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

        assertEquals("record_legacy", restore?.selectedWindowId)
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
    fun `deriveHostedWorkspaceRestoreState preserves workspace opened by an earlier turn`() {
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

        assertEquals("record_legacy", restore?.selectedWindowId)
        assertEquals(1, restore?.windows?.size)
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

    @Test
    fun `local bridge window is visible before transcript contains ui open step`() {
        val localSnapshot = NativeUIBridgeSnapshot(
            conversationId = "conv-1",
            windows = listOf(
                NativeUIBridgeWindow(
                    windowId = "chat/new",
                    windowKey = "chat/new",
                    windowTitle = "Chat",
                    conversationId = "conv-1"
                ),
                NativeUIBridgeWindow(
                    windowId = "order__conv-1",
                    windowKey = "order",
                    windowTitle = "Order 2626512",
                    conversationId = "conv-1",
                    presentation = "hosted",
                    region = "chat.top",
                    parentKey = "chat/new",
                    parameters = JsonObject(mapOf("OrderId" to JsonPrimitive(2626512)))
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(
            state = null,
            streamSnapshot = null,
            localSnapshot = localSnapshot
        )

        assertNotNull(restore)
        assertEquals("order__conv-1", restore?.selectedWindowId)
        assertEquals("order", restore?.windows?.singleOrNull()?.windowKey)
    }

    @Test
    fun `local bridge window remains authoritative during transcript rehydrate`() {
        val localSnapshot = NativeUIBridgeSnapshot(
            conversationId = "conv-1",
            windows = listOf(
                NativeUIBridgeWindow(
                    windowId = "order__conv-1",
                    windowKey = "order",
                    windowTitle = "Order 2626512",
                    conversationId = "conv-1",
                    presentation = "hosted",
                    region = "chat.top",
                    parentKey = "chat/new"
                )
            )
        )
        val rehydratedStateWithoutWindow = ConversationStateResponse(
            conversation = ConversationState(conversationId = "conv-1")
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(
            rehydratedStateWithoutWindow,
            streamSnapshot = null,
            localSnapshot = localSnapshot
        )

        assertEquals("order__conv-1", restore?.selectedWindowId)
    }

    @Test
    fun `durable authored report survives stale local default window`() {
        val windowId = "report__conv-1"
        val state = ConversationStateResponse(
            conversation = ConversationState(
                conversationId = "conv-1",
                turns = listOf(TurnState(
                    turnId = "turn-1",
                    execution = ExecutionState(pages = listOf(ExecutionPageState(
                        pageId = "page-1",
                        toolSteps = listOf(
                            ToolStepState(
                                toolCallId = "open",
                                toolName = "ui/view/open",
                                status = "completed",
                                responsePayload = json.parseToJsonElement(
                                    """{"windowId":"$windowId","windowKey":"reportBuilder","windowTitle":"Report","conversationId":"conv-1","presentation":"hosted","region":"chat.top","parentKey":"chat/new"}"""
                                )
                            ),
                            ToolStepState(
                                toolCallId = "set",
                                toolName = "ui/window/setFormData",
                                status = "completed",
                                requestPayload = json.parseToJsonElement(
                                    """{"windowId":"$windowId","values":{"reportDefinition":{"id":"delivery","documentPatch":{"blocks":[{"id":"spend","datasetRef":"summary"}]}}}}"""
                                )
                            )
                        )
                    )))
                ))
            )
        )
        val local = NativeUIBridgeSnapshot(
            conversationId = "conv-1",
            windows = listOf(NativeUIBridgeWindow(
                windowId = windowId,
                windowKey = "reportBuilder",
                windowTitle = "Report",
                conversationId = "conv-1",
                presentation = "hosted",
                region = "chat.top",
                parentKey = "chat/new",
                windowForm = JsonObject(mapOf(
                    "reportBuilderRef" to JsonPrimitive("metricsCubeBuilder"),
                    "reportBuilder:metricsCubeBuilder" to JsonObject(mapOf(
                        "reportDocumentTitle" to JsonPrimitive("Performance Delivery"),
                                    "reportDocumentBlocks" to JsonArray(emptyList())
                    )),
                    "reportMaterialization" to JsonObject(mapOf(
                        "status" to JsonPrimitive("running"),
                        "requestId" to JsonPrimitive("orphaned-run")
                    )),
                    "reportRunRequest" to JsonObject(mapOf("id" to JsonPrimitive("orphaned-run")))
                ))
            ))
        )

        val restored = deriveAgentlyHostedWorkspaceRestoreState(state, null, local)
        val reportDefinition = restored?.windows?.singleOrNull()?.windowForm?.get("reportDefinition") as? JsonObject

        assertEquals("delivery", (reportDefinition?.get("id") as? JsonPrimitive)?.content)
        assertEquals("metricsCubeBuilder", (restored?.windows?.singleOrNull()?.windowForm?.get("reportBuilderRef") as? JsonPrimitive)?.content)
        assertNull(restored?.windows?.singleOrNull()?.windowForm?.get("reportBuilder:metricsCubeBuilder"))
        assertNull(restored?.windows?.singleOrNull()?.windowForm?.get("reportMaterialization"))
        assertNull(restored?.windows?.singleOrNull()?.windowForm?.get("reportRunRequest"))
    }

    @Test
    fun `local bridge snapshot does not expose window from another conversation`() {
        val localSnapshot = NativeUIBridgeSnapshot(
            conversationId = "conv-2",
            windows = listOf(
                NativeUIBridgeWindow(
                    windowId = "order__conv-1",
                    windowKey = "order",
                    windowTitle = "Order 2626512",
                    conversationId = "conv-1",
                    presentation = "hosted",
                    region = "chat.top",
                    parentKey = "chat/new"
                )
            )
        )

        val restore = deriveAgentlyHostedWorkspaceRestoreState(localSnapshot)

        assertNull(restore)
    }

    @Test
    fun `local bridge snapshot does not expose stale window without active conversation`() {
        val localSnapshot = NativeUIBridgeSnapshot(
            windows = listOf(
                NativeUIBridgeWindow(
                    windowId = "order__conv-1",
                    windowKey = "order",
                    windowTitle = "Order 2626512",
                    conversationId = "conv-1",
                    presentation = "hosted",
                    region = "chat.top",
                    parentKey = "chat/new"
                )
            )
        )

        assertNull(deriveAgentlyHostedWorkspaceRestoreState(localSnapshot))
    }
}
