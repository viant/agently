package com.viant.agently.android

import com.viant.agentlysdk.OAuthInitiateOutput
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.net.URI
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

internal const val AndroidOAuthRedirectURI = "agently-android://oauth/callback"

internal fun resolveOAuthInitiateUrl(output: OAuthInitiateOutput): String {
    return output.authURL?.takeIf { it.isNotBlank() }
        ?: output.authUrl?.takeIf { it.isNotBlank() }
        ?: error("OAuth initiate did not return an auth URL")
}

internal fun authUrlUsesRedirect(authUrl: String, expectedRedirectUri: String): Boolean {
    val expected = expectedRedirectUri.trim()
    if (expected.isEmpty()) return false
    return runCatching {
        queryParameter(authUrl, "redirect_uri")?.trim() == expected
    }.getOrDefault(false)
}

private fun queryParameter(url: String, name: String): String? {
    val query = URI(url).rawQuery ?: return null
    return query.split("&")
        .asSequence()
        .mapNotNull { item ->
            val index = item.indexOf("=")
            if (index < 0) return@mapNotNull null
            val key = decodeQueryComponent(item.substring(0, index))
            if (key != name) return@mapNotNull null
            decodeQueryComponent(item.substring(index + 1))
        }
        .firstOrNull()
}

private fun decodeQueryComponent(value: String): String {
    return URLDecoder.decode(value, StandardCharsets.UTF_8.name())
}

internal fun normalizeDeveloperSessionCredential(rawValue: String): String {
    var value = rawValue.trim()
    if (value.isEmpty()) return ""

    credentialFromJson(value)?.let { return it }

    value = stripSurroundingQuotes(value)
    if (value.startsWith("authorization:", ignoreCase = true)) {
        value = value.removePrefixIgnoringCase("authorization:").trim()
    }
    if (value.startsWith("cookie:", ignoreCase = true)) {
        value = value.removePrefixIgnoringCase("cookie:").trim()
    }
    if (value.startsWith("bearer ", ignoreCase = true)) {
        return value.removePrefixIgnoringCase("bearer ").trim()
    }

    listOf("agently_session", "sessionId", "sessionID", "session_id").forEach { cookieName ->
        cookieValue(cookieName, value)?.let { return it }
    }
    return stripSurroundingQuotes(value)
}

private fun credentialFromJson(value: String): String? {
    if (!value.startsWith("{")) return null
    val objectValue = runCatching {
        Json.parseToJsonElement(value).jsonObject
    }.getOrNull() ?: return null
    return listOf("sessionId", "sessionID", "token", "accessToken", "idToken")
        .firstNotNullOfOrNull { key ->
            objectValue[key]?.jsonPrimitive?.contentOrNull
                ?.trim()
                ?.takeIf { it.isNotEmpty() }
                ?.let(::stripSurroundingQuotes)
        }
}

private fun cookieValue(name: String, value: String): String? {
    val prefix = "$name="
    return value.split(";")
        .asSequence()
        .map { it.trim() }
        .firstOrNull { it.startsWith(prefix, ignoreCase = true) }
        ?.substring(prefix.length)
        ?.trim()
        ?.let(::stripSurroundingQuotes)
        ?.takeIf { it.isNotEmpty() }
}

private fun stripSurroundingQuotes(value: String): String {
    var result = value.trim()
    while (
        result.length >= 2 &&
        ((result.startsWith("\"") && result.endsWith("\"")) ||
            (result.startsWith("'") && result.endsWith("'")))
    ) {
        result = result.substring(1, result.length - 1).trim()
    }
    return result
}

private fun String.removePrefixIgnoringCase(prefix: String): String {
    return if (startsWith(prefix, ignoreCase = true)) substring(prefix.length) else this
}
