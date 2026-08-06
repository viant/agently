package com.viant.agently.android

import android.net.Uri
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import java.net.URI

internal enum class AuthState {
    Checking,
    Required,
    Ready
}

@Composable
internal fun AuthRequiredScreen(
    busy: Boolean,
    developerSessionEnabled: Boolean = false,
    onSignIn: () -> Unit,
    onOpenSettings: () -> Unit,
    onDeveloperSessionSignIn: (String) -> Unit = {}
) {
    var showDeveloperSession by remember { mutableStateOf(false) }
    var developerSessionDraft by remember { mutableStateOf("") }
    val canUseDeveloperSession = !busy && developerSessionDraft.trim().isNotBlank()

    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center
    ) {
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .widthIn(max = 760.dp)
                .wrapContentHeight()
        ) {
            BoxWithConstraints {
                val isCompactWidth = maxWidth < 520.dp
                Column(
                    modifier = Modifier.padding(24.dp),
                    verticalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(
                            "This workspace requires authorization.",
                            style = MaterialTheme.typography.titleMedium,
                            modifier = Modifier.weight(1f)
                        )
                        IconButton(onClick = onOpenSettings) {
                            Icon(
                                imageVector = Icons.Outlined.Settings,
                                contentDescription = "Workspace settings"
                            )
                        }
                    }
                    if (isCompactWidth) {
                        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                            Button(
                                onClick = onSignIn,
                                enabled = !busy,
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                Text("Sign in")
                            }
                        }
                    } else {
                        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                            Button(onClick = onSignIn, enabled = !busy) {
                                Text("Sign in")
                            }
                        }
                    }
                    if (developerSessionEnabled) {
                        TextButton(
                            onClick = { showDeveloperSession = !showDeveloperSession },
                            enabled = !busy
                        ) {
                            Text(if (showDeveloperSession) "Hide developer session sign-in" else "Use developer session")
                        }
                        if (showDeveloperSession) {
                            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                                OutlinedTextField(
                                    value = developerSessionDraft,
                                    onValueChange = { developerSessionDraft = it },
                                    label = { Text("Session ID or token") },
                                    trailingIcon = {
                                        TextButton(
                                            onClick = { onDeveloperSessionSignIn(developerSessionDraft) },
                                            enabled = canUseDeveloperSession
                                        ) {
                                            Text("Use")
                                        }
                                    },
                                    modifier = Modifier.fillMaxWidth(),
                                    singleLine = true,
                                    enabled = !busy,
                                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Done),
                                    keyboardActions = KeyboardActions(
                                        onDone = {
                                            if (canUseDeveloperSession) {
                                                onDeveloperSessionSignIn(developerSessionDraft)
                                            }
                                        }
                                    )
                                )
                                Button(
                                    onClick = { onDeveloperSessionSignIn(developerSessionDraft) },
                                    enabled = canUseDeveloperSession,
                                    modifier = if (isCompactWidth) {
                                        Modifier.fillMaxWidth()
                                    } else {
                                        Modifier.widthIn(min = 160.dp)
                                    }
                                ) {
                                    Text("Use session")
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
internal fun OAuthWebDialog(
    authUrl: String,
    callbackPrefix: String,
    onDismiss: () -> Unit,
    onCallback: (String, String) -> Unit
) {
    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var loadedAuthUrl by remember { mutableStateOf<String?>(null) }
    var pageStatus by remember { mutableStateOf("Opening sign-in page…") }
    var webError by remember { mutableStateOf<String?>(null) }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
    ) {
        AndroidView(
            modifier = Modifier.fillMaxSize(),
            factory = { viewContext ->
                WebView(viewContext).apply {
                    webViewRef = this
                    settings.javaScriptEnabled = true
                    settings.domStorageEnabled = true
                    settings.useWideViewPort = true
                    settings.loadWithOverviewMode = true
                    settings.textZoom = 100
                    webViewClient = object : WebViewClient() {
                        private fun intercept(url: String): Boolean {
                            if (!matchesOAuthCallbackUrl(url, callbackPrefix)) {
                                return false
                            }
                            val uri = Uri.parse(url)
                            val code = uri.getQueryParameter("code").orEmpty()
                            val state = uri.getQueryParameter("state").orEmpty()
                            if (code.isBlank() || state.isBlank()) {
                                return false
                            }
                            onCallback(code, state)
                            return true
                        }

                        private fun updatePageState(view: WebView?, url: String?) {
                            val target = url.orEmpty().ifBlank { view?.url.orEmpty() }
                            if (target.isBlank()) {
                                pageStatus = "Opening sign-in page…"
                                return
                            }
                            webError = null
                            pageStatus = when {
                                matchesOAuthCallbackUrl(target, callbackPrefix) ->
                                    "Finishing sign-in…"
                                else ->
                                    Uri.parse(target).host?.let { "Loading $it…" } ?: "Loading sign-in page…"
                            }
                        }

                        override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
                            return intercept(request?.url?.toString().orEmpty())
                        }

                        @Deprecated("Deprecated in Android API 24")
                        override fun shouldOverrideUrlLoading(view: WebView?, url: String?): Boolean {
                            return intercept(url.orEmpty())
                        }

                        override fun onPageFinished(view: WebView?, url: String?) {
                            super.onPageFinished(view, url)
                            updatePageState(view, url)
                        }

                        override fun onReceivedError(
                            view: WebView?,
                            request: WebResourceRequest?,
                            error: WebResourceError?
                        ) {
                            super.onReceivedError(view, request, error)
                            if (request?.isForMainFrame != true) {
                                return
                            }
                            val code = error?.errorCode ?: 0
                            val description = error?.description?.toString().orEmpty()
                            pageStatus = "Sign-in page failed to load."
                            webError = when {
                                code == -2 || description.contains("ERR_NAME_NOT_RESOLVED", ignoreCase = true) ->
                                    "The emulator could not resolve the login host. Reload or open the page in the system browser."
                                else ->
                                    "The identity provider sign-in page could not be loaded${if (description.isNotBlank()) ": $description" else "."}"
                            }
                        }
                    }
                    loadUrl(authUrl)
                    loadedAuthUrl = authUrl
                }
            },
            update = { webView ->
                webViewRef = webView
                if (loadedAuthUrl != authUrl) {
                    loadedAuthUrl = authUrl
                    pageStatus = "Opening sign-in page…"
                    webView.loadUrl(authUrl)
                }
            }
        )
        TextButton(
            onClick = onDismiss,
            modifier = Modifier
                .align(Alignment.TopEnd)
                .offset(x = (-8).dp, y = 8.dp)
        ) {
            Text("Close")
        }
        webError?.let { message ->
            Row(
                modifier = Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .padding(12.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text(
                    message,
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFFB42318),
                    modifier = Modifier.weight(1f)
                )
                TextButton(
                    onClick = {
                        webError = null
                        pageStatus = "Retrying sign-in page…"
                        webViewRef?.reload()
                    }
                ) {
                    Text("Reload")
                }
            }
        }
    }
}

internal fun matchesOAuthCallbackUrl(url: String, callbackPrefix: String): Boolean {
    val callback = callbackPrefix.trim()
    if (callback.isEmpty()) {
        return false
    }
    val uri = runCatching { URI(url) }.getOrNull() ?: return false
    return if (callback.contains("://")) {
        val callbackUri = runCatching { URI(callback) }.getOrNull() ?: return false
        uri.scheme.equals(callbackUri.scheme, ignoreCase = true) &&
            uri.host.equals(callbackUri.host, ignoreCase = true) &&
            uri.path.orEmpty() == callbackUri.path.orEmpty()
    } else {
        uri.path.orEmpty().endsWith(callback)
    }
}

internal fun normalizeAuthError(raw: String?): String? {
    val message = raw?.trim().orEmpty()
    if (message.isBlank()) {
        return null
    }
    val lowered = message.lowercase()
    return when {
        lowered.contains("left the composition") ||
            lowered.contains("job was cancelled") ||
            lowered.contains("job was canceled") ->
            null
        lowered == "timeout" ->
            "The sign-in request timed out. The Agently endpoint is reachable, but the upstream identity provider did not respond in time."
        lowered.contains("401") || lowered.contains("403") ->
            "Authentication required. Sign in to load the Agently workspace."
        lowered.contains("unable to reach app api") ||
            lowered.contains("failed to connect") ||
            lowered.contains("connection refused") ||
            lowered.contains("network is unreachable") ->
            "Agently could not reach the configured endpoint. Check the server and emulator connection, then try again."
        lowered.contains("oauth initiate did not return an auth url") ->
            "OAuth sign-in is available, but the server did not return a login URL."
        else -> message
    }
}
