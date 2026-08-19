package com.viant.agently.android

import android.speech.SpeechRecognizer
import org.junit.Assert.assertEquals
import org.junit.Test

class ComposerMediaTest {
    @Test
    fun voiceTranscriptPopulatesBlankComposer() {
        assertEquals(
            "troubleshoot order 2674628",
            mergeVoiceTranscript("", " troubleshoot order 2674628 ")
        )
    }

    @Test
    fun voiceTranscriptPreservesExistingComposerText() {
        assertEquals(
            "Troubleshoot order 2674628\nand summarize the delivery blocker",
            mergeVoiceTranscript(
                "Troubleshoot order 2674628",
                "and summarize the delivery blocker"
            )
        )
    }

    @Test
    fun voiceTranscriptIsInsertedAtCursorAndPreservesSuffix() {
        assertEquals(
            VoiceTranscriptInsertion(
                text = "Troubleshoot order 2674628 and summarize delivery",
                cursorPosition = 26
            ),
            insertVoiceTranscript(
                existingQuery = "Troubleshoot  and summarize delivery",
                speechText = "order 2674628",
                cursorPosition = 13
            )
        )
    }

    @Test
    fun noMatchHasActionableMessage() {
        assertEquals(
            "I didn't catch that. Tap the microphone to try again.",
            voiceRecognitionErrorMessage(SpeechRecognizer.ERROR_NO_MATCH)
        )
    }
}
