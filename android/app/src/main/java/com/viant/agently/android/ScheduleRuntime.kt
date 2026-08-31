package com.viant.agently.android

import com.viant.agentlysdk.AgentlyClient
import com.viant.agentlysdk.Schedule
import com.viant.forgeandroid.runtime.DataSourceContext
import com.viant.forgeandroid.runtime.ExecutionArgs
import com.viant.forgeandroid.runtime.ForgeRuntime
import com.viant.forgeandroid.runtime.ItemDef
import com.viant.forgeandroid.runtime.SelectionState
import com.viant.forgeandroid.runtime.valueKey
import java.time.OffsetDateTime
import java.util.UUID

private const val DEFAULT_CALENDAR_TIME = "09:00 AM"
private const val DEFAULT_TIMEOUT_SECONDS = 300
private val allWeekdays = listOf("mon", "tue", "wed", "thu", "fri", "sat", "sun")
private val weekdayCron = mapOf("sun" to 0, "mon" to 1, "tue" to 2, "wed" to 3, "thu" to 4, "fri" to 5, "sat" to 6)

internal data class ScheduleHistoryFilter(
    val scheduleId: String,
    val scheduleName: String
)

internal fun registerScheduleHandlers(
    runtime: ForgeRuntime,
    client: AgentlyClient,
    onOpenScheduleHistory: (ScheduleHistoryFilter) -> Unit
) {
    runtime.registerHandler("schedule.noListSelection") { args -> !args.hasScheduleSelection() }
    runtime.registerHandler("schedule.hasListSelection") { args -> args.hasScheduleSelection() }
    runtime.registerHandler("schedule.addNewSchedule") { args ->
        args.context?.apply {
            resetSelection()
            setForm(normalizeScheduleForm(emptyMap()))
        }
        true
    }
    runtime.registerHandler("schedule.editSelected") { args ->
        val context = args.context ?: return@registerHandler false
        val selected = context.peekSelection().selected ?: return@registerHandler false
        if (selected["id"]?.toString().isNullOrBlank()) return@registerHandler false
        val normalized = normalizeScheduleForm(selected)
        context.setSelection(context.peekSelection().copy(selected = normalized))
        context.setForm(normalized)
        true
    }
    runtime.registerHandler("schedule.backToList") { true }
    runtime.registerHandler("schedule.syncScheduleFields") { args ->
        val context = args.context ?: return@registerHandler false
        val item = args.args["item"] as? ItemDef
        val key = item?.valueKey()
        if (!key.isNullOrBlank() && args.args.containsKey("value")) {
            context.setFormField(key, args.args["value"])
        }
        context.setForm(normalizeScheduleForm(context.peekForm()))
        true
    }
    runtime.registerHandler("schedule.showIfCalendar") { args -> args.editorKind() == "calendar" }
    runtime.registerHandler("schedule.showIfCalendarEvery") { args ->
        args.editorKind() == "calendar" && args.context?.peekForm()?.get("calendarPattern") == "every"
    }
    runtime.registerHandler("schedule.showIfElapsed") { args -> args.editorKind() == "elapsed" }
    runtime.registerHandler("schedule.showIfAdvanced") { args -> args.editorKind() == "advanced" }
    runtime.registerHandler("schedule.showIfEdit") { args ->
        !args.context?.peekForm()?.get("id")?.toString().isNullOrBlank()
    }
    runtime.registerHandler("schedule.hideField") { false }
    runtime.registerHandler("schedule.onSelectSchedule") { args ->
        val context = args.context ?: return@registerHandler false
        val selected = context.peekSelection().selected ?: return@registerHandler false
        context.setForm(normalizeScheduleForm(selected))
        true
    }
    runtime.registerHandler("schedule.onFetchSchedules") { true }
    runtime.registerHandler("schedule.onFetchAgentsLov") { true }
    runtime.registerHandler("schedule.onFetchModelsLov") { true }
    runtime.registerHandler("schedule.applyLookupFilter") { true }
    runtime.registerHandler("schedule.saveSchedule") { args ->
        val context = args.context ?: return@registerHandler false
        runScheduleAction(context) {
            val form = normalizeScheduleForm(context.peekForm())
            val validation = validateScheduleForm(form)
            if (validation.isNotEmpty()) {
                context.setForm(form + ("validationErrors" to validation))
                error(validation.values.joinToString(". "))
            }
            val id = form["id"]?.toString()?.trim().orEmpty().ifBlank { UUID.randomUUID().toString() }
            val schedule = scheduleFromForm(form, id)
            client.upsertSchedules(listOf(schedule))
            val saved = client.getSchedule(id) ?: schedule
            val row = scheduleRow(saved)
            val current = context.collection.peek().toMutableList()
            val index = current.indexOfFirst { it["id"]?.toString() == id }
            val selectedIndex = if (index >= 0) {
                current[index] = row
                index
            } else {
                current += row
                current.lastIndex
            }
            context.collection.set(current)
            val normalizedSaved = normalizeScheduleForm(row)
            context.setSelection(SelectionState(selected = normalizedSaved, rowIndex = selectedIndex))
            context.setForm(normalizedSaved)
            context.fetchCollection()
        }
    }
    runtime.registerHandler("schedule.runSelected") { args ->
        val context = args.context ?: return@registerHandler false
        val id = context.peekSelection().selected?.get("id")?.toString()?.trim().orEmpty()
        if (id.isBlank()) return@registerHandler false
        runScheduleAction(context) { client.runScheduleNow(id) }
    }
    runtime.registerHandler("schedule.openHistory") { args ->
        val context = args.context ?: return@registerHandler false
        val selected = context.peekSelection().selected ?: return@registerHandler false
        val id = selected["id"]?.toString()?.trim().orEmpty()
        if (id.isBlank()) return@registerHandler false
        onOpenScheduleHistory(
            ScheduleHistoryFilter(
                scheduleId = id,
                scheduleName = selected["name"]?.toString()?.trim().orEmpty().ifBlank { "Automation" }
            )
        )
        true
    }
    runtime.registerHandler("schedule.deleteSchedule") { args ->
        val context = args.context ?: return@registerHandler false
        val id = context.peekSelection().selected?.get("id")?.toString()?.trim().orEmpty()
        if (id.isBlank()) return@registerHandler false
        runScheduleAction(context) {
            client.deleteSchedule(id)
            context.collection.set(context.collection.peek().filterNot { it["id"]?.toString() == id })
            context.resetSelection()
            context.setForm(emptyMap())
            context.fetchCollection()
        }
    }
    runtime.registerHandler("schedule.formatStatus") { args ->
        val value = args.rowValue()
        value?.replaceFirstChar { it.uppercase() } ?: "—"
    }
    runtime.registerHandler("schedule.formatDate") { args -> formatScheduleDate(args.rowValue()) }
    runtime.registerHandler("schedule.formatScheduleType") { args ->
        args.rowValue()?.replaceFirstChar { it.uppercase() } ?: "—"
    }
    runtime.registerHandler("schedule.formatCronSummary") { args ->
        val row = args.args["row"] as? Map<*, *> ?: emptyMap<String, Any?>()
        normalizeScheduleForm(row.entries.associate { it.key.toString() to it.value })["scheduleSummary"] ?: "—"
    }
}

private suspend fun runScheduleAction(context: DataSourceContext, action: suspend () -> Unit): Boolean {
    context.control.set(context.control.peek().copy(loading = true, error = null))
    return try {
        action()
        true
    } catch (error: Throwable) {
        context.control.set(context.control.peek().copy(error = error.message ?: "Automation action failed"))
        false
    } finally {
        context.control.set(context.control.peek().copy(loading = false))
    }
}

private fun ExecutionArgs.hasScheduleSelection(): Boolean =
    !context?.peekSelection()?.selected?.get("id")?.toString().isNullOrBlank()

private fun ExecutionArgs.editorKind(): String =
    normalizeScheduleForm(context?.peekForm().orEmpty())["scheduleEditorKind"]?.toString() ?: "calendar"

private fun ExecutionArgs.rowValue(): String? {
    val row = args["row"] as? Map<*, *> ?: return null
    val item = args["col"] as? Map<*, *>
    val key = item?.get("id")?.toString() ?: return null
    return row[key]?.toString()
}

internal fun normalizeScheduleForm(source: Map<String, Any?>): Map<String, Any?> {
    val next = source.toMutableMap()
    var kind = next["scheduleEditorKind"]?.toString()?.lowercase()
    if (kind !in setOf("calendar", "elapsed", "advanced")) {
        kind = if (next["scheduleType"]?.toString()?.lowercase() == "interval") "elapsed"
        else inferCronEditorKind(next["cronExpr"]?.toString())
    }
    val weekdays = normalizeWeekdays(next["weekdays"])
    val calendarTime = next["calendarTime"]?.toString()?.takeIf { it.isNotBlank() } ?: DEFAULT_CALENDAR_TIME
    val calendarPattern = next["calendarPattern"]?.toString()?.takeIf { it in setOf("once", "every") } ?: "once"
    val calendarInterval = positiveInt(next["calendarIntervalHours"]) ?: 2
    val elapsed = elapsedParts(next["intervalSeconds"], next["elapsedIntervalValue"], next["elapsedIntervalUnit"])
    val cron = when (kind) {
        "calendar" -> calendarCron(calendarTime, calendarPattern, calendarInterval, weekdays)
        "advanced" -> next["cronExpr"]?.toString()?.trim().orEmpty()
        else -> elapsedPseudoCron(elapsed.first, elapsed.second)
    }
    next["scheduleEditorKind"] = kind
    next["calendarPattern"] = calendarPattern
    next["calendarTime"] = calendarTime
    next["calendarIntervalHours"] = calendarInterval
    next["weekdays"] = weekdays
    next["elapsedIntervalValue"] = elapsed.first
    next["elapsedIntervalUnit"] = elapsed.second
    next["visibility"] = next["visibility"]?.toString()?.takeIf { it.isNotBlank() } ?: "private"
    next["timezone"] = next["timezone"]?.toString()?.takeIf { it.isNotBlank() } ?: "UTC"
    next["timeoutSeconds"] = positiveInt(next["timeoutSeconds"]) ?: DEFAULT_TIMEOUT_SECONDS
    next["scheduleType"] = if (kind == "elapsed") "interval" else "cron"
    next["intervalSeconds"] = if (kind == "elapsed") elapsed.first * elapsedMultiplier(elapsed.second) else null
    next["cronExpr"] = cron
    next["scheduleSummary"] = when (kind) {
        "elapsed" -> "Every ${elapsed.first} ${elapsed.second}"
        "advanced" -> cron.ifBlank { "Custom cron" }
        else -> if (calendarPattern == "every") "Every $calendarInterval hours" else "At $calendarTime"
    }
    return next
}

private fun validateScheduleForm(form: Map<String, Any?>): Map<String, String> = buildMap {
    if (form["name"]?.toString().isNullOrBlank()) put("name", "Schedule Name is required")
    if (form["agentRef"]?.toString().isNullOrBlank()) put("agentRef", "Agent is required")
    if (form["taskPrompt"]?.toString().isNullOrBlank() && form["taskPromptUri"]?.toString().isNullOrBlank()) {
        put("taskPrompt", "Task Prompt or Task Prompt URI is required")
    }
    if (form["scheduleEditorKind"] == "advanced" && form["cronExpr"]?.toString().isNullOrBlank()) {
        put("cronExpr", "Cron Expression is required")
    }
}

private fun scheduleFromForm(form: Map<String, Any?>, id: String): Schedule = Schedule(
    id = id,
    name = form["name"]?.toString()?.trim().orEmpty(),
    description = optionalText(form["description"]),
    visibility = optionalText(form["visibility"]),
    agentRef = form["agentRef"]?.toString()?.trim().orEmpty(),
    modelOverride = optionalText(form["modelOverride"]),
    userCredUrl = optionalText(form["userCredUrl"]),
    enabled = booleanValue(form["enabled"]),
    startAt = optionalText(form["startAt"]),
    endAt = optionalText(form["endAt"]),
    scheduleType = form["scheduleType"]?.toString()?.ifBlank { "cron" } ?: "cron",
    cronExpr = optionalText(form["cronExpr"]),
    intervalSeconds = positiveInt(form["intervalSeconds"]),
    timezone = optionalText(form["timezone"]),
    timeoutSeconds = positiveInt(form["timeoutSeconds"]),
    taskPromptUri = optionalText(form["taskPromptUri"]),
    taskPrompt = optionalText(form["taskPrompt"])
)

private fun scheduleRow(schedule: Schedule): Map<String, Any?> = mapOf(
    "id" to schedule.id,
    "name" to schedule.name,
    "description" to schedule.description,
    "visibility" to schedule.visibility,
    "agentRef" to schedule.agentRef,
    "modelOverride" to schedule.modelOverride,
    "userCredUrl" to schedule.userCredUrl,
    "enabled" to schedule.enabled,
    "startAt" to schedule.startAt,
    "endAt" to schedule.endAt,
    "scheduleType" to schedule.scheduleType,
    "cronExpr" to schedule.cronExpr,
    "intervalSeconds" to schedule.intervalSeconds,
    "timezone" to schedule.timezone,
    "timeoutSeconds" to schedule.timeoutSeconds,
    "taskPromptUri" to schedule.taskPromptUri,
    "taskPrompt" to schedule.taskPrompt,
    "nextRunAt" to schedule.nextRunAt,
    "lastRunAt" to schedule.lastRunAt,
    "lastStatus" to schedule.lastStatus,
    "createdAt" to schedule.createdAt,
    "updatedAt" to schedule.updatedAt
)

private fun inferCronEditorKind(cron: String?): String {
    val parts = cron?.trim()?.split(Regex("\\s+")) ?: return "calendar"
    return if (parts.size == 5 && parts[0].toIntOrNull() != null && parts[1].toIntOrNull() != null &&
        parts[2] == "*" && parts[3] == "*") "calendar" else "advanced"
}

private fun normalizeWeekdays(value: Any?): List<String> {
    val values = when (value) {
        is List<*> -> value.mapNotNull { it?.toString() }
        is String -> value.split(',')
        else -> emptyList()
    }.map { it.trim().lowercase() }.filter { it in allWeekdays }.distinct()
    return values.ifEmpty { allWeekdays }
}

private fun calendarCron(time: String, pattern: String, every: Int, weekdays: List<String>): String {
    val (hour, minute) = parseTime(time)
    val dayOfWeek = if (weekdays.size == allWeekdays.size) "*"
    else weekdays.mapNotNull(weekdayCron::get).distinct().joinToString(",")
    val hourExpression = if (pattern != "every") hour.toString()
    else if (hour == 0) "*/${every.coerceIn(1, 23)}" else "$hour-23/${every.coerceIn(1, 23)}"
    return "$minute $hourExpression * * $dayOfWeek"
}

private fun parseTime(value: String): Pair<Int, Int> {
    val match = Regex("^(\\d{1,2}):(\\d{2})(?:\\s*([AaPp][Mm]))?$").matchEntire(value.trim())
        ?: return 9 to 0
    var hour = match.groupValues[1].toIntOrNull() ?: 9
    val minute = (match.groupValues[2].toIntOrNull() ?: 0).coerceIn(0, 59)
    when (match.groupValues[3].lowercase()) {
        "pm" -> if (hour in 1..11) hour += 12
        "am" -> if (hour == 12) hour = 0
    }
    return hour.coerceIn(0, 23) to minute
}

private fun elapsedParts(intervalSeconds: Any?, rawValue: Any?, rawUnit: Any?): Pair<Int, String> {
    val explicitValue = positiveInt(rawValue)
    val explicitUnit = rawUnit?.toString()?.lowercase()?.takeIf { it in setOf("minutes", "hours", "days") }
    if (explicitValue != null) return explicitValue to (explicitUnit ?: "hours")
    val seconds = positiveInt(intervalSeconds) ?: return 24 to "hours"
    return when {
        seconds % 86400 == 0 -> seconds / 86400 to "days"
        seconds % 3600 == 0 -> seconds / 3600 to "hours"
        else -> (seconds / 60).coerceAtLeast(1) to "minutes"
    }
}

private fun elapsedMultiplier(unit: String): Int = when (unit) {
    "minutes" -> 60
    "days" -> 86400
    else -> 3600
}

private fun elapsedPseudoCron(value: Int, unit: String): String = when (unit) {
    "minutes" -> "*/$value * * * *"
    "days" -> "0 0 */$value * *"
    else -> "0 */$value * * *"
}

private fun positiveInt(value: Any?): Int? = when (value) {
    is Number -> value.toInt().takeIf { it > 0 }
    else -> value?.toString()?.trim()?.toIntOrNull()?.takeIf { it > 0 }
}

private fun booleanValue(value: Any?): Boolean = when (value) {
    is Boolean -> value
    is Number -> value.toInt() != 0
    else -> value?.toString()?.trim()?.lowercase() in setOf("true", "1", "yes", "on")
}

private fun optionalText(value: Any?): String? = value?.toString()?.trim()?.takeIf { it.isNotEmpty() }

private fun formatScheduleDate(value: String?): String {
    if (value.isNullOrBlank()) return "—"
    return runCatching { OffsetDateTime.parse(value).toLocalDateTime().toString() }.getOrDefault(value)
}
