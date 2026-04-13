package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AuthScreensTest {

    @Test
    fun normalizeAuthError_hidesBenignCancellationMessages() {
        assertNull(normalizeAuthError("left the composition"))
        assertNull(normalizeAuthError("Job was cancelled"))
    }

    @Test
    fun normalizeAuthError_rewritesConnectionFailures() {
        assertEquals(
            "Agently could not reach the configured endpoint. Check the server and emulator connection, then try again.",
            normalizeAuthError("failed to connect to localhost")
        )
    }

    @Test
    fun resolveAuthRequirementMode_usesConnectionProblemForServerOutage() {
        assertEquals(
            AuthRequirementMode.ConnectionProblem,
            resolveAuthRequirementMode(
                "Agently could not reach the configured endpoint. Check the server and emulator connection, then try again."
            )
        )
    }

    @Test
    fun resolveAuthRequiredTitle_usesConnectionProblemForServerOutage() {
        assertEquals(
            "Connection problem",
            resolveAuthRequiredTitle(AuthRequirementMode.ConnectionProblem)
        )
    }

    @Test
    fun resolveAuthRequiredDescription_usesServerReachabilityCopyForServerOutage() {
        assertEquals(
            "Agently cannot load conversations, approvals, or Forge content until the configured Agently endpoint is reachable from the emulator.",
            resolveAuthRequiredDescription(
                AuthRequirementMode.ConnectionProblem,
                "Agently could not reach the configured endpoint. Check the server and emulator connection, then try again."
            )
        )
    }

    @Test
    fun resolveAuthRequiredTitle_defaultsToSignInRequiredForAuthFailures() {
        assertEquals(
            "Sign in required",
            resolveAuthRequiredTitle(AuthRequirementMode.SignInRequired)
        )
    }

    @Test
    fun resolveAuthRequiredDescription_usesTimeoutCopyForTimeouts() {
        assertEquals(
            "Agently reached the endpoint, but the sign-in flow timed out before the workspace could finish loading.",
            resolveAuthRequiredDescription(
                AuthRequirementMode.ConnectionProblem,
                "The sign-in request timed out. The Agently endpoint is reachable, but the upstream identity provider did not respond in time."
            )
        )
    }

    @Test
    fun resolveAuthRequirementMode_defaultsToSignInRequiredForAuthFailure() {
        assertEquals(
            AuthRequirementMode.SignInRequired,
            resolveAuthRequirementMode("Authentication required. Sign in to load the Agently workspace.")
        )
    }
}
