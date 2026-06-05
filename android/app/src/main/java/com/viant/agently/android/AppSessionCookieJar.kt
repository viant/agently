package com.viant.agently.android

import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import java.util.concurrent.TimeUnit
import java.util.concurrent.ConcurrentHashMap

internal class AppSessionCookieJar : CookieJar {
    private val store = ConcurrentHashMap<String, MutableList<Cookie>>()

    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        if (cookies.isEmpty()) {
            return
        }
        val key = url.host
        val existing = store.computeIfAbsent(key) { mutableListOf() }
        synchronized(existing) {
            cookies.forEach { incoming ->
                existing.removeAll { current ->
                    current.name == incoming.name &&
                        current.domain == incoming.domain &&
                        current.path == incoming.path
                }
                if (incoming.expiresAt >= System.currentTimeMillis()) {
                    existing += incoming
                }
            }
        }
    }

    override fun loadForRequest(url: HttpUrl): List<Cookie> {
        val now = System.currentTimeMillis()
        return store.values.flatMap { bucket ->
            synchronized(bucket) {
                bucket.removeAll { it.expiresAt < now }
                bucket.filter { cookie -> cookie.matches(url) }
            }
        }
    }
}

internal fun appSessionHttpClient(cookieJar: CookieJar = AppSessionCookieJar()): OkHttpClient {
    return OkHttpClient.Builder()
        .cookieJar(cookieJar)
        .readTimeout(0, TimeUnit.MILLISECONDS)
        .callTimeout(0, TimeUnit.MILLISECONDS)
        .build()
}
