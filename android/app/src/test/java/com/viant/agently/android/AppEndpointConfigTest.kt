package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AppEndpointConfigTest {

    @Test
    fun `buildApiCandidates prefers host alias by default`() {
        val candidates = buildApiCandidates("")

        assertEquals("http://10.0.2.2:9191", candidates.first())
        assertTrue(candidates.contains("http://10.0.3.2:9191"))
    }

    @Test
    fun `buildApiCandidates keeps emulator loopback available for localhost config`() {
        val candidates = buildApiCandidates("http://localhost:9191")

        assertEquals("http://localhost:9191", candidates.first())
        assertTrue(candidates.contains("http://10.0.2.2:9191"))
        assertTrue(candidates.contains("http://10.0.3.2:9191"))
    }

    @Test
    fun `mergeApiCandidates keeps current base url first and removes duplicates`() {
        val candidates = mergeApiCandidates(
            currentBaseUrl = "http://10.0.2.2:9191",
            configuredCandidates = listOf(
                "http://10.0.2.2:9191",
                "http://localhost:9191"
            )
        )

        assertEquals("http://10.0.2.2:9191", candidates.first())
        assertEquals(1, candidates.count { it == "http://10.0.2.2:9191" })
        assertEquals(1, candidates.count { it == "http://10.0.3.2:9191" })
        assertTrue(candidates.contains("http://localhost:9191"))
        assertFalse(candidates.contains("http://127.0.0.1:9191"))
    }
}
