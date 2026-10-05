package com.viant.agently.android
import kotlinx.serialization.json.*
import java.util.UUID
internal data class NativeReportCommandIdentity(val requestId: String, val reportAdmissionRef: String?)
internal fun nativeReportCommandIdentity(params: JsonObject): NativeReportCommandIdentity {
    val hasId = params.containsKey("requestId"); val hasRef = params.containsKey("reportAdmissionRef")
    if (!hasId && !hasRef) return NativeReportCommandIdentity("native-" + UUID.randomUUID(), null)
    require(hasId && hasRef) { "requestId and reportAdmissionRef must be supplied together." }
    val id = (params["requestId"] as? JsonPrimitive)?.takeIf { it.isString }?.content
    val ref = (params["reportAdmissionRef"] as? JsonPrimitive)?.takeIf { it.isString }?.content
    require(id != null && Regex("[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}").matches(id) && ref != null && ref.isNotBlank()) { "A valid requestId UUID and nonempty reportAdmissionRef are required." }
    return NativeReportCommandIdentity(id, ref)
}
internal fun nativeReportRequestedParamsMatch(actual: JsonElement?, expected: JsonObject, requestId: String, ref: String?): Boolean {
    val key = "_agentlyForecastCommand"
    if (expected.containsKey(key)) return false
    val value = actual as? JsonObject ?: return false
    if (ref == null) return !value.containsKey(key) && value == expected
    val link = buildJsonObject { put("version", 1); put("ref", ref); put("requestId", requestId) }
    return value[key] == link && JsonObject(value - key) == expected
}
