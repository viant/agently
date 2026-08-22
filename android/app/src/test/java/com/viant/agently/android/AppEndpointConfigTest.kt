package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Test

class AppEndpointConfigTest {

    @Test
    fun `public Steward remains the default workspace`() {
        assertEquals(
            "https://steward.agently.viantinc.com",
            workspaceEndpointOptions.first().value
        )
    }
}
