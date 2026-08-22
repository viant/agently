package com.viant.agently.android

import android.net.Uri
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.BorderStroke
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
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.wrapContentHeight
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
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
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import java.net.URI

internal enum class AuthState {
    Checking,
    Unavailable,
    Required,
    Ready
}

@Composable
internal fun WorkspaceUnavailableScreen(
    error: String?,
    onRetry: () -> Unit,
    onOpenSettings: () -> Unit
) {
    Box(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .padding(horizontal = 20.dp, vertical = 28.dp),
        contentAlignment = Alignment.Center
    ) {
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .widthIn(max = 760.dp)
                .wrapContentHeight(),
            shape = RoundedCornerShape(32.dp),
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.error.copy(alpha = 0.35f)),
            colors = CardDefaults.cardColors(containerColor = Color(0xFFFFF7F7)),
            elevation = CardDefaults.cardElevation(defaultElevation = 6.dp)
        ) {
            Column(
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 26.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                Text("Can't connect to workspace", style = MaterialTheme.typography.titleMedium)
                Text(
                    error ?: "Check the device internet or VPN connection, then try again.",
                    style = MaterialTheme.typography.bodyMedium
                )
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Button(onClick = onRetry) { Text("Try again") }
                    TextButton(onClick = onOpenSettings) { Text("Workspace settings") }
                }
            }
        }
    }
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
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
            .padding(horizontal = 20.dp, vertical = 28.dp),
        contentAlignment = Alignment.Center
    ) {
        Card(
            modifier = Modifier
                .fillMaxWidth()
                .widthIn(max = 760.dp)
                .wrapContentHeight()
                .graphicsLayer {
                    shadowElevation = 10.dp.toPx()
                    shape = RoundedCornerShape(32.dp)
                    clip = false
                },
            shape = RoundedCornerShape(32.dp),
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant.copy(alpha = 0.55f)),
            colors = CardDefaults.cardColors(containerColor = Color(0xFFF5F1F7)),
            elevation = CardDefaults.cardElevation(defaultElevation = 6.dp)
        ) {
            BoxWithConstraints {
                val isCompactWidth = maxWidth < 520.dp
                Column(
                    modifier = Modifier.padding(horizontal = 24.dp, vertical = 26.dp),
                    verticalArrangement = Arrangement.spacedBy(16.dp)
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
    var webError by remember { mutableStateOf<String?>(null) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .statusBarsPadding()
            .navigationBarsPadding()
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(horizontal = 8.dp, vertical = 2.dp),
            horizontalArrangement = Arrangement.End,
            verticalAlignment = Alignment.CenterVertically
        ) {
            TextButton(onClick = onDismiss) {
                Text("Close")
            }
        }
        Box(modifier = Modifier.weight(1f)) {
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
                                    return
                                }
                                webError = null
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
                                // Some identity-provider pages autofocus the username field
                                // after layout, which makes WebView scroll the rounded form's
                                // heading and top corners above the visible viewport. Start each
                                // completed navigation at the document top; delayed passes cover
                                // focus scripts that run just after `load` without fighting later
                                // user-initiated scrolling.
                                view?.clearFocus()
                                view?.scrollTo(0, 0)
                                resetOAuthDocumentScroll(view)
                                view?.post { resetOAuthDocumentScroll(view) }
                                view?.postDelayed({ resetOAuthDocumentScroll(view) }, 300)
                                view?.postDelayed({ resetOAuthDocumentScroll(view) }, 900)
                                view?.postDelayed({ resetOAuthDocumentScroll(view) }, 1_800)
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
                        webView.loadUrl(authUrl)
                    }
                }
            )
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
                            webViewRef?.reload()
                        }
                    ) {
                        Text("Reload")
                    }
                }
            }
        }
    }
}

private fun resetOAuthDocumentScroll(webView: WebView?) {
    webView ?: return
    webView.scrollTo(0, 0)
    webView.evaluateJavascript(
        """
        (() => {
          const focused = document.activeElement;
          if (focused && typeof focused.blur === 'function') focused.blur();
          // Android WebView can resolve an identity page's `body { height:
          // 100vh }` to 0px while the viewport is embedded in Compose. A
          // centered login card is then positioned half above the viewport.
          // Correct only that broken zero-height case; leave ordinary provider
          // layouts untouched.
          if (document.body && document.body.clientHeight <= 1) {
            document.documentElement.style.minHeight = '100%';
            document.body.style.height = 'auto';
            document.body.style.minHeight = 'calc(100vh - 16px)';
            document.body.style.boxSizing = 'border-box';
          }
          if (document.documentElement) document.documentElement.scrollTop = 0;
          if (document.body) document.body.scrollTop = 0;
          window.scrollTo(0, 0);
        })();
        """.trimIndent(),
        null
    )
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
        lowered == "timeout" || lowered.contains("timed out") ->
            "The workspace did not respond. Check this device's internet or VPN connection, then try again."
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
