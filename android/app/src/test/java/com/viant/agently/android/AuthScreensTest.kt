package com.viant.agently.android

import org.junit.Assert.assertNull
import org.junit.Test

class AuthScreensTest {

    @Test
    fun normalizeAuthError_hidesBenignCancellationMessages() {
        assertNull(normalizeAuthError("left the composition"))
        assertNull(normalizeAuthError("Job was cancelled"))
    }

}
