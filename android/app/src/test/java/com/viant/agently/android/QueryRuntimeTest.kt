package com.viant.agently.android

import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.WindowMetadata
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.longOrNull
import java.util.concurrent.atomic.AtomicInteger

class QueryRuntimeTest {

    @Test
    fun `app session client allows long lived reads for query stream and ui bridge`() {
        val client = appSessionHttpClient()

        assertEquals(0, client.readTimeoutMillis)
        assertEquals(0, client.callTimeoutMillis)
    }

    @Test
    fun `post query bind keeps existing same conversation stream`() {
        assertTrue(
            shouldKeepConversationStream(
                activeConversationId = "conv-1",
                targetConversationId = "conv-1",
                replaceTranscript = false,
                hasStreamJob = true
            )
        )
        assertEquals(
            false,
            shouldKeepConversationStream(
                activeConversationId = "conv-1",
                targetConversationId = "conv-1",
                replaceTranscript = true,
                hasStreamJob = true
            )
        )
        assertEquals(
            false,
            shouldKeepConversationStream(
                activeConversationId = "conv-1",
                targetConversationId = "conv-2",
                replaceTranscript = false,
                hasStreamJob = true
            )
        )
    }

    @Test
    fun `post query bind preserves same conversation stream snapshot`() {
        val snapshot = ConversationStreamSnapshot(
            conversationId = "conv-1",
            activeTurnId = "turn-1",
            feeds = emptyList(),
            pendingElicitation = null,
            bufferedMessages = emptyList(),
            liveExecutionGroupsById = emptyMap()
        )

        assertTrue(
            shouldPreserveConversationStreamSnapshot(
                targetConversationId = "conv-1",
                replaceTranscript = false,
                streamSnapshot = snapshot
            )
        )
        assertEquals(
            false,
            shouldPreserveConversationStreamSnapshot(
                targetConversationId = "conv-1",
                replaceTranscript = true,
                streamSnapshot = snapshot
            )
        )
        assertEquals(
            false,
            shouldPreserveConversationStreamSnapshot(
                targetConversationId = "conv-2",
                replaceTranscript = false,
                streamSnapshot = snapshot
            )
        )
    }

    @Test
    fun `ui bridge window open returns without waiting for forge metadata`() {
        runBlocking {
            val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
            val runtime = ForgeRuntime(endpoints = emptyMap(), scope = scope)
            val metadata = CompletableDeferred<WindowMetadata?>()
            runtime.registerWindowMetadataLoader {
                metadata.await()
            }

            try {
                val response = withTimeout(1_000) {
                    handleAndroidUIBridgeCommand(
                        method = "ui.window.open",
                        params = buildJsonObject {
                            put("windowKey", JsonPrimitive("record-detail"))
                            put("windowTitle", JsonPrimitive("Record Detail"))
                            put("windowId", JsonPrimitive("record-detail-1"))
                            put(
                                "parameters",
                                buildJsonObject {
                                    put("recordId", JsonPrimitive(42L))
                                }
                            )
                            put(
                                "options",
                                buildJsonObject {
                                    put("conversationId", JsonPrimitive("conv-1"))
                                    put("presentation", JsonPrimitive("hosted"))
                                    put("region", JsonPrimitive("workspace"))
                                    put("parentKey", JsonPrimitive("parent-window"))
                                    put("workspaceSharePct", JsonPrimitive(45))
                                    put("workspaceMinHeight", JsonPrimitive(320))
                                }
                            )
                        },
                        forgeRuntime = runtime
                    )
                }

                assertEquals(true, (response["ok"] as? JsonPrimitive)?.booleanOrNull)
                assertEquals("record-detail-1", (response["selectedWindowId"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("record-detail-1", (response["windowId"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("record-detail", (response["windowKey"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("Record Detail", (response["windowTitle"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("conv-1", (response["conversationId"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("hosted", (response["presentation"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("workspace", (response["region"] as? JsonPrimitive)?.contentOrNull)
                assertEquals("parent-window", (response["parentKey"] as? JsonPrimitive)?.contentOrNull)
                val parameters = response["parameters"] as JsonObject
                assertEquals(42L, (parameters["recordId"] as? JsonPrimitive)?.longOrNull)
                assertNull(runtime.metadataSignal("record-detail-1").peek())
                metadata.complete(WindowMetadata())
                withTimeout(1_000) {
                    runtime.metadataSignal("record-detail-1").flow.first { it != null }
                }
            } finally {
                if (!metadata.isCompleted) {
                    metadata.complete(null)
                }
                scope.cancel()
            }
        }
    }

    @Test
    fun `ui bridge data fetch returns before metadata and refreshes after load`() {
        runBlocking {
            val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
            val runtime = ForgeRuntime(endpoints = emptyMap(), scope = scope)
            val metadata = CompletableDeferred<WindowMetadata?>()
            val fetched = CompletableDeferred<ForgeRuntime.DataSourceFetchRequest>()
            val fetchAttempts = AtomicInteger(0)
            runtime.registerWindowMetadataLoader {
                metadata.await()
            }
            runtime.registerDataSourceLoader { request ->
                fetchAttempts.incrementAndGet()
                fetched.complete(request)
                ForgeRuntime.DataSourceFetchResult()
            }

            try {
                handleAndroidUIBridgeCommand(
                    method = "ui.window.open",
                    params = buildJsonObject {
                        put("windowKey", JsonPrimitive("record-detail"))
                        put("windowTitle", JsonPrimitive("Record Detail"))
                        put("windowId", JsonPrimitive("record-detail-2"))
                    },
                    forgeRuntime = runtime
                )

                val response = withTimeout(1_000) {
                    handleAndroidUIBridgeCommand(
                        method = "ui.data.fetch",
                        params = buildJsonObject {
                            put("windowId", JsonPrimitive("record-detail-2"))
                        },
                        forgeRuntime = runtime
                    )
                }

                assertEquals(true, (response["ok"] as? JsonPrimitive)?.booleanOrNull)
                assertEquals(0, fetchAttempts.get())
                metadata.complete(
                    WindowMetadata(
                        dataSources = mapOf("detail" to DataSourceDef())
                    )
                )
                val request = withTimeout(1_000) {
                    fetched.await()
                }
                assertEquals("record-detail-2", request.windowId)
                assertEquals("detail", request.dataSourceRef)
            } finally {
                if (!metadata.isCompleted) {
                    metadata.complete(null)
                }
                scope.cancel()
            }
        }
    }

    @Test
    fun `ui bridge set form data patches generic window form`() {
        runBlocking {
            val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
            val runtime = ForgeRuntime(endpoints = emptyMap(), scope = scope)

            try {
                val state = runtime.openWindowInline(
                    windowKey = "report-builder",
                    title = "Report Builder",
                    metadata = WindowMetadata()
                )
                runtime.setWindowFormValues(
                    state.windowId,
                    mapOf(
                        "prefill" to mapOf(
                            "accountId" to 7
                        )
                    ),
                    replace = true
                )

                val response = handleAndroidUIBridgeCommand(
                    method = "ui.window.setFormData",
                    params = buildJsonObject {
                        put("windowId", JsonPrimitive(state.windowId))
                        put(
                            "values",
                            buildJsonObject {
                                put(
                                    "prefill",
                                    buildJsonObject {
                                        put("segmentId", JsonPrimitive(9))
                                    }
                                )
                            }
                        )
                    },
                    forgeRuntime = runtime
                )

                assertEquals(true, (response["ok"] as? JsonPrimitive)?.booleanOrNull)
                val responseWindowForm = response["windowForm"] as JsonObject
                val responsePrefill = responseWindowForm["prefill"] as JsonObject
                assertEquals(7L, (responsePrefill["accountId"] as? JsonPrimitive)?.longOrNull)
                assertEquals(9L, (responsePrefill["segmentId"] as? JsonPrimitive)?.longOrNull)
                val runtimePrefill = runtime.windowContext(state.windowId).peekWindowForm()["prefill"] as Map<*, *>
                assertEquals(7, runtimePrefill["accountId"])
                assertEquals(9L, runtimePrefill["segmentId"])
            } finally {
                scope.cancel()
            }
        }
    }

    @Test
    fun `resolveEffectivePrompt uses attachment fallback when prompt is blank`() {
        val prompt = resolveEffectivePrompt(
            prompt = "   ",
            attachments = listOf(
                ComposerAttachmentDraft(
                    name = "screen.png",
                    mimeType = "image/png",
                    bytes = ByteArray(8),
                    source = "Photo"
                )
            )
        )

        assertEquals("Please analyze the attached file(s).", prompt)
    }

    @Test
    fun `buildPendingUserChatEntry marks transcript entry as sending`() {
        val entry = buildPendingUserChatEntry(
            entryId = "user-1",
            prompt = "Review this image",
            attachments = emptyList(),
            timestampMs = 1_700_000_000_000
        )

        assertEquals("user", entry.role)
        assertEquals("sending", entry.deliveryState)
        assertTrue(entry.markdown.contains("Review this image"))
    }

    @Test
    fun `prepareQuerySubmission builds pending entry with resolved prompt`() {
        val prepared = prepareQuerySubmission(
            draft = ComposerDraftState(
                prompt = "   ",
                attachments = listOf(
                    ComposerAttachmentDraft(
                        name = "screen.png",
                        mimeType = "image/png",
                        bytes = ByteArray(4),
                        source = "Photo"
                    )
                )
            ),
            timestampMs = 1234L
        )

        assertEquals("Please analyze the attached file(s).", prepared.effectivePrompt)
        assertEquals("user-1234", prepared.entryId)
        assertEquals("sending", prepared.pendingEntry.deliveryState)
        assertTrue(prepared.pendingEntry.markdown.contains("Please analyze the attached file(s)."))
    }

    @Test
    fun `clearComposerDraft returns empty prompt and attachments`() {
        val cleared = clearComposerDraft()

        assertEquals("", cleared.prompt)
        assertTrue(cleared.attachments.isEmpty())
    }

    @Test
    fun `shouldRestoreComposerDraft only restores when composer is still empty`() {
        assertTrue(shouldRestoreComposerDraft("", emptyList()))
        assertEquals(false, shouldRestoreComposerDraft("typed", emptyList()))
        assertEquals(
            false,
            shouldRestoreComposerDraft(
                "",
                listOf(
                    ComposerAttachmentDraft(
                        name = "note.txt",
                        mimeType = "text/plain",
                        bytes = byteArrayOf(1),
                        source = "File"
                    )
                )
            )
        )
    }

    @Test
    fun `buildArtifactPreview keeps text for previewable files`() {
        val preview = buildArtifactPreview(
            file = GeneratedFileEntry(
                id = "artifact-1",
                filename = "report.md",
                mimeType = "text/markdown"
            ),
            downloaded = DownloadFileOutput(
                name = "report.md",
                contentType = "text/markdown",
                data = "# Report".toByteArray()
            )
        )

        assertEquals("report.md", preview.name)
        assertEquals("# Report", preview.text)
    }

    @Test
    fun `buildArtifactPreview leaves binary files without inline text`() {
        val preview = buildArtifactPreview(
            file = GeneratedFileEntry(
                id = "artifact-2",
                filename = "image.png",
                mimeType = "image/png"
            ),
            downloaded = DownloadFileOutput(
                name = "image.png",
                contentType = "image/png",
                data = byteArrayOf(1, 2, 3)
            )
        )

        assertNull(preview.text)
        assertEquals(3, preview.sizeBytes)
    }

    @Test
    fun `buildQuerySuccessState prefers query output conversation id and trims approval edits`() {
        val state = buildQuerySuccessState(
            execution = QueryExecutionResult(
                metadata = WorkspaceMetadata(workspaceRoot = "/workspace"),
                conversationId = "conversation-created",
                queryOutput = QueryOutput(
                    conversationId = "conversation-result",
                    content = "final reply",
                    messageId = "message-1"
                ),
                generatedFiles = listOf(GeneratedFileEntry(id = "artifact-1")),
                pendingApprovals = listOf(
                    PendingToolApproval(
                        id = "approval-1",
                        toolName = "system/test",
                        title = "Approve",
                        status = "pending"
                    )
                )
            ),
            approvalEdits = mapOf(
                "approval-1" to mapOf("enabled" to JsonPrimitive(true)),
                "approval-stale" to mapOf("enabled" to JsonPrimitive(false))
            )
        )

        assertEquals("/workspace", state.metadata.workspaceRoot)
        assertEquals("conversation-result", state.activeConversationId)
        assertEquals("final reply", state.streamedMarkdown)
        assertEquals(listOf("approval-1"), state.approvalEdits.keys.toList())
        assertEquals(listOf("artifact-1"), state.generatedFiles.map { it.id })
    }
}
