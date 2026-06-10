package com.viant.agently.android

internal sealed interface GoalCommandAction {
    data object Show : GoalCommandAction
    data class Set(val objective: String) : GoalCommandAction
    data object Pause : GoalCommandAction
    data object Resume : GoalCommandAction
    data object Clear : GoalCommandAction
    data object Help : GoalCommandAction
}

internal fun parseGoalCommand(text: String): GoalCommandAction? {
    val raw = text.trim()
    if (!raw.startsWith("/goal", ignoreCase = true)) return null
    val rest = raw.removePrefix(raw.take(5)).trim()
    if (rest.isBlank()) return GoalCommandAction.Show
    return when (rest.lowercase()) {
        "show", "status" -> GoalCommandAction.Show
        "pause" -> GoalCommandAction.Pause
        "resume" -> GoalCommandAction.Resume
        "clear" -> GoalCommandAction.Clear
        "help" -> GoalCommandAction.Help
        else -> {
            if (rest.startsWith("set ", ignoreCase = true)) {
                GoalCommandAction.Set(rest.drop(4).trim())
            } else {
                GoalCommandAction.Set(rest)
            }
        }
    }
}
