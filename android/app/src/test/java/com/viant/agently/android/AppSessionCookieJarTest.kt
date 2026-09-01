package com.viant.agently.android

import okhttp3.HttpUrl.Companion.toHttpUrl
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetSocketAddress
import java.net.Proxy

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

    @Test
    fun localOobSessionCanBeInstalledAsHostedWorkspaceCookie() {
        val jar = AppSessionCookieJar()

        assertTrue(
            jar.installSession(
                "https://steward.agently.viantinc.com/",
                "f78ae791-2a4d-4961-8d00-session"
            )
        )

        val cookies = jar.loadForRequest(
            "https://steward.agently.viantinc.com/v1/api/auth/me".toHttpUrl()
        )
        assertEquals(1, cookies.size)
        assertEquals("agently_session", cookies.single().name)
        assertEquals("f78ae791-2a4d-4961-8d00-session", cookies.single().value)
        assertTrue(cookies.single().secure)
        assertTrue(cookies.single().httpOnly)
        assertTrue(jar.webViewCookies("https://steward.agently.viantinc.com").single().startsWith("agently_session="))
    }

    @Test
    fun configuredHttpProxyIsAppliedToEveryWorkspaceClient() {
        val proxyUrl = "http://192.168.118.48:8888"
        val expected = resolveAppHttpProxy(proxyUrl)

        assertEquals(Proxy.Type.HTTP, expected?.type())
        val address = expected?.address() as InetSocketAddress
        assertEquals("192.168.118.48", address.hostString)
        assertEquals(8888, address.port)
        assertTrue(address.isUnresolved)

        listOf(
            appSessionHttpClient(proxyUrl = proxyUrl),
            appLongRunningHttpClient(proxyUrl = proxyUrl),
            appStreamHttpClient(proxyUrl = proxyUrl)
        ).forEach { client ->
            assertEquals(expected, client.proxy)
        }
    }

    @Test
    fun invalidHttpProxyIsIgnored() {
        assertNull(resolveAppHttpProxy(""))
        assertNull(resolveAppHttpProxy("socks://127.0.0.1:8888"))
        assertNull(resolveAppHttpProxy("http://missing-port"))
    }
}
