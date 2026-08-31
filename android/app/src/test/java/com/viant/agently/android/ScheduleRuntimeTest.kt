package com.viant.agently.android

import org.junit.Assert.assertEquals
import org.junit.Test

class ScheduleRuntimeTest {
    @Test
    fun `calendar form derives cron metadata`() {
        val form = normalizeScheduleForm(
            mapOf(
                "scheduleEditorKind" to "calendar",
                "calendarPattern" to "once",
                "calendarTime" to "09:30 AM",
                "weekdays" to listOf("mon", "wed", "fri")
            )
        )

        assertEquals("cron", form["scheduleType"])
        assertEquals("30 9 * * 1,3,5", form["cronExpr"])
        assertEquals(null, form["intervalSeconds"])
    }

    @Test
    fun `elapsed form derives interval seconds`() {
        val form = normalizeScheduleForm(
            mapOf(
                "scheduleEditorKind" to "elapsed",
                "elapsedIntervalValue" to 2,
                "elapsedIntervalUnit" to "days"
            )
        )

        assertEquals("interval", form["scheduleType"])
        assertEquals(172800, form["intervalSeconds"])
        assertEquals("Every 2 days", form["scheduleSummary"])
    }
}
