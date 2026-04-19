package com.viant.agently.android

import com.viant.agentlysdk.DownloadFileOutput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayOutputStream
import java.util.zip.GZIPOutputStream

class PayloadPreviewRuntimeTest {

    @Test
    fun `decodePreviewBytes inflates gzip payloads`() {
        val raw = """{"hello":"world"}""".toByteArray()
        val gzipped = gzip(raw)

        val decoded = decodePreviewBytes(gzipped)

        assertEquals(String(raw), String(decoded))
    }

    @Test
    fun `buildPayloadPreview exposes decoded text for gzip json payloads`() {
        val raw = """{"hello":"world"}""".toByteArray()
        val preview = buildPayloadPreview(
            payloadId = "payload-1",
            title = "llm.request",
            downloaded = DownloadFileOutput(
                name = "payload-1.json",
                contentType = "application/json",
                data = gzip(raw)
            )
        )

        assertEquals("payload-1", preview.artifactId)
        assertEquals("llm.request", preview.name)
        assertTrue(preview.text?.contains("hello") == true)
    }

    private fun gzip(data: ByteArray): ByteArray {
        val output = ByteArrayOutputStream()
        GZIPOutputStream(output).use { it.write(data) }
        return output.toByteArray()
    }
}
