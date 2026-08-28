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
                      "rows": [{"accountId": 13579, "accountName": "Acme"}],
                      "dataInfo": {"recordCount": 1, "pageCount": 1},
                      "metrics": {"primaryValue": 180, "secondaryIndex": 26}
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
                dataSourceRef = "entity_performance_profile",
                dataSource = DataSourceDef(
                    service = ServiceDef(
                        endpoint = "agentlyAPI",
                        uri = "/v1/api/datasources/entity_performance_profile/fetch",
                        method = "POST"
                    )
                ),
                input = InputState(fetch = true)
            )
        )

        val recorded = server.takeRequest()
        assertEquals("/v1/api/datasources/entity_performance_profile/fetch", recorded.path)
        assertEquals(13579L, result?.rows?.firstOrNull()?.get("accountId"))
        assertEquals(180L, result?.metrics?.get("primaryValue"))
        assertEquals(26L, result?.metrics?.get("secondaryIndex"))
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
                dataSourceRef = "account_lookup",
                dataSource = DataSourceDef(
                    service = ServiceDef(
                        endpoint = "agentlyAPI",
                        uri = "/v1/api/datasources/account_lookup/fetch",
                        method = "POST"
                    )
                ),
                input = InputState(fetch = true)
            )
        )

        assertTrue(result?.metrics?.isEmpty() == true)
    }

    @Test
    fun `loader adapts scheduler API to forge rows and paging`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {"data":{"schedules":[
                      {"id":"one","name":"Hello World","agentRef":"steward","enabled":false,"scheduleType":"cron"},
                      {"id":"two","name":"Other","agentRef":"steward","enabled":true,"scheduleType":"interval"}
                    ]}}
                    """.trimIndent()
                )
        )
        val client = AgentlyClient(
            endpoints = mapOf("appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/')))
        )

        val result = makeForgeAgentlyDataSourceLoader(client)(
            ForgeRuntime.DataSourceFetchRequest(
                windowId = "automation",
                dataSourceRef = "schedules",
                dataSource = DataSourceDef(
                    service = ServiceDef(endpoint = "agentlyAPI", uri = "/v1/api/agently/scheduler/", method = "GET"),
                    paging = com.viant.forgeandroid.runtime.PagingDef(size = 10, enabled = true)
                ),
                input = InputState(filter = mapOf("name" to "hello"), fetch = true)
            )
        )

        assertEquals("/v1/api/agently/scheduler/", server.takeRequest().path)
        assertEquals(listOf("one"), result?.rows?.map { it["id"] })
        assertEquals(1, result?.metrics?.get("totalCount"))
    }

    @Test
    fun `workspace agent lookup uses canonical public agents endpoint`() = runBlocking {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json")
                .setBody(
                    """
                    {"agentInfos":[
                      {"id":"steward","name":"Steward","internal":false}
                    ]}
                    """.trimIndent()
                )
        )
        val client = AgentlyClient(
            endpoints = mapOf("appAPI" to EndpointConfig(baseUrl = server.url("/").toString().trimEnd('/')))
        )

        val result = makeForgeAgentlyDataSourceLoader(client)(
            ForgeRuntime.DataSourceFetchRequest(
                windowId = "automation",
                dataSourceRef = "agentsLov",
                dataSource = DataSourceDef(
                    service = ServiceDef(endpoint = "agentlyAPI", uri = "/v1/workspace/metadata/publicagents", method = "GET"),
                    selectors = com.viant.forgeandroid.runtime.SelectorDef(data = "agentInfos")
                ),
                input = InputState(fetch = true)
            )
        )

        assertEquals(listOf("steward"), result?.rows?.map { it["id"] })
    }
}
