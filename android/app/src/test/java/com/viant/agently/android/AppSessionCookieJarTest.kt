package com.viant.agently.android

import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AppSessionCookieJarTest {
    @Test
    fun encodedSessionCookiesRoundTripThroughOkHttpParser() {
        val url = "https://steward.agently.viantinc.com/v1/auth/mobile/callback"
        val stored = StoredSessionCookie(
            url = url,
            setCookie = "agently_session=session-123; Path=/; Expires=Fri, 01 Jun 2035 00:00:00 GMT; HttpOnly; Secure"
        )

        val decoded = decodeSessionCookies(encodeSessionCookies(listOf(stored)))

        assertEquals(1, decoded.size)
        assertEquals("agently_session", decoded.single().cookie.name)
        assertEquals("session-123", decoded.single().cookie.value)
        assertTrue(decoded.single().cookie.matches("https://steward.agently.viantinc.com/v1/me".toHttpUrl()))
    }

    @Test
    fun decodeSessionCookiesIgnoresExpiredCookies() {
        val raw = encodeSessionCookies(
            listOf(
                StoredSessionCookie(
                    url = "https://steward.agently.viantinc.com/",
                    setCookie = "agently_session=old; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT; HttpOnly; Secure"
                )
            )
        )

        assertTrue(decodeSessionCookies(raw).isEmpty())
    }
}
