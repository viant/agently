package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceAgentInfo
import com.viant.agentlysdk.WorkspaceDefaults
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.StarterTask
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AppSettingsRuntimeTest {

    @Test
    fun resolvePreferredAgentId_prefersAppOverride() {
        val metadata = WorkspaceMetadata(
            defaultAgent = "workspace-agent",
            defaults = WorkspaceDefaults(agent = "fallback-agent")
        )

        assertEquals("phone-agent", resolvePreferredAgentId("phone-agent", metadata))
    }

    @Test
    fun resolvePreferredAgentId_fallsBackToWorkspaceDefaults() {
        val metadata = WorkspaceMetadata(
            defaultAgent = "",
            defaults = WorkspaceDefaults(agent = "fallback-agent")
        )

        assertEquals("fallback-agent", resolvePreferredAgentId("", metadata))
    }

    @Test
    fun resolvePreferredAgentId_returnsNullWhenNothingAvailable() {
        assertNull(resolvePreferredAgentId("", null))
    }

    @Test
    fun workspaceAgentChoices_prefersAgentInfoLabelsAndDeduplicates() {
        val metadata = WorkspaceMetadata(
            agents = listOf("chatter", "planner", "internal-agent"),
            agentInfos = listOf(
                WorkspaceAgentInfo(id = "chatter", name = "Chatter"),
                WorkspaceAgentInfo(id = "planner", name = "Planner"),
                WorkspaceAgentInfo(id = "internal-agent", name = "Internal Agent", internalAgent = true)
            )
        )

        assertEquals(
            listOf(
                WorkspaceAgentChoice(id = "chatter", label = "Chatter"),
                WorkspaceAgentChoice(id = "planner", label = "Planner")
            ),
            workspaceAgentChoices(metadata)
        )
    }

    @Test
    fun resolveSelectedAgentLabel_prefersPublishedAgentLabel() {
        val metadata = WorkspaceMetadata(
            defaultAgent = "workspace-agent",
            agentInfos = listOf(
                WorkspaceAgentInfo(id = "workspace-agent", name = "Workspace Agent")
            )
        )

        assertEquals("Workspace Agent", resolveSelectedAgentLabel("", metadata))
    }

    @Test
    fun workspaceStarterTasks_readsSelectedAgentTasks() {
        val metadata = WorkspaceMetadata(
            defaultAgent = "workspace-agent",
            agentInfos = listOf(
                WorkspaceAgentInfo(
                    id = "workspace-agent",
                    name = "Workspace Agent",
                    starterTasks = listOf(
                        StarterTask(
                            id = "open-workspace",
                            title = "Open workspace",
                            prompt = "Open workspace",
                            description = "Launch the workspace."
                        )
                    )
                )
            )
        )

        assertEquals(1, workspaceStarterTasks("", metadata).size)
        assertEquals("open-workspace", workspaceStarterTasks("", metadata).first().id)
    }

    @Test
    fun resolveWorkspaceBrandTitle_preservesWorkspaceIdentityWithoutBrandHeuristics() {
        assertEquals(
            "Viant Workspace",
            resolveWorkspaceBrandTitle(
                workspaceRoot = "/tmp/Viant Workspace",
                defaultAgent = "workspace"
            )
        )
    }

    @Test
    fun resolveWorkspaceHeaderTitle_usesConfiguredAppNameThenWorkspaceTitle() {
        val metadata = WorkspaceMetadata(appName = "Steward")
        assertEquals("Steward", resolveWorkspaceHeaderTitle(metadata, "Viant Steward"))
        assertEquals("Metrics", resolveWorkspaceHeaderTitle(null, "Metrics"))
        assertEquals("Agently", resolveWorkspaceHeaderTitle(null, ""))
    }

    @Test
    fun buildSettingsApplyTransition_trimsAgentAndNormalizesBaseUrl() {
        val transition = buildSettingsApplyTransition(
            configuredBaseUrl = "http://configured",
            currentBaseUrl = "http://configured",
            nextBaseUrl = " http://configured/v1/api/ ",
            nextPreferredAgentId = " coder "
        )

        assertEquals("http://configured", transition.resolvedBaseUrl)
        assertEquals("coder", transition.preferredAgentId)
        assertEquals(false, transition.requiresWorkspaceReset)
    }

    @Test
    fun buildResetOverridesTransition_clearsAgentAndRequestsResetWhenNeeded() {
        val transition = buildResetOverridesTransition(
            configuredBaseUrl = "http://configured",
            currentBaseUrl = "http://custom"
        )

        assertEquals("http://configured", transition.resolvedBaseUrl)
        assertEquals("", transition.preferredAgentId)
        assertEquals(true, transition.requiresWorkspaceReset)
    }

    @Test
    fun configuredWorkspaceEndpointOptions_preserveAuthoredPresentation() {
        val options = mergeWorkspaceEndpointOptions(
            parseWorkspaceEndpointOptions(
                """
                [
                  {
                    "title": "Example",
                    "subtitle": "Example workspace",
                    "value": "https://workspace.example.com"
                  }
                ]
                """.trimIndent()
            )
        )
        val option = options.first()

        assertEquals(
            WorkspaceEndpointOption(
                title = "Example",
                subtitle = "Example workspace",
                value = "https://workspace.example.com"
            ),
            option
        )
        assertEquals(1, options.size)
    }

    @Test
    fun workspaceEndpointOptions_haveNoSourceLevelWorkspacePreset() {
        assertEquals(emptyList<WorkspaceEndpointOption>(), workspaceEndpointOptions)
    }

    @Test
    fun selectedWorkspaceEndpointOption_returnsNullForUnconfiguredLocalhost() {
        assertNull(selectedWorkspaceEndpointOption("http://localhost:9292/v1/api/"))
    }

    @Test
    fun mergeWorkspaceEndpointOptions_allowsExplicitDevEndpoints() {
        val options = mergeWorkspaceEndpointOptions(
            configured = listOf(
                WorkspaceEndpointOption(
                    title = "Development",
                    subtitle = "Development workspace",
                    value = "http://10.0.2.2:9292"
                )
            )
        )
        assertEquals(
            listOf(
                WorkspaceEndpointOption(
                    title = "Development",
                    subtitle = "Development workspace",
                    value = "http://10.0.2.2:9292"
                )
            ),
            options
        )
    }

    @Test
    fun normalizeApiBaseUrl_removesApiSuffixesAndTrailingSlash() {
        assertEquals(
            "https://workspace.example.com",
            normalizeApiBaseUrl(" https://workspace.example.com/v1/api/ ")
        )
        assertEquals(
            "https://workspace.example.com",
            normalizeApiBaseUrl("https://workspace.example.com/v1")
        )
    }

    @Test
    fun workspaceEndpointOption_derivesGenericPresentationFromHost() {
        assertEquals(
            WorkspaceEndpointOption(
                title = "workspace.example.com",
                subtitle = "Configured workspace",
                value = "https://workspace.example.com"
            ),
            workspaceEndpointOption("https://workspace.example.com/v1/api/")
        )
        assertNull(workspaceEndpointOption("not-a-url"))
    }

    @Test
    fun resolveInitialApiBaseUrl_explicitDebugEndpointOverridesStaleStoredEndpoint() {
        assertEquals(
            "http://127.0.0.1:8080",
            resolveInitialApiBaseUrl(
                configuredBaseUrl = "http://127.0.0.1:8080/",
                storedSettings = AppSettings(
                    baseUrlOverride = "http://127.0.0.1:18080",
                    hasWorkspaceEndpointSelection = true
                ),
                preferExplicitBuildEndpoint = true
            )
        )
    }

    @Test
    fun hasInitialWorkspaceEndpointSelection_explicitDebugEndpointSkipsProductionChooser() {
        assertTrue(
            hasInitialWorkspaceEndpointSelection(
                configuredBaseUrl = "https://workspace.example.com",
                storedSettings = AppSettings(),
                preferExplicitBuildEndpoint = true
            )
        )
        assertFalse(
            hasInitialWorkspaceEndpointSelection(
                configuredBaseUrl = "",
                storedSettings = AppSettings(),
                preferExplicitBuildEndpoint = false
            )
        )
    }

    @Test
    fun resolveInitialApiBaseUrl_preservesStoredEndpointForNormalBuild() {
        assertEquals(
            "http://127.0.0.1:18080",
            resolveInitialApiBaseUrl(
                configuredBaseUrl = "https://workspace.example.com",
                storedSettings = AppSettings(
                    baseUrlOverride = "http://127.0.0.1:18080",
                    hasWorkspaceEndpointSelection = true
                ),
                preferExplicitBuildEndpoint = false
            )
        )
    }

    @Test
    fun resolveInitialApiBaseUrl_ignoresInvalidStoredScheme() {
        assertEquals(
            "https://workspace.example.com",
            resolveInitialApiBaseUrl(
                configuredBaseUrl = "https://workspace.example.com",
                storedSettings = AppSettings(
                    baseUrlOverride = "hqttps://workspace.example.com",
                    hasWorkspaceEndpointSelection = true
                ),
                preferExplicitBuildEndpoint = false
            )
        )
    }

    @Test
    fun workspaceEndpointValidation_acceptsOnlyHttpUrlsWithHosts() {
        assertTrue(isValidWorkspaceBaseUrl("https://workspace.example.com"))
        assertTrue(isValidWorkspaceBaseUrl("http://127.0.0.1:9191"))
        assertFalse(isValidWorkspaceBaseUrl("hqttps://workspace.example.com"))
        assertFalse(isValidWorkspaceBaseUrl("https://"))
    }

    @Test
    fun resolveInitialApiBaseUrl_returnsEmptyWhenNothingWasConfigured() {
        assertEquals(
            "",
            resolveInitialApiBaseUrl(
                configuredBaseUrl = "",
                storedSettings = AppSettings(),
                preferExplicitBuildEndpoint = false
            )
        )
    }
}
