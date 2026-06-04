package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.EndpointConfig
import com.viant.forgeandroid.runtime.DataSourceDef
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.InputState
import com.viant.forgeandroid.runtime.ServiceDef
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

class ForgeAgentlyDataSourceLoaderTest {
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
    fun `loader maps forge metrics from datasource metrics field`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {
                      "rows": [{"advertiserId": 13579, "advertiserName": "Acme"}],
                      "dataInfo": {"recordCount": 1, "pageCount": 1},
                      "metrics": {"dailyBudget": 180, "lifetimePacingIndex": 26}
                    }
                    """.trimIndent()
                )
        )

        val client = AgentlyClient(
            endpoints = mapOf(
                "appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/'))
            )
        )
        val loader = makeForgeAgentlyDataSourceLoader(client)

        val result = loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowId = "w1",
                dataSourceRef = "order_performance_profile",
                dataSource = DataSourceDef(
                    service = ServiceDef(
                        endpoint = "agentlyAPI",
                        uri = "/v1/api/datasources/order_performance_profile/fetch",
                        method = "POST"
                    )
                ),
                input = InputState(fetch = true)
            )
        )

        val recorded = server.takeRequest()
        assertEquals("/v1/api/datasources/order_performance_profile/fetch", recorded.path)
        assertEquals(13579L, result?.rows?.firstOrNull()?.get("advertiserId"))
        assertEquals(180L, result?.metrics?.get("dailyBudget"))
        assertEquals(26L, result?.metrics?.get("lifetimePacingIndex"))
    }

    @Test
    fun `loader does not treat dataInfo as forge metrics when metrics are absent`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {
                      "rows": [],
                      "dataInfo": {"recordCount": 3, "pageCount": 1}
                    }
                    """.trimIndent()
                )
        )

        val client = AgentlyClient(
            endpoints = mapOf(
                "appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/'))
            )
        )
        val loader = makeForgeAgentlyDataSourceLoader(client)

        val result = loader(
            ForgeRuntime.DataSourceFetchRequest(
                windowId = "w1",
                dataSourceRef = "advertiser_lookup",
                dataSource = DataSourceDef(
                    service = ServiceDef(
                        endpoint = "agentlyAPI",
                        uri = "/v1/api/datasources/advertiser_lookup/fetch",
                        method = "POST"
                    )
                ),
                input = InputState(fetch = true)
            )
        )

        assertTrue(result?.metrics?.isEmpty() == true)
    }
}
