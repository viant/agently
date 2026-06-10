package com.viant.agently.android

import com.viant.agentlysdk.stream.ActiveFeed
import com.viant.agentlysdk.stream.ConversationStreamSnapshot
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

internal data class GoalControllerScheduleInfo(
    val mode: String = "",
    val reason: String? = null,
    val preview: String? = null,
    val wakeAt: String? = null,
)

internal fun goalControllerScheduleInfo(snapshot: ConversationStreamSnapshot?): GoalControllerScheduleInfo? {
    val goalFeed = snapshot?.feeds?.firstOrNull { it.feedId.trim() == "goal" } ?: return null
    return goalControllerScheduleInfo(goalFeed)
}

private fun goalControllerScheduleInfo(feed: ActiveFeed): GoalControllerScheduleInfo? {
    val root = feed.data as? JsonObject ?: return null
    val schedule = root["controllerSchedule"] as? JsonObject ?: return null
    return GoalControllerScheduleInfo(
        mode = (schedule["mode"] as? JsonPrimitive)?.content.orEmpty(),
        reason = (schedule["reason"] as? JsonPrimitive)?.content,
        preview = (schedule["preview"] as? JsonPrimitive)?.content,
        wakeAt = (schedule["wakeAt"] as? JsonPrimitive)?.content,
    )
}
