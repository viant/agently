package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceAgentInfo
import com.viant.agentlysdk.WorkspaceDefaults
import com.viant.agentlysdk.WorkspaceMetadata
import com.viant.agentlysdk.StarterTask
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
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
    fun resolveWorkspaceBrandTitle_stripsViantPrefixWhenLogoAlreadyShowsIt() {
        assertEquals(
            "Workspace",
            resolveWorkspaceBrandTitle(
                workspaceRoot = "/tmp/Viant Workspace",
                defaultAgent = "workspace"
            )
        )
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
    fun selectedWorkspaceEndpointOption_resolvesStewardPreset() {
        val steward = workspaceEndpointOptions.first()

        assertEquals(
            steward,
            selectedWorkspaceEndpointOption("https://steward.agently.viantinc.com/v1/api/")
        )
    }

    @Test
    fun selectedWorkspaceEndpointOption_resolvesLocalhostPreset() {
        val localhost = workspaceEndpointOptions.first { it.value == "http://localhost:9191" }

        assertEquals(
            localhost,
            selectedWorkspaceEndpointOption("http://localhost:9191/v1/api/")
        )
    }

    @Test
    fun normalizeApiBaseUrl_removesApiSuffixesAndTrailingSlash() {
        assertEquals(
            "https://steward.agently.viantinc.com",
            normalizeApiBaseUrl(" https://steward.agently.viantinc.com/v1/api/ ")
        )
        assertEquals(
            "https://steward.agently.viantinc.com",
            normalizeApiBaseUrl("https://steward.agently.viantinc.com/v1")
        )
    }
}
