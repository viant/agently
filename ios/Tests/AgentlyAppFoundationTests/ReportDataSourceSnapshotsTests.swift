import XCTest
import ForgeIOSRuntime
@testable import AgentlyAppFoundation

final class ReportDataSourceSnapshotsTests: XCTestCase {
    func testRegisteredPrimaryIsVisibleBeforeAndAfterExplicitFetchWithoutChildAliasing() async throws {
        let runtime = ForgeRuntime()
        let metadata = try JSONDecoder().decode(WindowMetadata.self, from: Data(#"{"view":{"content":{"containers":[{"id":"builder","kind":"dashboard.reportBuilder","dataSourceRef":"cube"}]}},"dataSource":{"cube":{"autoFetch":false}}}"#.utf8))
        let window = await runtime.openWindowInline(key: "report", title: "Report", metadata: metadata, conversationID: "conversation")
        let identity = await runtime.reportPreparationIdentity(windowID: window.id, builderRef: "builder")
        let query: [String: JSONValue] = ["filters": .object(["orderIds": .array([.number(7)])])]
        let packet = PreparedReportRequest(identity: identity, status: "ready", hookStatus: "completed", dataSourceRef: "cube", request: query)
        _ = await runtime.publishPreparedReportRequest(packet)
        let absent = await registeredReportDataSourceSnapshots(runtime: runtime, windowID: window.id, physicalRefs: ["cube"], admission: nil)
        XCTAssertTrue(absent.isEmpty)
        try await runtime.registerPreparedReportPrimaryContext(packet)
        let staged = await registeredReportDataSourceSnapshots(runtime: runtime, windowID: window.id, physicalRefs: ["cube"], admission: nil)
        XCTAssertEqual(staged["cube"]?.objectValue?["input"]?.objectValue?["parameters"], .object(query.mapValues(\.appValue)))
        XCTAssertEqual(staged["cube"]?.objectValue?["collection"], .null)
        XCTAssertEqual(staged["cube"]?.objectValue?["control"]?.objectValue?["resolved"], .bool(false))
        await runtime.registerDataSourceLoader { _ in ForgeRuntime.DataSourceFetchResult(rows: [["spend": .number(309)]], metrics: ["rowCount": .number(1)]) }
        try await runtime.fetchPreparedReportDataSource(windowID: window.id, dataSourceRef: "cube")
        let loaded = await registeredReportDataSourceSnapshots(runtime: runtime, windowID: window.id, physicalRefs: ["cube"], admission: nil)
        XCTAssertEqual(loaded["cube"]?.objectValue?["collection"], .array([.object(["spend": .number(309)])]))
        XCTAssertEqual(loaded["cube"]?.objectValue?["control"]?.objectValue?["resolved"], .bool(true))
    }

    func testCompletedLogicalAliasesPreserveSeparateRequestsAndEmptyRows() async throws {
        let runtime = ForgeRuntime()
        let metadata = try JSONDecoder().decode(WindowMetadata.self, from: Data(#"{"view":{"content":{"containers":[]}}}"#.utf8))
        let window = await runtime.openWindowInline(key: "report", title: "Report", metadata: metadata, conversationID: "conversation")
        let bindings: [JSONValue] = [
            .object(["id": .string("today"), "dataSourceRef": .string("cube"), "request": .object(["filters": .object(["day": .string("today")])])]),
            .object(["id": .string("yesterday"), "dataSourceRef": .string("cube"), "request": .object(["filters": .object(["day": .string("yesterday")])])])
        ]
        var form: [String: JSONValue] = [
            "reportBuilderRef": .string("builder"),
            "reportMaterialization": .object(["id": .string("run"), "status": .string("completed")]),
            "reportStaticDatasets": .array([
                .object(["id": .string("today"), "dataSourceRef": .string("cube"), "rows": .array([.object(["spend": .number(309)])])]),
                .object(["id": .string("yesterday"), "dataSourceRef": .string("cube"), "rows": .array([])])
            ])
        ]
        form["reportValidatedRestore"] = .object(["runId": .string("run"), "authorInputs": .string(nativeReportLocalDigest(.object(reportPreparationAuthorInputs(form)))), "datasets": .array(bindings)])
        await runtime.setWindowFormValue(windowID: window.id, values: form, replace: true, bumpPrefillRevision: false)
        let untrusted = await runtime.completedNativeReportDatasets(windowID: window.id)
        XCTAssertNil(untrusted, "A form proof alone must not authorize rows")
        try await runtime.installVerifiedLegacyReportCache(windowID: window.id, conversationID: "conversation", reportRunID: "run", ownerID: "owner", form: form, generation: 0)
        let verified = await runtime.completedNativeReportDatasets(windowID: window.id)
        let snapshots = completedReportDataSourceSnapshots(verified)
        XCTAssertEqual(Set(snapshots.keys), Set(["today", "yesterday"]))
        XCTAssertNil(snapshots["cube"], "A shared physical source must not select an arbitrary logical dataset")
        XCTAssertEqual(snapshots["today"]?.objectValue?["filter"]?.objectValue?["day"], .string("today"))
        XCTAssertEqual(snapshots["yesterday"]?.objectValue?["collection"], .array([]))
        form["reportStaticDatasets"] = .array([.object(["id": .string("today"), "dataSourceRef": .string("cube"), "rows": .array([.object(["spend": .number(999)])])]), .object(["id": .string("yesterday"), "dataSourceRef": .string("cube"), "rows": .array([])])])
        await runtime.setWindowFormValue(windowID: window.id, values: form, replace: true, bumpPrefillRevision: false)
        let tampered = await runtime.completedNativeReportDatasets(windowID: window.id)
        XCTAssertNil(tampered)
        await runtime.bindNativeReportAccount(generation: 1)
        let staleAccount = await runtime.completedNativeReportDatasets(windowID: window.id)
        XCTAssertNil(staleAccount)
    }
}
