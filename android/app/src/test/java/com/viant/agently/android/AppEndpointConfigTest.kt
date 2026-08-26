package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Test

class AppEndpointConfigTest {

    @Test
    fun `workspace presets come only from build configuration`() {
        val declared = parseWorkspaceEndpointOptions(BuildConfig.WORKSPACE_ENDPOINTS_JSON)
        val buildOption = listOfNotNull(workspaceEndpointOption(BuildConfig.APP_API_BASE_URL))
        assertEquals(
            mergeWorkspaceEndpointOptions(declared, buildOption),
            workspaceEndpointOptions
        )
    }
}
