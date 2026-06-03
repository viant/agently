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
            defaultAgent = "steward",
            agentInfos = listOf(
                WorkspaceAgentInfo(id = "steward", name = "Steward")
            )
        )

        assertEquals("Steward", resolveSelectedAgentLabel("", metadata))
    }

    @Test
    fun workspaceStarterTasks_readsSelectedAgentTasks() {
        val metadata = WorkspaceMetadata(
            defaultAgent = "steward",
            agentInfos = listOf(
                WorkspaceAgentInfo(
                    id = "steward",
                    name = "Steward",
                    starterTasks = listOf(
                        StarterTask(
                            id = "forecast",
                            title = "Open forecasting",
                            prompt = "Open forecasting",
                            description = "Launch the forecasting workspace."
                        )
                    )
                )
            )
        )

        assertEquals(1, workspaceStarterTasks("", metadata).size)
        assertEquals("forecast", workspaceStarterTasks("", metadata).first().id)
    }

    @Test
    fun resolveWorkspaceBrandTitle_stripsViantPrefixWhenLogoAlreadyShowsIt() {
        assertEquals(
            "Steward",
            resolveWorkspaceBrandTitle(
                workspaceRoot = "/tmp/Viant Steward",
                defaultAgent = "steward"
            )
        )
    }

    @Test
    fun buildSettingsApplyTransition_trimsAgentAndResolvesBlankBaseUrl() {
        val transition = buildSettingsApplyTransition(
            configuredBaseUrl = "http://configured",
            currentBaseUrl = "http://configured",
            nextBaseUrl = "   ",
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
}
