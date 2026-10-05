package com.viant.agently.android

import com.viant.agentlysdk.*
import com.viant.agentlysdk.EndpointConfig
import com.viant.forgeandroid.runtime.*
import kotlinx.coroutines.*
import kotlinx.serialization.json.*
import okhttp3.mockwebserver.*
import org.junit.Assert.*
import org.junit.Test

class NativeReportRunLifecycleHandlerTest {
    private fun admission(): NativeReportAdmission {
        val request = buildJsonObject { put("filters", buildJsonObject { put("From", "2026-09-27"); put("To", "2026-10-04"); put("orderIds", JsonArray(listOf(JsonPrimitive(2659534)))) }); put("limit", 1000); put("offset", 0) }
        val source = ReportBuilderPublishedDataSourceDef(id = "summary", dataSourceRef = "cube", request = buildJsonObject { put("limit", 1); put("offset", 0) })
        val state = buildJsonObject { put("viewMode", "table"); put("groupBy", "channelId"); put("pageSize", 50); put("orderField", "eventDate"); put("orderDir", "asc") }
        val prepared = PreparedReportRequest(ReportPreparationIdentity("W", "builder", "form", "state"), "ready", "cube", request, state, publishedSources = listOf(source))
        val document = buildJsonObject { put("title", "Delivery"); put("blocks", JsonArray(listOf(buildJsonObject { put("id", "spend"); put("kind", "kpiBlock"); put("datasetRef", "summary") }))) }
        return NativeReportAdmission(prepared, "conversation", "builder-state", document, listOf(NativeReportDatasetAdmission("summary", "cube", preparePublishedReportRequest(prepared.identity, prepared, source).request!!)),buildJsonObject { put("opaque",true) })
    }
    private fun run(admission: NativeReportAdmission, status: String, spec: JsonElement? = null, fill: JsonElement? = null, print: JsonElement? = null) = buildJsonObject {
        put("reportRunId", "durable-run"); put("ownerId", "owner"); put("status", status); put("revision", if (status == "running") 1 else 2)
        put("conversationId", admission.conversationId); put("builderRef", admission.preparation.identity.builderRef); put("effectiveParams", admission.preparation.primaryRequest)
        put("requestedParams",nativeReportRequestedParams(admission))
        spec?.let { put("reportSpec", it) }; fill?.let { put("reportFill", it) }; print?.let { put("reportPrint", it) }
    }
    private fun beginResponse(admission: NativeReportAdmission) = buildJsonObject {
        put("run", run(admission, "running")); put("context", buildJsonObject { put("ownerId", "owner"); put("conversationId", admission.conversationId); put("activeReportRunId", "previous-run"); put("revision", 7) })
    }
    private fun response(body: JsonElement) = MockResponse().setHeader("Content-Type", "application/json").setBody(body.toString())
    private fun fill(admission:NativeReportAdmission,rows:JsonObject=buildJsonObject { put("summary",JsonArray(emptyList())) }):JsonObject {
        val invocation=nativeReportInvocation(admission)
        return buildJsonObject { put("source",invocation.getValue("source"));put("parameters",invocation.getValue("parameters"));put("datasets",JsonArray(invocation.getValue("datasets").jsonArray.map { raw ->
            val dataset=raw.jsonObject;JsonObject(dataset+mapOf("rows" to (rows[dataset.getValue("id").jsonPrimitive.content]?:JsonArray(emptyList())),"provenance" to buildJsonObject { put("opaque",true) }))
        })) }
    }
    private fun enqueueSaved(server: MockWebServer, admission: NativeReportAdmission): JsonObject {
        val invocation = nativeReportInvocation(admission)
        val spec = buildJsonObject { put("source", invocation.getValue("source")); put("datasets", invocation.getValue("datasets")); put("parameters", invocation.getValue("parameters")) }
        val fill = fill(admission); val print = buildJsonObject { put("pages", JsonArray(emptyList())) }
        server.enqueue(response(beginResponse(admission)))
        server.enqueue(response(buildJsonObject { put("result", JsonPrimitive(buildJsonObject { put("reportSpec", spec); put("reportFill", fill); put("reportPrint", print) }.toString())) }))
        server.enqueue(response(run(admission, "completed", spec, fill, print)))
        return spec
    }
    @Test fun activationConflictKeepsSavedRecordAndNeverOverwritesNewerContext() = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            val admission = admission(); enqueueSaved(server, admission)
            server.enqueue(MockResponse().setResponseCode(409).setBody("context revision conflict"))
            server.enqueue(response(buildJsonObject { put("ownerId", "owner"); put("conversationId", "conversation"); put("activeReportRunId", "newer-run"); put("revision", 8) }))
            val handler = makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle = handler.begin(admission, "ui-request", "prompt")
            assertEquals(7L, handle.contextRevision)
            val result = handler.complete(handle, buildJsonObject { put("summary", JsonArray(emptyList())) }) { true }
            assertEquals("durable-run", result.reportRunId); assertEquals("superseded", result.contextStatus); assertEquals(false, result.active)
            handler.fail(handle, "should_not_fail", "Saved records cannot fail")
            assertEquals(5, server.requestCount)
            repeat(3) { server.takeRequest() }
            val activate = server.takeRequest(); assertTrue(activate.path!!.endsWith("/activate"))
            assertEquals(JsonPrimitive(7), Json.parseToJsonElement(activate.body.readUtf8()).jsonObject["expectedContextRevision"])
            assertTrue(server.takeRequest().path!!.contains("/context/"))
        } finally { server.shutdown() }
    }
    @Test fun lostCompletionResponseReadsExactArtifactThenActivatesWithoutRepeatingWrite() = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            val admission = admission(); val invocation = nativeReportInvocation(admission)
            val spec = buildJsonObject { put("source", invocation.getValue("source")); put("datasets", invocation.getValue("datasets")); put("parameters", invocation.getValue("parameters")) }
            val fill = fill(admission); val print = buildJsonObject { put("pages", JsonArray(emptyList())) }
            server.enqueue(response(beginResponse(admission)))
            server.enqueue(response(buildJsonObject { put("result", JsonPrimitive(buildJsonObject { put("reportSpec", spec); put("reportFill", fill); put("reportPrint", print) }.toString())) }))
            server.enqueue(MockResponse().setResponseCode(500).setBody("completion response lost"))
            server.enqueue(response(run(admission, "completed", spec, fill, print)))
            server.enqueue(response(buildJsonObject { put("ownerId", "owner"); put("conversationId", "conversation"); put("activeReportRunId", "durable-run"); put("revision", 8) }))
            val handler = makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle = handler.begin(admission, "ui-request", "prompt")
            val result = handler.complete(handle, buildJsonObject { put("summary", JsonArray(emptyList())) }) { true }
            assertEquals("active", result.contextStatus); assertEquals(5, server.requestCount)
            repeat(2) { server.takeRequest() }
            assertTrue(server.takeRequest().path!!.endsWith("/complete"))
            assertEquals("GET", server.takeRequest().method)
            assertTrue(server.takeRequest().path!!.endsWith("/activate"))
        } finally { server.shutdown() }
    }
    @Test fun unavailableActivationContextKeepsCompletedArtifactUnconfirmed() = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            val admission = admission(); enqueueSaved(server, admission)
            server.enqueue(MockResponse().setResponseCode(500).setBody("activation unavailable"))
            server.enqueue(MockResponse().setResponseCode(404).setBody("context unavailable"))
            val handler = makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle = handler.begin(admission, "ui-request", "prompt")
            val result = handler.complete(handle, buildJsonObject { put("summary", JsonArray(emptyList())) }) { true }
            assertEquals("durable-run", result.reportRunId); assertEquals(2L, result.revision)
            assertEquals("unconfirmed", result.contextStatus); assertNull(result.active); assertNotNull(result.activationError)
            handler.fail(handle, "should_not_fail", "Saved records cannot fail")
            assertEquals(5, server.requestCount)
        } finally { server.shutdown() }
    }
    @Test fun ambiguousActivationResponseReconcilesOnlyTheExactSavedPointer() = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            val admission = admission(); enqueueSaved(server, admission)
            server.enqueue(MockResponse().setResponseCode(500).setBody("response unavailable after activation"))
            server.enqueue(response(buildJsonObject { put("ownerId", "owner"); put("conversationId", "conversation"); put("activeReportRunId", "durable-run"); put("revision", 8) }))
            val handler = makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle = handler.begin(admission, "ui-request", "prompt")
            val result = handler.complete(handle, buildJsonObject { put("summary", JsonArray(emptyList())) }) { true }
            assertEquals("active", result.contextStatus); assertEquals(true, result.active); assertEquals(5, server.requestCount)
        } finally { server.shutdown() }
    }
    @Test fun exactAdmissionAndRowsReachDurableCompletionWithoutUsingFetchLimitAsPageSize() = runBlocking {
        val server = MockWebServer(); server.start()
        try {
            val admission = admission(); val invocation = nativeReportInvocation(admission)
            val rows = buildJsonObject { put("summary", JsonArray(listOf(buildJsonObject { put("totalSpend", 309) }))) }
            val spec = buildJsonObject { put("source", invocation.getValue("source")); put("datasets", invocation.getValue("datasets")); put("parameters", invocation.getValue("parameters")) }
            val fill = fill(admission,rows); val print = buildJsonObject { put("pages", JsonArray(emptyList())) }
            server.enqueue(response(buildJsonObject { put("run", run(admission, "running")) }))
            server.enqueue(response(buildJsonObject { put("result", JsonPrimitive(buildJsonObject { put("reportSpec", spec); put("reportFill", fill); put("reportPrint", print) }.toString())) }))
            server.enqueue(response(run(admission, "completed", spec, fill, print)))
            server.enqueue(response(buildJsonObject { put("ownerId", "owner"); put("conversationId", "conversation"); put("activeReportRunId", "durable-run"); put("revision", 1) }))
            val handler = makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle = handler.begin(admission, "ui-request", "ui.report.run")
            val completed = handler.complete(handle, rows) { true }
            assertEquals("durable-run", completed.reportRunId)
            assertEquals(fill.getValue("datasets"),completed.verifiedDatasets)
            assertEquals(0L, handle.contextRevision); assertEquals("active", completed.contextStatus); assertEquals(true, completed.active)
            val begin = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals(JsonPrimitive("prompt"), begin["origin"]); assertEquals(admission.preparation.primaryRequest, begin["effectiveParams"])
            assertEquals(nativeReportRequestedParams(admission),begin["requestedParams"])
            val compile = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals(invocation, compile["invocation"])
            assertEquals(JsonPrimitive(50), invocation.getValue("parameters").jsonObject["pageSize"])
            assertEquals(JsonPrimitive(1000), admission.preparation.primaryRequest["limit"])
            val complete = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals(JsonPrimitive(1), complete["expectedRevision"]); assertEquals(spec, complete["reportSpec"]); assertEquals(fill, complete["reportFill"])
            val activate = Json.parseToJsonElement(server.takeRequest().body.readUtf8()).jsonObject
            assertEquals(JsonPrimitive(2), activate["expectedRunRevision"]); assertEquals(JsonPrimitive(0), activate["expectedContextRevision"])
        } finally { server.shutdown() }
    }
    @Test fun oldCompilerCannotCompleteAnArtifactThatDropsInvocationIdentity() = runBlocking {
        val server = MockWebServer(); server.start(); val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
        try {
            val admission = admission()
            server.enqueue(response(beginResponse(admission)))
            server.enqueue(response(buildJsonObject { put("result", JsonPrimitive("{\"reportSpec\":{\"source\":{\"kind\":\"fenced\"},\"datasets\":[]},\"reportFill\":{},\"reportPrint\":{}}")) }))
            server.enqueue(response(run(admission, "running")))
            server.enqueue(response(run(admission, "failed")))
            val lifecycle = NativeReportLifecycle(); lifecycle.register(makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/'))))))
            lifecycle.publish(admission); val handle = lifecycle.begin("W", "ui-request", "prompt", admission.preparation) { true }
            val failed = CompletableDeferred<String>()
            lifecycle.start(scope, handle, { true }, { JsonArray(emptyList()) }, {}, { _, _ -> fail("Old compiler must not complete") }, { failed.complete(it) })
            assertTrue(withTimeout(3000) { failed.await() }.contains("compiler"))
            assertTrue(server.takeRequest().path!!.contains("/begin")); assertTrue(server.takeRequest().path!!.contains("compile_fenced_report"))
            assertTrue(server.takeRequest().path!!.contains("/durable-run?"))
            assertTrue(server.takeRequest().path!!.contains("/fail")); assertNull(lifecycle.completed("ui-request"))
        } finally { scope.cancel(); server.shutdown() }
    }
    @Test fun mismatchedFilledRequestCannotReachCompletionOrActivation()=runBlocking {
        val server=MockWebServer();server.start()
        try {
            val admission=admission();val invocation=nativeReportInvocation(admission)
            val spec=buildJsonObject { put("source",invocation.getValue("source"));put("parameters",invocation.getValue("parameters"));put("datasets",invocation.getValue("datasets")) }
            val original=fill(admission);val datasets=original.getValue("datasets").jsonArray.toMutableList()
            datasets[0]=JsonObject(datasets[0].jsonObject+mapOf("request" to JsonObject(emptyMap())))
            val bad=JsonObject(original+mapOf("datasets" to JsonArray(datasets)))
            server.enqueue(response(beginResponse(admission)))
            server.enqueue(response(buildJsonObject { put("result",JsonPrimitive(buildJsonObject { put("reportSpec",spec);put("reportFill",bad);put("reportPrint",JsonObject(emptyMap())) }.toString())) }))
            val handler=makeNativeReportRunLifecycleHandler(AgentlyClient(mapOf("appAPI" to EndpointConfig(server.url("/").toString().trimEnd('/')))))
            val handle=handler.begin(admission,"ui-request","prompt")
            assertTrue(runCatching { handler.complete(handle,buildJsonObject { put("summary",JsonArray(emptyList())) }) { true } }.exceptionOrNull()?.message.orEmpty().contains("fill"))
            assertEquals(2,server.requestCount)
        } finally { server.shutdown() }
    }
}
