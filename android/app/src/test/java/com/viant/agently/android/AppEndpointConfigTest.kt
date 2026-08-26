package com.viant.agently.android

import org.junit.Assert.assertTrue
import org.junit.Test

class AppEndpointConfigTest {

    @Test
    fun `workspace presets come only from build configuration`() {
        assertTrue(workspaceEndpointOptions.isEmpty())
    }
}
