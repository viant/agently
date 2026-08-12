package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Test

class TranscriptScreenTest {
    @Test
    fun `transcript window keeps the newest bounded batch`() {
        assertEquals(60, transcriptWindowStart(totalItemCount = 100, visibleItemCount = 40))
    }

    @Test
    fun `transcript window exposes every item when the batch covers history`() {
        assertEquals(0, transcriptWindowStart(totalItemCount = 32, visibleItemCount = 40))
        assertEquals(0, transcriptWindowStart(totalItemCount = 80, visibleItemCount = 80))
    }

    @Test
    fun `transcript window rejects negative visible counts safely`() {
        assertEquals(12, transcriptWindowStart(totalItemCount = 12, visibleItemCount = -1))
    }
}
