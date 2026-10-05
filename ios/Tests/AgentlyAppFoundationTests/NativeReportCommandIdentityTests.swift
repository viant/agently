import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation
final class NativeReportCommandIdentityTests: XCTestCase {
    func testSuppliedIdentityIsExactAndPartialPairsFail() throws {
        let id = "12345678-1234-1234-1234-123456789ABC", ref = " opaque/ref "
        let command = try nativeReportCommandIdentity(["requestId": .string(id), "reportAdmissionRef": .string(ref)])
        XCTAssertEqual(command.requestID, id); XCTAssertEqual(command.reportAdmissionRef, ref)
        for invalid: [String: AgentlySDK.JSONValue] in [["requestId": .string(id)], ["reportAdmissionRef": .string(ref)], ["requestId": .string("bad"), "reportAdmissionRef": .string(ref)], ["requestId": .string(id), "reportAdmissionRef": .null]] {
            XCTAssertThrowsError(try nativeReportCommandIdentity(invalid))
        }
        let a = try nativeReportCommandIdentity([:]), b = try nativeReportCommandIdentity([:])
        XCTAssertTrue(a.requestID.hasPrefix("native-")); XCTAssertNil(a.reportAdmissionRef); XCTAssertNotEqual(a.requestID,b.requestID)
    }
    func testOnlyExactServerLinkageMayBeRemovedForEquality() {
        let expected: [String: ForgeIOSRuntime.JSONValue] = ["scope": .string("original")]
        let linked: AgentlySDK.JSONValue = .object(["scope": .string("original"), "_agentlyForecastCommand": .object(["version": .number(1), "ref": .string("ref"), "requestId": .string("id")])])
        XCTAssertTrue(nativeReportRequestedParamsMatch(linked, expected: expected, requestID: "id", ref: "ref"))
        XCTAssertFalse(nativeReportRequestedParamsMatch(linked, expected: expected, requestID: "other", ref: "ref"))
        XCTAssertFalse(nativeReportRequestedParamsMatch(linked, expected: expected, requestID: "id", ref: nil))
        XCTAssertFalse(nativeReportRequestedParamsMatch(linked, expected: ["scope": .string("changed")], requestID: "id", ref: "ref"))
        XCTAssertTrue(nativeReportRequestedParamsMatch(.object(["scope": .string("original")]), expected: expected, requestID: "manual", ref: nil))
    }
}
