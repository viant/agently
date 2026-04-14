package com.viant.agently.android

import com.viant.agentlysdk.ApprovalEditor
import com.viant.agentlysdk.ApprovalForgeView
import com.viant.agentlysdk.ApprovalMeta
import com.viant.agentlysdk.ApprovalOption
import com.viant.agentlysdk.PendingToolApproval
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ForgeTargetContext
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test

class ApprovalComponentsTest {

    private val approvalJson = Json { ignoreUnknownKeys = true }
    private val sharedForgeRuntime = ForgeRuntime(
        endpoints = mapOf(
            "appAPI" to com.viant.forgeandroid.runtime.EndpointConfig(baseUrl = "http://localhost:9496")
        ),
        scope = CoroutineScope(SupervisorJob() + Dispatchers.Unconfined),
        targetContext = buildForgeTargetContext("phone")
    )

    @Test
    fun buildApprovalEditorWindow_usesForgeSchemaFormWhenContainerRefIsPresent() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            editors = listOf(
                ApprovalEditor(
                    name = "envNames",
                    kind = "multi_list",
                    label = "Environments",
                    description = "Pick environments",
                    options = listOf(
                        ApprovalOption(id = "dev", label = "Dev", selected = true),
                        ApprovalOption(id = "prod", label = "Prod")
                    )
                )
            ),
            forge = ApprovalForgeView(containerRef = "approvalEnvPicker")
        )

        val window = buildApprovalEditorWindow(metadata)
        val container = window.view?.content?.containers?.single()
            ?: error("Expected a single approval container")

        assertEquals("approvalEnvPicker", container.id)
        assertNotNull(container.schemaBasedForm)
        assertTrue(container.items.isEmpty())

        val schema = container.schemaBasedForm?.schema as JsonObject
        val properties = schema["properties"] as JsonObject
        val envNames = properties["envNames"] as JsonObject
        assertEquals("array", (envNames["type"] as JsonPrimitive).content)
        assertEquals("multiSelect", (envNames["x-ui-widget"] as JsonPrimitive).content)
        val defaults = envNames["default"] as JsonArray
        assertEquals(listOf("dev"), defaults.map { (it as JsonPrimitive).content })
    }

    @Test
    fun buildApprovalEditorWindow_usesForgeDataSourceOverrideWhenProvided() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            editors = listOf(
                ApprovalEditor(
                    name = "envNames",
                    kind = "multi_list",
                    label = "Environments",
                    options = listOf(
                        ApprovalOption(id = "dev", label = "Dev", selected = true)
                    )
                )
            ),
            forge = ApprovalForgeView(
                containerRef = "approvalEnvPicker",
                dataSource = "approvalEditor"
            )
        )

        val window = buildApprovalEditorWindow(metadata)
        val container = window.view?.content?.containers?.single()
            ?: error("Expected a single approval container")

        assertTrue(window.dataSources.containsKey("approvalEditor"))
        assertEquals("approvalEditor", container.dataSourceRef)
        assertEquals("approvalEditor", container.schemaBasedForm?.dataSourceRef)
    }

    @Test
    fun buildApprovalEditorWindow_usesItemListWhenForgeContainerIsMissing() {
        val metadata = ApprovalMeta(
            title = "Runtime approval",
            toolName = "runtime",
            editors = listOf(
                ApprovalEditor(
                    name = "target",
                    kind = "radio_list",
                    label = "Target",
                    options = listOf(
                        ApprovalOption(id = "canary", label = "Canary", selected = true),
                        ApprovalOption(id = "stable", label = "Stable")
                    )
                )
            )
        )

        val window = buildApprovalEditorWindow(metadata)
        val container = window.view?.content?.containers?.single()
            ?: error("Expected a single approval container")

        assertEquals("approvalEditors", container.id)
        assertFalse(container.items.isEmpty())
        assertEquals(null, container.schemaBasedForm)

        val item = container.items.single()
        assertEquals("target", item.id)
        assertEquals("radio", item.type)
        assertEquals(listOf("canary", "stable"), item.options.map { it.value })
        assertEquals(true, item.options.first().default)
    }

    @Test
    fun buildApprovalEditorWindow_keepsForgeContainerEvenWithoutEditors() {
        val metadata = ApprovalMeta(
            title = "Rich approval",
            toolName = "deploy",
            editors = emptyList(),
            forge = ApprovalForgeView(containerRef = "approvalEnvPicker")
        )

        val window = buildApprovalEditorWindow(metadata)
        val container = window.view?.content?.containers?.single()
            ?: error("Expected a single approval container")

        assertEquals("approvalEnvPicker", container.id)
        assertNotNull(container.schemaBasedForm)
        assertTrue(container.items.isEmpty())
    }

    @Test
    fun buildApprovalSurfaceWindow_wrapsMetadataAroundEditorContainer() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            message = "Choose target environments before continuing.",
            toolName = "deploy",
            editors = listOf(
                ApprovalEditor(
                    name = "envNames",
                    kind = "multi_list",
                    label = "Environments",
                    options = listOf(
                        ApprovalOption(id = "dev", label = "Dev", selected = true)
                    )
                )
            ),
            forge = ApprovalForgeView(containerRef = "approvalEnvPicker")
        )

        val window = buildApprovalSurfaceWindow(
            approval = PendingToolApproval(
                id = "approval-1",
                toolName = "deploy",
                title = "Deploy",
                status = "pending"
            ),
            meta = metadata
        )
        val outer = window.view?.content?.containers?.single()
            ?: error("Expected a single approval surface container")
        val nested = outer.containers.single()

        assertEquals("approvalSurface", outer.id)
        assertEquals("Deploy approval", outer.title)
        assertEquals("Choose target environments before continuing.", outer.subtitle)
        assertEquals("approvalEnvPicker", nested.id)
        assertNotNull(nested.schemaBasedForm)
    }

    @Test
    fun approvalUsesRemoteWindow_requiresSharedRuntimeAndWindowRef() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            forge = ApprovalForgeView(windowRef = "approval/envPicker")
        )

        assertTrue(approvalUsesRemoteWindow(metadata, sharedForgeRuntime))
        assertFalse(approvalUsesRemoteWindow(metadata, null))
    }

    @Test
    fun approvalRequiresSharedRuntime_tracksWindowRefIndependently() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            forge = ApprovalForgeView(windowRef = "approval/envPicker")
        )

        assertTrue(approvalRequiresSharedRuntime(metadata))
    }

    @Test
    fun approvalUsesRemoteWindow_ignoresBlankWindowRef() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            forge = ApprovalForgeView(windowRef = " ")
        )

        assertFalse(approvalUsesRemoteWindow(metadata, sharedForgeRuntime))
        assertFalse(approvalRequiresSharedRuntime(metadata))
    }

    @Test
    fun resolveApprovalRenderState_targetsSpecificContainerRef() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            forge = ApprovalForgeView(containerRef = "approvalEnvPicker")
        )
        val window = buildApprovalSurfaceWindow(
            approval = PendingToolApproval(
                id = "approval-1",
                toolName = "deploy",
                title = "Deploy",
                status = "pending"
            ),
            meta = metadata.copy(
                editors = listOf(
                    ApprovalEditor(
                        name = "envNames",
                        kind = "multi_list",
                        label = "Environments",
                        options = listOf(ApprovalOption(id = "dev", label = "Dev", selected = true))
                    )
                )
            )
        )

        val state = resolveApprovalRenderState(window, metadata, usesRemoteWindow = true)

        assertNull(state.error)
        assertEquals(1, state.containers.size)
        assertEquals("approvalEnvPicker", state.containers.single().id)
    }

    @Test
    fun resolveApprovalRenderState_returnsExplicitErrorWhenContainerRefMissing() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            forge = ApprovalForgeView(containerRef = "missing")
        )
        val window = buildApprovalEditorWindow(
            ApprovalMeta(
                title = "Deploy approval",
                toolName = "deploy",
                editors = listOf(
                    ApprovalEditor(
                        name = "target",
                        kind = "radio_list",
                        label = "Target",
                        options = listOf(ApprovalOption(id = "canary", label = "Canary", selected = true))
                    )
                )
            )
        )

        val state = resolveApprovalRenderState(window, metadata, usesRemoteWindow = true)

        assertTrue(state.containers.isEmpty())
        assertEquals(
            "Forge approval view unavailable: container \"missing\" not found.",
            state.error
        )
    }

    @Test
    fun decodeApprovalMeta_readsNestedApprovalEnvelope() {
        val metadata = Json.parseToJsonElement(
            """
            {
              "approval": {
                "toolName": "system_os-getEnv",
                "title": "OS Env Access",
                "message": "The agent wants access to your HOME and PATH environment variables.",
                "editors": [
                  {
                    "name": "names",
                    "kind": "checkbox_list",
                    "label": "Environment variables",
                    "options": [
                      { "id": "HOME", "label": "HOME", "selected": true },
                      { "id": "PATH", "label": "PATH", "selected": true }
                    ]
                  }
                ]
              },
              "opId": "call-1"
            }
            """.trimIndent()
        )

        val decoded = decodeApprovalMeta(metadata, approvalJson)

        assertEquals("OS Env Access", decoded?.title)
        assertEquals("system_os-getEnv", decoded?.toolName)
        assertEquals(1, decoded?.editors?.size)
        assertEquals("names", decoded?.editors?.firstOrNull()?.name)
    }

    @Test
    fun decodeApprovalMeta_throwsWhenEditorIsMalformed() {
        val metadata = Json.parseToJsonElement(
            """
            {
              "approval": {
                "toolName": "system_os-getEnv",
                "title": "OS Env Access",
                "editors": [
                  {
                    "kind": "checkbox_list",
                    "label": "Environment variables"
                  }
                ]
              }
            }
            """.trimIndent()
        )

        try {
            decodeApprovalMeta(metadata, approvalJson)
            fail("Expected malformed approval editor to throw")
        } catch (error: IllegalStateException) {
            assertTrue(error.message.orEmpty().contains("Approval editor"))
        }
    }

    @Test
    fun decodeApprovalMeta_throwsWhenCallbackIsMalformed() {
        val metadata = Json.parseToJsonElement(
            """
            {
              "approval": {
                "toolName": "system_os-getEnv",
                "forge": {
                  "containerRef": "approvalEnvPicker",
                  "callbacks": [
                    {
                      "event": "submit"
                    }
                  ]
                }
              }
            }
            """.trimIndent()
        )

        try {
            decodeApprovalMeta(metadata, approvalJson)
            fail("Expected malformed approval callback to throw")
        } catch (error: IllegalStateException) {
            assertTrue(error.message.orEmpty().contains("Approval callback"))
        }
    }

    @Test
    fun buildApprovalEditorSeed_preservesOriginalArgsPayload() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            editors = listOf(
                ApprovalEditor(
                    name = "envNames",
                    kind = "multi_list",
                    label = "Environments",
                    options = listOf(
                        ApprovalOption(id = "dev", label = "Dev", selected = true),
                        ApprovalOption(id = "prod", label = "Prod")
                    )
                )
            ),
            forge = ApprovalForgeView(containerRef = "approvalEnvPicker")
        )
        val originalArgs = JsonObject(
            mapOf(
                "names" to JsonArray(listOf(JsonPrimitive("dev"), JsonPrimitive("prod"))),
                "mode" to JsonPrimitive("safe")
            )
        )

        val seed = buildApprovalEditorSeed(metadata, emptyMap(), originalArgs)

        val seededArgs = seed["originalArgs"] as Map<*, *>
        assertEquals(listOf("dev", "prod"), seededArgs["names"])
        assertEquals("safe", seededArgs["mode"])
    }

    @Test
    fun buildApprovalEditorSeed_includesEditedFieldsPayload() {
        val metadata = ApprovalMeta(
            title = "Deploy approval",
            toolName = "deploy",
            editors = listOf(
                ApprovalEditor(
                    name = "envNames",
                    kind = "multi_list",
                    label = "Environments",
                    options = listOf(
                        ApprovalOption(id = "dev", label = "Dev", selected = true),
                        ApprovalOption(id = "prod", label = "Prod")
                    )
                )
            )
        )
        val selectedFields = mapOf(
            "envNames" to JsonArray(listOf(JsonPrimitive("prod")))
        )

        val seed = buildApprovalEditorSeed(metadata, selectedFields)

        assertEquals(listOf("prod"), seed["envNames"])
        val editedFields = seed["editedFields"] as Map<*, *>
        assertEquals(listOf("prod"), editedFields["envNames"])
    }

    @Test
    fun extractApprovalEditedFields_prefersLiveFieldsOverSeededEditedFields() {
        val edited = extractApprovalEditedFields(
            mapOf(
                "approval" to mapOf("toolName" to "deploy"),
                "editedFields" to mapOf(
                    "envNames" to listOf("prod")
                ),
                "envNames" to listOf("dev")
            )
        )

        assertEquals(mapOf("envNames" to listOf("dev")), edited)
    }
}
