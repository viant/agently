package com.viant.agently.android

import com.viant.agentlysdk.WorkspaceAgentInfo
import com.viant.agentlysdk.WorkspaceDefaults
import com.viant.agentlysdk.WorkspaceMetadata
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
            agents = listOf("chatter", "planner"),
            agentInfos = listOf(
                WorkspaceAgentInfo(id = "chatter", name = "Chatter"),
                WorkspaceAgentInfo(id = "planner", name = "Planner")
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
