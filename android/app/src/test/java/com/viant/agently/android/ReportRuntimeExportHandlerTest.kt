package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.EndpointConfig
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.util.Base64

class ReportRuntimeExportHandlerTest {
    private lateinit var server: MockWebServer

    @Before
    fun setUp() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun `exportReportRuntimePdf submits canonical export and downloads pdf artifact`() = runBlocking {
        val pdfBytes = "%PDF-1.7\nreport".toByteArray()
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"result":"{\"jobId\":\"job-1\",\"status\":\"queued\"}"}""")
        )
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody("""{"result":"{\"jobId\":\"job-1\",\"status\":\"succeeded\",\"artifactId\":\"artifact-1\",\"artifactRef\":\"report://runtime/performance\"}"}""")
        )
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {
                      "result": "{\"artifactId\":\"artifact-1\",\"name\":\"performance.pdf\",\"contentType\":\"application/pdf\",\"data\":\"${Base64.getEncoder().encodeToString(pdfBytes)}\"}"
                    }
                    """.trimIndent()
                )
        )
        val client = AgentlyClient(
            endpoints = mapOf(
                "appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/'))
            )
        )

        val exported = exportReportRuntimePdf(
            client = client,
            conversationId = "conversation-1",
            exportRequest = mapOf(
                "artifactRef" to "report://runtime/performance",
                "title" to "Performance",
                "reportSpec" to mapOf("kind" to "reportSpec"),
                "reportFill" to mapOf("kind" to "reportFill"),
                "reportPrint" to mapOf("kind" to "reportPrint")
            )
        )

        assertEquals("artifact-1", exported.file.id)
        assertEquals("performance.pdf", exported.file.filename)
        assertEquals("application/pdf", exported.downloaded.contentType)
        assertTrue(exported.downloaded.data.contentEquals(pdfBytes))
        assertEquals(
            "/v1/tools/reporting%3Asubmit_export/execute?conversationId=conversation-1",
            server.takeRequest().path
        )
        assertEquals(
            "/v1/tools/reporting%3Aget_export_status/execute?conversationId=conversation-1",
            server.takeRequest().path
        )
        assertEquals(
            "/v1/tools/reporting%3Aget_artifact/execute?conversationId=conversation-1",
            server.takeRequest().path
        )
    }

    @Test
    fun `exportReportRuntimePdf compiles and exports inline fences in one backend call`() = runBlocking {
        val pdfBytes = "%PDF-1.7\ninline".toByteArray()
        server.enqueue(MockResponse().setBody(
            """{"result":"{\"job\":{\"jobId\":\"job-2\",\"status\":\"succeeded\",\"artifactId\":\"artifact-2\"},\"artifact\":{\"artifactId\":\"artifact-2\",\"contentType\":\"application/pdf\"}}"}"""
        ))
        server.enqueue(MockResponse().setBody(
            """{"result":"{\"artifactId\":\"artifact-2\",\"name\":\"inline.pdf\",\"contentType\":\"application/pdf\",\"data\":\"${Base64.getEncoder().encodeToString(pdfBytes)}\"}"}"""
        ))
        val client = AgentlyClient(
            endpoints = mapOf("appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/')))
        )

        val exported = exportReportRuntimePdf(
            client = client,
            conversationId = "conversation-2",
            exportRequest = mapOf(
                "reportId" to "inline-report",
                "title" to "Inline",
                "fences" to listOf(
                    mapOf(
                        "kind" to "forge-report",
                        "payload" to mapOf("version" to 1, "id" to "inline-report", "mode" to "commit")
                    )
                )
            )
        )

        assertEquals("artifact-2", exported.file.id)
        assertEquals(
            "/v1/tools/reporting%3Acompile_and_export_fenced_report/execute?conversationId=conversation-2",
            server.takeRequest().path
        )
        assertEquals(
            "/v1/tools/reporting%3Aget_artifact/execute?conversationId=conversation-2",
            server.takeRequest().path
        )
    }

    @Test
    fun `report export errors hide transport urls`() {
        val message = reportRuntimeExportErrorMessage(
            IllegalStateException(
                "POST https://example.invalid/v1/tools/reporting failed: 400: " +
                    "{\"error\":\"reporting export: invalid reportSpec: missing version\"}"
            )
        )

        assertEquals(
            "Unable to create the report PDF: invalid reportSpec: missing version",
            message
        )
    }

    @Test
    fun `report export errors preserve escaped field names`() {
        val message = reportRuntimeExportErrorMessage(
            IllegalStateException(
                "POST https://example.invalid/v1/tools/reporting failed: 400: " +
                    "{\"error\":\"reporting export: decode tableBlock: json: unknown field \\\"legacyField\\\"\"}"
            )
        )

        assertEquals(
            "Unable to create the report PDF: decode tableBlock: json: unknown field \"legacyField\"",
            message
        )
    }

    @Test
    fun `report export errors explain temporary scratchpad storage failure`() {
        val message = reportRuntimeExportErrorMessage(
            IllegalStateException(
                "request failed: 500: {\"error\":\"reporting scratchpad publish: " +
                    "upload artifact bytes failed: unable to generate access token\"}"
            )
        )

        assertEquals(
            "The PDF was created, but report storage is temporarily unavailable. Please try again.",
            message
        )
    }
}
