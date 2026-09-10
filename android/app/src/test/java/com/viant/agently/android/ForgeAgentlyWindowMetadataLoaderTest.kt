package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.EndpointConfig
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ForgeTargetContext
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ForgeAgentlyWindowMetadataLoaderTest {
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
    fun `authorized metadata is applied before native renderer receives it`() = runBlocking {
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"data":{"authorization":{"scope":"resource"},"view":{"content":{"containers":[{"id":"complete"}]}}}}"""
            )
        )
        server.enqueue(
            MockResponse().setHeader("Content-Type", "application/json").setBody(
                """{"data":{"authorization":{"scope":"resource"},"authorizationSnapshot":{"authorizationVersion":"v1"},"view":{"content":{"containers":[{"id":"permitted"}]}}}}"""
            )
        )
        val client = AgentlyClient(
            endpoints = mapOf("appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/')))
        )
        val loader = makeForgeAgentlyWindowMetadataLoader(
            client,
            ForgeTargetContext(platform = "android", formFactor = "phone")
        )

        val metadata = loader(
            ForgeRuntime.WindowMetadataRequest(
                windowId = "advertiser-85141",
                windowKey = "advertiser",
                parameters = mapOf("AdvertiserId" to listOf(85141)),
                conversationId = "conv-1"
            )
        )

        assertEquals("permitted", metadata?.view?.content?.containers?.singleOrNull()?.id)
        val completeRequest = server.takeRequest()
        assertFalse(completeRequest.requestUrl!!.queryParameterNames.contains("applyPermission"))
        val permissionRequest = server.takeRequest()
        assertEquals("true", permissionRequest.requestUrl!!.queryParameter("applyPermission"))
        assertEquals("conv-1", permissionRequest.requestUrl!!.queryParameter("conversationId"))
        assertTrue(permissionRequest.requestUrl!!.queryParameter("resource")!!.contains("85141"))
    }
}
