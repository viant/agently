package com.viant.agently.android
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Test
class NativeReportCommandIdentityTest {
    @Test fun suppliedIdentityIsExactAndPartialPairsFail() {
        val id="12345678-1234-1234-1234-123456789ABC"; val ref=" opaque/ref "
        val pair=buildJsonObject {put("requestId",id);put("reportAdmissionRef",ref)}
        val command=nativeReportCommandIdentity(pair)
        assertEquals(id,command.requestId);assertEquals(ref,command.reportAdmissionRef)
        for (invalid in listOf(buildJsonObject{put("requestId",id)},buildJsonObject{put("reportAdmissionRef",ref)},buildJsonObject{put("requestId","bad");put("reportAdmissionRef",ref)})) {
            assertTrue(runCatching{nativeReportCommandIdentity(invalid)}.isFailure)
        }
        val a=nativeReportCommandIdentity(JsonObject(emptyMap()));val b=nativeReportCommandIdentity(JsonObject(emptyMap()))
        assertTrue(a.requestId.startsWith("native-"));assertNull(a.reportAdmissionRef);assertNotEquals(a.requestId,b.requestId)
    }
    @Test fun onlyExactServerLinkageMayBeRemovedForEquality() {
        val expected=buildJsonObject{put("scope","original")}
        val linked=JsonObject(expected+mapOf("_agentlyForecastCommand" to buildJsonObject{put("version",1);put("ref","ref");put("requestId","id")}))
        assertTrue(nativeReportRequestedParamsMatch(linked,expected,"id","ref"))
        assertFalse(nativeReportRequestedParamsMatch(linked,expected,"other","ref"))
        assertFalse(nativeReportRequestedParamsMatch(linked,expected,"id",null))
        assertFalse(nativeReportRequestedParamsMatch(linked,buildJsonObject{put("scope","changed")},"id","ref"))
        assertTrue(nativeReportRequestedParamsMatch(expected,expected,"manual",null))
    }
}
