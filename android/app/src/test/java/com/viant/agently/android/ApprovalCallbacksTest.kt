package com.viant.agently.android

import com.viant.agentlysdk.ApprovalCallback
import com.viant.agentlysdk.ApprovalCallbackPayload
import com.viant.agentlysdk.ApprovalForgeView
import com.viant.agentlysdk.ApprovalMeta
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Test

class ApprovalCallbacksTest {

    @Test
    fun executeApprovalCallbacks_filtersSelectionsToOriginalOrder() = runBlocking {
        val meta = ApprovalMeta(
            toolName = "deploy",
            forge = ApprovalForgeView(
                callbacks = listOf(
                    ApprovalCallback(
                        event = "approve",
                        handler = "approval.filterEnvNames"
                    )
                )
            )
        )
        val payload = ApprovalCallbackPayload(
            editedFields = JsonObject(
                mapOf(
                    "names" to JsonArray(
                        listOf(
                            JsonPrimitive("prod"),
                            JsonPrimitive("dev")
                        )
                    )
                )
            ),
            originalArgs = JsonObject(
                mapOf(
                    "names" to JsonArray(
                        listOf(
                            JsonPrimitive("dev"),
                            JsonPrimitive("stage"),
                            JsonPrimitive("prod")
                        )
                    )
                )
            )
        )

        val result = executeApprovalCallbacks(meta, "approve", payload)
        val names = ((result.editedFields as JsonObject)["names"] as JsonArray).map { (it as JsonPrimitive).content }

        assertEquals(listOf("dev", "prod"), names)
    }
}
