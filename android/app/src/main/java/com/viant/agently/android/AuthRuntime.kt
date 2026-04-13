package com.viant.agently.android

import com.viant.agentlysdk.OAuthInitiateOutput

internal fun resolveOAuthInitiateUrl(output: OAuthInitiateOutput): String {
    return output.authURL?.takeIf { it.isNotBlank() }
        ?: output.authUrl?.takeIf { it.isNotBlank() }
        ?: error("OAuth initiate did not return an auth URL")
}
