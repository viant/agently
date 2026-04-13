package com.viant.agently.android

import android.content.Intent
import android.net.Uri
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import com.viant.agentlysdk.AuthProvider
import com.viant.agentlysdk.AuthUser
import org.json.JSONObject

internal enum class AuthState {
    Checking,
    Required,
    Ready
}

internal enum class AuthRequirementMode {
    SignInRequired,
    ConnectionProblem
}

@Composable
internal fun AuthRequiredScreen(
    busy: Boolean,
    error: String?,
    providers: List<AuthProvider>,
    user: AuthUser?,
    savedLoginConfig: SavedLoginConfig,
    onSignIn: () -> Unit,
    onManageSavedLogin: () -> Unit,
    onOpenSettings: () -> Unit,
    onRetry: () -> Unit
) {
    val providerLabel = resolveProviderLabel(providers)
    val normalizedError = normalizeAuthError(error)
    val authMode = resolveAuthRequirementMode(normalizedError)
    val authTitle = resolveAuthRequiredTitle(authMode)
    val authDescription = resolveAuthRequiredDescription(authMode, normalizedError)
    val prefersRetry = authMode == AuthRequirementMode.ConnectionProblem

    Box(
        modifier = Modifier.fillMaxSize(),
        contentAlignment = Alignment.Center
    ) {
        Card(
            modifier = Modifier
                .fillMaxWidth(0.72f)
                .widthIn(max = 760.dp)
        ) {
            Column(
                modifier = Modifier.padding(24.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp)
            ) {
                Text(authTitle, style = MaterialTheme.typography.headlineSmall)
                Text(
                    authDescription,
                    style = MaterialTheme.typography.bodyMedium,
                    color = Color(0xFF667085)
                )
                if (savedLoginConfig.hasStoredIdpCredential) {
                    Text(
                        "Saved sign-in helper credentials are available for $providerLabel. They can autofill the OAuth login page, but the sign-in flow still completes through OAuth.",
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF475467)
                    )
                }
                user?.displayName?.takeIf { it.isNotBlank() }?.let {
                    Text("Current user: $it", style = MaterialTheme.typography.bodySmall)
                }
                normalizedError?.takeIf { it.isNotBlank() }?.let {
                    Text(
                        it,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFFB42318)
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Button(onClick = if (prefersRetry) onRetry else onSignIn, enabled = !busy) {
                        Text(
                            when {
                                busy && prefersRetry -> "Checking…"
                                busy -> "Starting…"
                                prefersRetry -> "Retry connection"
                                else -> "Sign in with $providerLabel"
                            }
                        )
                    }
                    if (!prefersRetry) {
                        OutlinedButton(onClick = onRetry, enabled = !busy) {
                            Text("Retry")
                        }
                    }
                }
                TextButton(onClick = onManageSavedLogin, enabled = !busy) {
                    Text(
                        if (savedLoginConfig.hasStoredIdpCredential) {
                            "Manage saved sign-in helper"
                        } else {
                            "Set up saved sign-in helper"
                        }
                    )
                }
                TextButton(onClick = onOpenSettings, enabled = !busy) {
                    Text("Open client settings")
                }
            }
        }
    }
}

internal fun resolveAuthRequirementMode(error: String?): AuthRequirementMode {
    val message = error.orEmpty().lowercase()
    return when {
        "could not reach the agently server" in message -> AuthRequirementMode.ConnectionProblem
        "could not reach the configured endpoint" in message -> AuthRequirementMode.ConnectionProblem
        "timed out" in message -> AuthRequirementMode.ConnectionProblem
        else -> AuthRequirementMode.SignInRequired
    }
}

internal fun resolveAuthRequiredTitle(mode: AuthRequirementMode): String =
    when (mode) {
        AuthRequirementMode.ConnectionProblem -> "Connection problem"
        AuthRequirementMode.SignInRequired -> "Sign in required"
    }

internal fun resolveAuthRequiredDescription(
    mode: AuthRequirementMode,
    error: String?
): String {
    val message = error.orEmpty().lowercase()
    return when (mode) {
        AuthRequirementMode.ConnectionProblem ->
            when {
                "could not reach the agently server" in message ->
                    "Agently cannot load conversations, approvals, or Forge content until the configured Agently endpoint is reachable from the emulator."
                "could not reach the configured endpoint" in message ->
                    "Agently cannot load conversations, approvals, or Forge content until the configured Agently endpoint is reachable from the emulator."
                else ->
                    "Agently reached the endpoint, but the sign-in flow timed out before the workspace could finish loading."
            }
        AuthRequirementMode.SignInRequired ->
            "This Agently endpoint requires OAuth before conversations, approvals, and Forge-rendered content can load."
    }
}

@Composable
internal fun SavedLoginConfigDialog(
    initial: SavedLoginConfig,
    onDismiss: () -> Unit,
    onSave: (SavedLoginConfig) -> Unit,
    onClear: () -> Unit
) {
    var username by remember(initial.username) { mutableStateOf(initial.username) }
    var password by remember(initial.password) { mutableStateOf(initial.password) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Saved Sign-In Helper") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                Text(
                    "Store optional username and password encrypted on-device so Agently can autofill the OAuth web login when the identity provider page appears.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Color(0xFF667085)
                )
                OutlinedTextField(
                    value = username,
                    onValueChange = { username = it },
                    label = { Text("IDP username") },
                    modifier = Modifier.fillMaxWidth()
                )
                OutlinedTextField(
                    value = password,
                    onValueChange = { password = it },
                    label = { Text("IDP password") },
                    visualTransformation = PasswordVisualTransformation(),
                    modifier = Modifier.fillMaxWidth()
                )
            }
        },
        confirmButton = {
            Button(
                onClick = {
                    onSave(
                        SavedLoginConfig(
                            username = username.trim(),
                            password = password
                        )
                    )
                }
            ) {
                Text("Save")
            }
        },
        dismissButton = {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(onClick = onClear) {
                    Text("Clear")
                }
                TextButton(onClick = onDismiss) {
                    Text("Cancel")
                }
            }
        }
    )
}

@Composable
internal fun OAuthWebDialog(
    authUrl: String,
    callbackPrefix: String,
    savedLoginConfig: SavedLoginConfig,
    onDismiss: () -> Unit,
    onCallback: (String, String) -> Unit
) {
    val context = LocalContext.current
    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var pageStatus by remember { mutableStateOf("Opening sign-in page…") }
    var autoFillTarget by remember { mutableStateOf<String?>(null) }
    var webError by remember { mutableStateOf<String?>(null) }

    Card(
        modifier = Modifier
            .fillMaxSize()
            .padding(12.dp)
    ) {
        Column(modifier = Modifier.fillMaxSize()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp, vertical = 12.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text("OAuth Sign In", style = MaterialTheme.typography.titleLarge)
                    Text(
                        pageStatus,
                        style = MaterialTheme.typography.bodySmall,
                        color = Color(0xFF667085)
                    )
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    TextButton(
                        onClick = {
                            runCatching {
                                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(authUrl)))
                            }
                        }
                    ) {
                        Text("Open in browser")
                    }
                    if (savedLoginConfig.hasStoredIdpCredential) {
                        TextButton(
                            onClick = {
                                webViewRef?.let {
                                    injectIdpCredentials(it, savedLoginConfig.username, savedLoginConfig.password)
                                }
                            }
                        ) {
                            Text("Use saved helper")
                        }
                    }
                    TextButton(onClick = onDismiss) {
                        Text("Close")
                    }
                }
            }
            webError?.let { message ->
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 16.dp),
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
                            autoFillTarget = null
                            webViewRef?.reload()
                        }
                    ) {
                        Text("Reload")
                    }
                }
            }
            AndroidView(
                modifier = Modifier.fillMaxSize(),
                factory = { viewContext ->
                    WebView(viewContext).apply {
                        webViewRef = this
                        settings.javaScriptEnabled = true
                        settings.domStorageEnabled = true
                        webViewClient = object : WebViewClient() {
                            private fun intercept(url: String): Boolean {
                                val uri = Uri.parse(url)
                                if (!uri.path.orEmpty().endsWith(callbackPrefix)) {
                                    return false
                                }
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
                                    target.contains("/oauth/callback") ->
                                        "Finishing sign-in…"
                                    isLikelyIdpPage(target) ->
                                        if (savedLoginConfig.hasStoredIdpCredential) {
                                            "Identity provider sign-in page ready. Saved helper credentials can be used here."
                                        } else {
                                            "Identity provider sign-in page ready."
                                        }
                                    else ->
                                        Uri.parse(target).host?.let { "Loading $it…" } ?: "Loading sign-in page…"
                                }
                                if (savedLoginConfig.hasStoredIdpCredential &&
                                    isLikelyIdpPage(target) &&
                                    autoFillTarget != target
                                ) {
                                    autoFillTarget = target
                                    val targetView = view ?: return
                                    targetView.postDelayed(
                                        {
                                            injectIdpCredentials(targetView, savedLoginConfig.username, savedLoginConfig.password)
                                        },
                                        350
                                    )
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
                    }
                },
                update = { webView ->
                    webViewRef = webView
                    if (webView.url != authUrl) {
                        pageStatus = "Opening sign-in page…"
                        autoFillTarget = null
                        webView.loadUrl(authUrl)
                    }
                }
            )
        }
    }
}

private fun injectIdpCredentials(webView: WebView, username: String, password: String) {
    val escapedUsername = JSONObject.quote(username)
    val escapedPassword = JSONObject.quote(password)
    val script = """
        (function() {
          var username = $escapedUsername;
          var password = $escapedPassword;
          var userInput = document.querySelector('input[type="text"], input[name*="user"], input[id*="user"]');
          var passInput = document.querySelector('input[type="password"]');
          if (userInput) {
            userInput.focus();
            userInput.value = username;
            userInput.dispatchEvent(new Event('input', { bubbles: true }));
            userInput.dispatchEvent(new Event('change', { bubbles: true }));
          }
          if (passInput) {
            passInput.focus();
            passInput.value = password;
            passInput.dispatchEvent(new Event('input', { bubbles: true }));
            passInput.dispatchEvent(new Event('change', { bubbles: true }));
          }
          var button = Array.from(document.querySelectorAll('button')).find(function(item) {
            return /log\s*in|sign\s*in/i.test((item.innerText || '').trim());
          });
          if (button) {
            button.click();
          } else if (passInput && passInput.form) {
            passInput.form.submit();
          }
        })();
    """.trimIndent()
    webView.evaluateJavascript(script, null)
}

private fun isLikelyIdpPage(url: String): Boolean {
    val lowered = url.lowercase()
    return lowered.contains("/idp/") ||
        lowered.contains("/login") ||
        lowered.contains("authorize")
}

private fun resolveProviderLabel(
    providers: List<AuthProvider>
): String {
    providers.firstOrNull { provider ->
        val type = provider.type.trim().lowercase()
        type == "oauth" || type == "bff" || type == "oidc" || type == "jwt"
    }?.label?.takeIf { it.isNotBlank() }?.let { return it }

    return "Identity Provider"
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
