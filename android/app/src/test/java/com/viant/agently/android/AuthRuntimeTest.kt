package com.viant.agently.android

import com.viant.agentlysdk.OAuthInitiateOutput
import org.junit.Assert.assertEquals
import org.junit.Test

class AuthRuntimeTest {

    @Test
    fun `resolveOAuthInitiateUrl prefers authURL but falls back to authUrl`() {
        assertEquals(
            "https://example.com/auth",
            resolveOAuthInitiateUrl(
                OAuthInitiateOutput(authURL = "https://example.com/auth")
            )
        )
        assertEquals(
            "https://example.com/fallback",
            resolveOAuthInitiateUrl(
                OAuthInitiateOutput(authUrl = "https://example.com/fallback")
            )
        )
    }
}
