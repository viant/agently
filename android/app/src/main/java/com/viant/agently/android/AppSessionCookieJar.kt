package com.viant.agently.android

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.HttpUrl.Companion.toHttpUrl
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.util.concurrent.TimeUnit
import java.util.concurrent.ConcurrentHashMap

internal class AppSessionCookieJar(context: Context? = null) : CookieJar {
    private val store = ConcurrentHashMap<String, MutableList<Cookie>>()
    private val persistentStore = context?.let { PersistentSessionCookieStore(it.applicationContext) }

    init {
        persistentStore?.load()?.forEach { stored ->
            putCookie(stored.cookie)
        }
    }

    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        if (cookies.isEmpty()) {
            return
        }
        cookies.forEach { incoming ->
            putCookie(incoming)
        }
        persist(url)
    }

    override fun loadForRequest(url: HttpUrl): List<Cookie> {
        val now = System.currentTimeMillis()
        var pruned = false
        val cookies = store.values.flatMap { bucket ->
            synchronized(bucket) {
                val removed = bucket.removeAll { it.expiresAt < now }
                if (removed) {
                    pruned = true
                }
                bucket.filter { cookie -> cookie.matches(url) }
            }
        }
        if (pruned) {
            persist(url)
        }
        return cookies
    }

    fun clear() {
        store.clear()
        persistentStore?.clear()
    }

    /** Installs a session produced by the local OOB flow into this app's jar. */
    fun installSession(baseUrl: String, sessionId: String): Boolean {
        val value = sessionId.trim()
        if (value.isEmpty()) return false
        val url = runCatching { baseUrl.trim().trimEnd('/').plus("/").toHttpUrl() }.getOrNull()
            ?: return false
        val builder = Cookie.Builder()
            .name("agently_session")
            .value(value)
            .hostOnlyDomain(url.host)
            .path("/")
            .httpOnly()
            .expiresAt(System.currentTimeMillis() + TimeUnit.DAYS.toMillis(7))
        if (url.isHttps) builder.secure()
        putCookie(builder.build())
        persist(url)
        return true
    }

    /** Returns matching cookie strings for seeding a same-origin OAuth WebView. */
    fun webViewCookies(baseUrl: String): List<String> {
        val url = runCatching { baseUrl.trim().trimEnd('/').plus("/").toHttpUrl() }.getOrNull()
            ?: return emptyList()
        return loadForRequest(url).map { it.toString() }
    }

    private fun putCookie(incoming: Cookie) {
        val key = incoming.domain
        val existing = store.computeIfAbsent(key) { mutableListOf() }
        synchronized(existing) {
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

    private fun persist(url: HttpUrl) {
        persistentStore?.save(snapshot(url))
    }

    private fun snapshot(url: HttpUrl): List<StoredSessionCookie> {
        val now = System.currentTimeMillis()
        return store.values.flatMap { bucket ->
            synchronized(bucket) {
                bucket.removeAll { it.expiresAt < now }
                bucket.map { cookie ->
                    StoredSessionCookie(
                        url = url.toString(),
                        setCookie = cookie.toString()
                    )
                }
            }
        }
    }
}

internal data class StoredSessionCookie(
    val url: String,
    val setCookie: String,
    val cookie: Cookie = Cookie.parse(url.toHttpUrl(), setCookie)
        ?: throw IllegalArgumentException("Invalid stored cookie")
)

internal fun encodeSessionCookies(cookies: List<StoredSessionCookie>): String {
    val payload = buildJsonArray {
        cookies.forEach { cookie ->
            add(
                buildJsonObject {
                    put("url", JsonPrimitive(cookie.url))
                    put("setCookie", JsonPrimitive(cookie.setCookie))
                }
            )
        }
    }
    return payload.toString()
}

internal fun decodeSessionCookies(raw: String): List<StoredSessionCookie> {
    if (raw.isBlank()) {
        return emptyList()
    }
    return runCatching {
        val payload = SessionCookieJson.parseToJsonElement(raw) as? JsonArray ?: return@runCatching emptyList()
        buildList {
            payload.forEach { element ->
                val item = element as? JsonObject ?: return@forEach
                val url = item["url"]?.jsonPrimitive?.contentOrNull?.trim().orEmpty()
                val setCookie = item["setCookie"]?.jsonPrimitive?.contentOrNull?.trim().orEmpty()
                if (url.isBlank() || setCookie.isBlank()) {
                    return@forEach
                }
                val stored = runCatching { StoredSessionCookie(url = url, setCookie = setCookie) }.getOrNull()
                if (stored != null && stored.cookie.expiresAt >= System.currentTimeMillis()) {
                    add(stored)
                }
            }
        }
    }.getOrDefault(emptyList())
}

private val SessionCookieJson = Json { ignoreUnknownKeys = true }

private class PersistentSessionCookieStore(context: Context) {
    private val prefs = EncryptedSharedPreferences.create(
        context,
        PREFS_NAME,
        MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build(),
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM
    )

    fun load(): List<StoredSessionCookie> {
        return decodeSessionCookies(prefs.getString(KEY_COOKIES, "").orEmpty())
    }

    fun save(cookies: List<StoredSessionCookie>) {
        prefs.edit()
            .putString(KEY_COOKIES, encodeSessionCookies(cookies))
            .apply()
    }

    fun clear() {
        prefs.edit().remove(KEY_COOKIES).apply()
    }

    companion object {
        private const val PREFS_NAME = "agently.auth.session"
        private const val KEY_COOKIES = "cookies"
    }
}

internal fun appSessionHttpClient(cookieJar: CookieJar = AppSessionCookieJar()): OkHttpClient {
    return appSessionHttpClientBuilder(cookieJar)
        .readTimeout(30, TimeUnit.SECONDS)
        .callTimeout(45, TimeUnit.SECONDS)
        .build()
}

internal fun appLongRunningHttpClient(cookieJar: CookieJar = AppSessionCookieJar()): OkHttpClient {
    return appSessionHttpClientBuilder(cookieJar)
        .readTimeout(0, TimeUnit.SECONDS)
        .callTimeout(0, TimeUnit.SECONDS)
        .build()
}

internal fun appStreamHttpClient(cookieJar: CookieJar = AppSessionCookieJar()): OkHttpClient {
    return appSessionHttpClientBuilder(cookieJar)
        .readTimeout(0, TimeUnit.SECONDS)
        .callTimeout(0, TimeUnit.SECONDS)
        .build()
}

private fun appSessionHttpClientBuilder(cookieJar: CookieJar): OkHttpClient.Builder {
    return OkHttpClient.Builder()
        .cookieJar(cookieJar)
        .connectTimeout(10, TimeUnit.SECONDS)
}
