import XCTest
import AgentlySDK
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class NativeReportContextRestoreTests: XCTestCase {
    private func fixture() throws -> ([String: AgentlySDK.JSONValue], ReportRun, AgentlySDK.JSONValue) {
        let configuration: [String: ForgeIOSRuntime.JSONValue] = ["request": .object(["autoFetch": .bool(false)])]
        let state: [String: ForgeIOSRuntime.JSONValue] = ["opaque": .string("keep")]
        let document: [String: ForgeIOSRuntime.JSONValue] = ["title": .string("Saved"), "blocks": .array([.object(["id": .string("table"), "datasetRef": .string("primary")])])]
        let request: [String: ForgeIOSRuntime.JSONValue] = ["filters": .object(["orderIds": .array([.number(7)])])]
        var calendar = Calendar(identifier: .gregorian); calendar.timeZone = TimeZone(identifier: "America/Los_Angeles")!
        let packet = PreparedReportRequest(identity: .init(windowId: "window", builderRef: "builder", formRevision: .string("form"), stateRevision: .string("state")), status: "ready", hookStatus: "completed", dataSourceRef: "cube", request: request, state: ["policy": .string("original")], preparedAt: Date(timeIntervalSince1970: 1000), preparedCalendar: calendar)
        let admission = NativeReportAdmission(preparation: packet, conversationID: "conversation", stateKey: "state", document: document, datasets: [.init(id: "primary", dataSourceRef: "cube", request: request)], authoredConfiguration: configuration, authorState: state)
        let spec = nativeReportInvocation(admission)
        var fill = spec.objectValue!
        fill["datasets"] = .array([.object(["id": .string("primary"), "dataSourceRef": .string("cube"), "request": .object(request), "rows": .array([])])])
        let rawRun = ForgeIOSRuntime.JSONValue.object(["reportRunId": .string("saved"), "ownerId": .string("owner"), "conversationId": .string("conversation"), "builderRef": .string("builder"), "status": .string("completed"), "revision": .number(2), "requestedParams": .object(try nativeReportRequestedParams(admission)), "effectiveParams": .object(request), "reportSpec": spec, "reportFill": .object(fill)])
        let run = try JSONDecoder().decode(ReportRun.self, from: JSONEncoder().encode(rawRun))
        let form: [String: AgentlySDK.JSONValue] = ["reportBuilderRef": .string("builder"), "state": ForgeIOSRuntime.JSONValue.object(state).appValue, "reportDefinition": .object(["documentPatch": ForgeIOSRuntime.JSONValue.object(document).appValue])]
        let metadata = ForgeIOSRuntime.JSONValue.object(["view": .object(["content": .object(["containers": .array([.object(["id": .string("builder"), "dataSourceRef": .string("cube"), "dashboard": .object(["reportBuilder": .object(configuration)])])])])])]).appValue
        return (form, run, metadata)
    }
    func testVerifiedNamespaceRestoresEmptyPrimaryWithoutChangingAuthoredState() throws {
        let (form, run, metadata) = try fixture()
        let restored = try restoreNativeReportContext(form, run: run, conversationID: "conversation", metadata: metadata, windowID: "window")
        XCTAssertEqual(restored.form["state"], form["state"])
        XCTAssertEqual(restored.form["reportDefinition"], form["reportDefinition"])
        XCTAssertEqual(restored.form["executeOnOpen"], .bool(false))
        XCTAssertEqual(restored.admission.preparation.preparedAt, Date(timeIntervalSince1970: 1000))
        XCTAssertEqual(restored.form["reportMaterialization"]?.objectValue?["reportRunId"], .string("saved"))
    }
    func testChangedPrefillRejectsOldQueryDespiteMatchingAuthorState() throws {
        let (original, run, metadata) = try fixture()
        for change: [String: AgentlySDK.JSONValue] in [
            ["prefill": .object(["orderId": .number(99)])],
            ["__forge": .object(["prefillRevision": .number(2)])]
        ] {
            let form = original.merging(change) { _, new in new }
            XCTAssertThrowsError(try restoreNativeReportContext(form, run: run, conversationID: "conversation", metadata: metadata, windowID: "window"))
            XCTAssertNil(restoredCompletedReportForm(form, run: run, conversationID: "conversation", liveMetadata: metadata))
        }
    }
    func testEditedAuthorStateRejectsNamespaceWithoutLegacyFallback() throws {
        var (form, run, metadata) = try fixture()
        form["state"] = .object(["opaque": .string("changed")])
        XCTAssertThrowsError(try restoreNativeReportContext(form, run: run, conversationID: "conversation", metadata: metadata, windowID: "window"))
        XCTAssertNil(restoredCompletedReportForm(form, run: run, conversationID: "conversation", liveMetadata: metadata))
    }
}
