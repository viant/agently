package com.viant.agently.android

import com.viant.agentlysdk.OAuthInitiateOutput
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
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

    @Test
    fun `authUrlUsesRedirect accepts native redirect and rejects web callback`() {
        assertTrue(
            authUrlUsesRedirect(
                "https://idp.viantinc.com/v1/api/oauth2/authorize?redirect_uri=agently-android%3A%2F%2Foauth%2Fcallback",
                AndroidOAuthRedirectURI
            )
        )
        assertFalse(
            authUrlUsesRedirect(
                "https://idp.viantinc.com/v1/api/oauth2/authorize?redirect_uri=https%3A%2F%2Fsteward.agently.viantinc.com%2Fv1%2Fapi%2Fauth%2Foauth%2Fcallback",
                AndroidOAuthRedirectURI
            )
        )
    }

    @Test
    fun `normalizeDeveloperSessionCredential accepts common paste formats`() {
        assertEquals(
            "sess-1",
            normalizeDeveloperSessionCredential("Cookie: agently_session=sess-1; Path=/")
        )
        assertEquals(
            "sess-json",
            normalizeDeveloperSessionCredential("""{"sessionId":"sess-json"}""")
        )
        assertEquals(
            "token-1",
            normalizeDeveloperSessionCredential("Authorization: Bearer token-1")
        )
        assertEquals(
            "bare-session",
            normalizeDeveloperSessionCredential("'bare-session'")
        )
    }
}
