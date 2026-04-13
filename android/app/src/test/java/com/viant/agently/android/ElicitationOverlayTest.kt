package com.viant.agently.android

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ElicitationOverlayTest {

    @Test
    fun prepareRequestedSchema_stripsHiddenFieldsAndNormalizesDefaults() {
        val schema = JsonObject(
            mapOf(
                "type" to JsonPrimitive("object"),
                "required" to JsonArray(
                    listOf(
                        JsonPrimitive("_approvalMeta"),
                        JsonPrimitive("name"),
                        JsonPrimitive("tags")
                    )
                ),
                "properties" to JsonObject(
                    mapOf(
                        "_approvalMeta" to JsonObject(
                            mapOf("const" to JsonPrimitive("secret"))
                        ),
                        "name" to JsonObject(
                            mapOf(
                                "type" to JsonPrimitive("string"),
                                "title" to JsonPrimitive("Name")
                            )
                        ),
                        "tags" to JsonObject(
                            mapOf(
                                "type" to JsonPrimitive("array")
                            )
                        )
                    )
                )
            )
        )

        val prepared = prepareRequestedSchema(schema) ?: error("prepared schema missing")
        val properties = prepared["properties"] as JsonObject
        val required = prepared["required"] as JsonArray

        assertFalse(properties.containsKey("_approvalMeta"))
        assertTrue(properties.containsKey("name"))
        assertTrue(properties.containsKey("tags"))
        assertEquals(listOf("name", "tags"), required.map { (it as JsonPrimitive).content })

        val tags = properties["tags"] as JsonObject
        assertNotNull(tags["default"])
        assertTrue(tags["default"] is JsonArray)
    }

    @Test
    fun buildElicitationWindow_createsSchemaBackedFormContainer() {
        val schema = JsonObject(
            mapOf(
                "type" to JsonPrimitive("object"),
                "properties" to JsonObject(
                    mapOf(
                        "name" to JsonObject(
                            mapOf("type" to JsonPrimitive("string"))
                        )
                    )
                )
            )
        )

        val window = buildElicitationWindow(schema)
        val container = window.view?.content?.containers?.single()
            ?: error("Expected a single elicitation container")

        assertEquals("agently.android.elicitation", window.namespace)
        assertEquals("elicitationForm", container.id)
        assertNotNull(container.schemaBasedForm)
        assertEquals(schema, container.schemaBasedForm?.schema)
        assertEquals(false, container.schemaBasedForm?.showSubmit)
    }

    @Test
    fun extractToolApprovalMeta_readsRichHiddenApprovalMeta() {
        val schema = JsonObject(
            mapOf(
                "type" to JsonPrimitive("object"),
                "properties" to JsonObject(
                    mapOf(
                        "_approvalMeta" to JsonObject(
                            mapOf(
                                "const" to JsonPrimitive(
                                    """
                                    {
                                      "type":"tool_approval",
                                      "title":"Deploy approval",
                                      "toolName":"deploy",
                                      "acceptLabel":"Approve",
                                      "rejectLabel":"Reject",
                                      "cancelLabel":"Cancel",
                                      "editors":[
                                        {
                                          "name":"env",
                                          "kind":"radio_list",
                                          "label":"Environment",
                                          "options":[
                                            {"id":"prod","label":"Production","selected":true}
                                          ]
                                        }
                                      ]
                                    }
                                    """.trimIndent()
                                )
                            )
                        )
                    )
                )
            )
        )

        val meta = extractToolApprovalMeta(schema) ?: error("approval meta missing")

        assertEquals("tool_approval", meta.type)
        assertEquals("Deploy approval", meta.title)
        assertEquals("deploy", meta.toolName)
        assertEquals("Approve", meta.acceptLabel)
        assertEquals(1, meta.editors.size)
        assertEquals("env", meta.editors.first().name)
    }

    @Test
    fun extractToolApprovalMeta_readsLegacyHiddenFields() {
        val schema = JsonObject(
            mapOf(
                "type" to JsonPrimitive("object"),
                "properties" to JsonObject(
                    mapOf(
                        "_type" to JsonObject(mapOf("const" to JsonPrimitive("tool_approval"))),
                        "_title" to JsonObject(mapOf("const" to JsonPrimitive("Approval Required"))),
                        "_toolName" to JsonObject(mapOf("const" to JsonPrimitive("shell.exec"))),
                        "_acceptLabel" to JsonObject(mapOf("const" to JsonPrimitive("Allow"))),
                        "_rejectLabel" to JsonObject(mapOf("const" to JsonPrimitive("Deny"))),
                        "_cancelLabel" to JsonObject(mapOf("const" to JsonPrimitive("Back")))
                    )
                )
            )
        )

        val meta = extractToolApprovalMeta(schema) ?: error("approval meta missing")

        assertEquals("tool_approval", meta.type)
        assertEquals("Approval Required", meta.title)
        assertEquals("shell.exec", meta.toolName)
        assertEquals("Allow", meta.acceptLabel)
        assertEquals("Deny", meta.rejectLabel)
        assertEquals("Back", meta.cancelLabel)
    }
}
