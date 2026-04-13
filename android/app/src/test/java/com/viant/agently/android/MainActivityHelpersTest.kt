package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class MainActivityHelpersTest {

    @Test
    fun `normalizeAuthError explains upstream idp timeout`() {
        val actual = normalizeAuthError(
            """failed to post form data Post "https://idp.viantinc.com/v1/api/oauth2/authorize": dial tcp 10.55.159.93:443: i/o timeout"""
        )

        assertEquals(
            "failed to post form data Post \"https://idp.viantinc.com/v1/api/oauth2/authorize\": dial tcp 10.55.159.93:443: i/o timeout",
            actual
        )
    }

    @Test
    fun `normalizeAuthError hides benign composition cancellation`() {
        assertNull(normalizeAuthError("The coroutine scope left the composition"))
    }

    @Test
    fun `buildUserComposerMarkdown includes attachment details`() {
        val markdown = buildUserComposerMarkdown(
            prompt = "Please review this image.",
            attachments = listOf(
                ComposerAttachmentDraft(
                    name = "screen.png",
                    mimeType = "image/png",
                    bytes = ByteArray(2_048),
                    source = "Photo"
                )
            )
        )

        assertTrue(markdown.contains("Please review this image."))
        assertTrue(markdown.contains("Attached:"))
        assertTrue(markdown.contains("Photo: screen.png (image/png, 2.0 KB)"))
    }

    @Test
    fun `buildConversationTitle prefers attachment name for attachment only turns`() {
        val title = buildConversationTitle(
            prompt = "Please analyze the attached file(s).",
            attachments = listOf(
                ComposerAttachmentDraft(
                    name = "screen.png",
                    mimeType = "image/png",
                    bytes = ByteArray(2_048),
                    source = "Photo"
                )
            )
        )

        assertEquals("screen.png", title)
    }

    @Test
    fun `buildConversationTitle keeps meaningful prompt when present`() {
        val title = buildConversationTitle(
            prompt = "Review this deployment screenshot for auth issues",
            attachments = listOf(
                ComposerAttachmentDraft(
                    name = "screen.png",
                    mimeType = "image/png",
                    bytes = ByteArray(2_048),
                    source = "Photo"
                )
            )
        )

        assertEquals("Review this deployment screenshot for auth issues", title)
    }

    @Test
    fun `formatSizeLabel formats bytes and kibibytes`() {
        assertEquals("512 B", formatSizeLabel(512))
        assertEquals("2.0 KB", formatSizeLabel(2_048))
        assertEquals("1.5 MB", formatSizeLabel(1_572_864))
    }

}
