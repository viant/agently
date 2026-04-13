package com.viant.agently.android

import com.viant.agentlysdk.DownloadFileOutput
import com.viant.agentlysdk.GeneratedFileEntry
import com.viant.agentlysdk.PendingToolApproval
import com.viant.agentlysdk.QueryOutput
import com.viant.agentlysdk.WorkspaceMetadata
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.serialization.json.JsonPrimitive

class QueryRuntimeTest {

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
