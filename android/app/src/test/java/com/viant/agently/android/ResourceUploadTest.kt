package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.EndpointConfig
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.Assert.assertEquals
import org.junit.Test

class ResourceUploadTest {
    @Test
    fun `composer prefers resource references and retains legacy fallback`(): Unit = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            server.enqueue(MockResponse().setBody("""{"id":"new","uri":"/v1/files/new","resource":{"uri":"scratchpad://artifact/new","id":"new","name":"new.csv","mimeType":"text/csv","sizeBytes":2}}"""))
            server.enqueue(MockResponse().setBody("""{"ID":"old","URI":"/v1/files/old"}"""))
            val client = AgentlyClient(endpoints = mapOf("appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/'))))
            val uploads = uploadComposerAttachments(client, "conv-1", listOf(
                ComposerAttachmentDraft(name = "new.csv", mimeType = "text/csv", bytes = "id".encodeToByteArray(), source = "file"),
                ComposerAttachmentDraft(name = "old.csv", mimeType = "text/csv", bytes = "id".encodeToByteArray(), source = "file")
            ))
            assertEquals(listOf("scratchpad://artifact/new"), uploads.resourceURIs)
            assertEquals(listOf("/v1/files/old"), uploads.attachments.map { it.uri })
            assertEquals("/v1/files", server.takeRequest().path)
            assertEquals("/v1/files", server.takeRequest().path)
        } finally { server.shutdown() }
    }
}
