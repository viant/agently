import Foundation
import AgentlySDK
import ForgeIOSRuntime

internal struct NativeReportRunLifecycleHandler: NativeReportLifecycleHandler {
    let client: AgentlyClient
    func begin(admission: NativeReportAdmission, uiRunRequestID: String, origin: String) async throws -> NativeReportRunHandle {
        guard !admission.conversationID.isEmpty else { throw NativeReportPersistenceError.invalidIdentity }
        let packet = admission.preparation
        let requested = try nativeReportRequestedParams(admission)
        let result = try await client.beginReportRun(BeginReportRunInput(
            uiRunRequestId: uiRunRequestID, conversationId: admission.conversationID, origin: origin == "ui.report.run" ? "prompt" : origin,
            builderRef: packet.identity.builderRef, presetId: packet.state["selectedReportPresetId"]?.stringValue,
            sourceKind: "dashboard.reportBuilder", sourceId: packet.identity.builderRef,
            requestedParams: .object(requested.mapValues(\.appValue)), effectiveParams: .object(packet.request.mapValues(\.appValue))))
        guard result.run.status == "running", result.run.conversationId == admission.conversationID,
              result.run.builderRef == packet.identity.builderRef,
              result.run.effectiveParams?.forgeValue == .object(packet.request), !result.run.ownerId.isEmpty else { throw NativeReportPersistenceError.invalidIdentity }
        if let context = result.context {
            guard context.ownerId == result.run.ownerId, context.conversationId == admission.conversationID else { throw NativeReportPersistenceError.invalidIdentity }
        }
        return NativeReportRunHandle(reportRunID: result.run.reportRunId, revision: result.run.revision, uiRunRequestID: uiRunRequestID, admission: admission, ownerID: result.run.ownerId, expectedContextRevision: result.context?.revision ?? 0)
    }
    func complete(handle: NativeReportRunHandle, rows: [String: [[String: ForgeIOSRuntime.JSONValue]]], current: @escaping @Sendable () async -> Bool) async throws -> NativeReportCompletedRun {
        guard await current(), Set(rows.keys) == Set(handle.admission.datasets.map(\.id)) else { throw NativeReportPersistenceError.invalidIdentity }
        let invocation = nativeReportInvocation(handle.admission)
        let raw = try await client.executeTool(name: "reporting:compile_fenced_report", args: [
            "reportId": .string(handle.uiRunRequestID), "fences": nativeReportFences(handle.uiRunRequestID, document: handle.admission.document, rows: rows).appValue,
            "invocation": invocation.appValue
        ], conversationID: handle.admission.conversationID)
        let compiled = try JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: Data(raw.utf8))
        guard let object = compiled.objectValue, let spec = object["reportSpec"], let fill = object["reportFill"], let print = object["reportPrint"] else { throw NativeReportPersistenceError.invalidArtifacts }
        try validateNativeReportCompilerIdentity(spec, invocation: invocation)
        try validateNativeReportFillIdentity(fill, spec: spec, admission: handle.admission, rows: rows)
        guard await current() else { throw NativeReportPersistenceError.invalidIdentity }
        let input = CompleteReportRunInput(expectedRevision: handle.revision, reportSpec: spec.appValue, reportFill: fill.appValue, reportPrint: print.appValue, conversationId: handle.admission.conversationID)
        let run: ReportRun
        do { run = try await client.completeReportRun(id: handle.reportRunID, input: input) }
        catch {
            // A lost response may follow a successful commit. Reconcile before
            // deciding whether completion failed; never repeat compilation IO.
            do { run = try await client.getReportRun(id: handle.reportRunID, conversationID: handle.admission.conversationID) }
            catch { throw NativeReportCompletionUncertainError() }
        }
        guard run.status == "completed", run.reportRunId == handle.reportRunID, run.conversationId == handle.admission.conversationID,
              run.builderRef == handle.admission.preparation.identity.builderRef, run.ownerId == handle.ownerID,
              run.reportSpec?.forgeValue == spec, run.reportFill?.forgeValue == fill, run.reportPrint?.forgeValue == print else {
            if run.status == "completed" { throw NativeReportCompletionUncertainError() }
            throw NativeReportPersistenceError.invalidArtifacts
        }
        // Completion does not select a run. Never substitute a newer context
        // revision after a conflict: that would overwrite another client's run.
        var context: ReportContext?
        if await current() {
            do {
                context = try await client.activateReportRun(id: run.reportRunId, input: ActivateReportRunInput(conversationId: handle.admission.conversationID, expectedRunRevision: run.revision, expectedContextRevision: handle.expectedContextRevision, source: "prompt"))
            } catch { /* A lost response may follow a successful activation. */ }
        }
        if context == nil {
            context = try? await client.getReportContext(conversationID: handle.admission.conversationID)
        }
        guard let context, context.ownerId == handle.ownerID, context.conversationId == handle.admission.conversationID else {
            return NativeReportCompletedRun(reportRunID: run.reportRunId, revision: run.revision, contextStatus: "unconfirmed", active: nil, activationError: "The report was saved, but its active selection could not be confirmed.")
        }
        if context.activeReportRunId == handle.reportRunID {
            return NativeReportCompletedRun(reportRunID: run.reportRunId, revision: run.revision)
        }
        if context.revision > handle.expectedContextRevision {
            return NativeReportCompletedRun(reportRunID: run.reportRunId, revision: run.revision, contextStatus: "superseded", active: false, activationError: "The report was saved, but a newer report is active in this conversation.")
        }
        return NativeReportCompletedRun(reportRunID: run.reportRunId, revision: run.revision, contextStatus: "unconfirmed", active: nil, activationError: "The report was saved, but its active selection could not be confirmed.")
    }

    func fail(handle: NativeReportRunHandle, code: String, text: String) async throws {
        _ = try await client.failReportRun(id: handle.reportRunID, input: FailReportRunInput(expectedRevision: handle.revision, conversationId: handle.admission.conversationID, failureCode: code, failureText: text))
    }
}
internal enum NativeReportPersistenceError: Error, LocalizedError {
    case invalidIdentity, invalidArtifacts, completionUncertain
    var errorDescription: String? {
        switch self {
        case .invalidIdentity: return "The report service did not preserve the admitted request identity."
        case .invalidArtifacts: return "The report service did not preserve the completed report artifacts."
        case .completionUncertain: return "Report completion could not be confirmed. Reopen the report to check its saved status."
        }
    }
}
internal func nativeReportInvocation(_ admission: NativeReportAdmission) -> ForgeIOSRuntime.JSONValue {
    .object([
        "source": .object(["kind": .string("dashboard.reportBuilder"), "containerId": .string(admission.preparation.identity.builderRef), "stateKey": .string(admission.stateKey), "dataSourceRef": .string(admission.preparation.dataSourceRef)]),
        "parameters": .object(admission.parameters),
        "datasets": .array(admission.datasets.map { .object(["id": .string($0.id), "dataSourceRef": .string($0.dataSourceRef), "request": .object($0.request)]) })
    ])
}
internal func nativeReportFences(_ reportID: String, document: [String: ForgeIOSRuntime.JSONValue], rows: [String: [[String: ForgeIOSRuntime.JSONValue]]]) -> ForgeIOSRuntime.JSONValue {
    var start = document
    start.merge(["version": .number(1), "scope": .string("message"), "id": .string(reportID), "sequence": .number(1), "mode": .string("start"), "grammar": .string("report-document-v1")]) { _, new in new }
    var fences: [ForgeIOSRuntime.JSONValue] = [.object(["kind": .string("forge-report"), "payload": .object(start)])]
    for id in rows.keys.sorted() {
        fences.append(.object(["kind": .string("forge-data"), "payload": .object([
            "version": .number(2), "scope": .string("message"), "id": .string(id), "reportRef": .string(reportID),
            "sequence": .number(Double(fences.count + 1)), "format": .string("json"), "mode": .string("replace"), "data": .array((rows[id] ?? []).map(ForgeIOSRuntime.JSONValue.object))
        ])]))
    }
    fences.append(.object(["kind": .string("forge-report"), "payload": .object(["version": .number(1), "scope": .string("message"), "id": .string(reportID), "sequence": .number(Double(fences.count + 1)), "mode": .string("commit")])]))
    return .array(fences)
}
internal func validateNativeReportCompilerIdentity(_ spec: ForgeIOSRuntime.JSONValue, invocation: ForgeIOSRuntime.JSONValue) throws {
    guard spec.objectValue?["source"] == invocation.objectValue?["source"], spec.objectValue?["parameters"] == invocation.objectValue?["parameters"],
          case .array(let expected) = invocation.objectValue?["datasets"], case .array(let actual) = spec.objectValue?["datasets"], actual.count == expected.count else { throw NativeReportPersistenceError.invalidIdentity }
    var actualByID: [String: ForgeIOSRuntime.JSONValue] = [:]
    for item in actual {
        guard let id = item.objectValue?["id"]?.stringValue, actualByID[id] == nil else { throw NativeReportPersistenceError.invalidIdentity }
        actualByID[id] = item
    }
    for item in expected {
        guard let id = item.objectValue?["id"]?.stringValue, let found = actualByID.removeValue(forKey: id),
              found.objectValue?["dataSourceRef"] == item.objectValue?["dataSourceRef"], found.objectValue?["request"] == item.objectValue?["request"] else { throw NativeReportPersistenceError.invalidIdentity }
    }
    guard actualByID.isEmpty else { throw NativeReportPersistenceError.invalidIdentity }
}

internal func validateNativeReportFillIdentity(_ fill: ForgeIOSRuntime.JSONValue, spec: ForgeIOSRuntime.JSONValue, admission: NativeReportAdmission, rows: [String: [[String: ForgeIOSRuntime.JSONValue]]]) throws {
    guard fill.objectValue?["source"] == spec.objectValue?["source"], case .array(let datasets) = fill.objectValue?["datasets"], datasets.count == admission.datasets.count else { throw NativeReportPersistenceError.invalidArtifacts }
    var remaining = Dictionary(uniqueKeysWithValues: admission.datasets.map { ($0.id, $0) })
    for value in datasets {
        guard let dataset = value.objectValue, let id = dataset["id"]?.stringValue, let expected = remaining.removeValue(forKey: id),
              dataset["dataSourceRef"] == .string(expected.dataSourceRef), dataset["request"] == .object(expected.request),
              let actualRows = rows[id], dataset["rows"] == .array(actualRows.map(ForgeIOSRuntime.JSONValue.object)) else { throw NativeReportPersistenceError.invalidArtifacts }
    }
    guard remaining.isEmpty else { throw NativeReportPersistenceError.invalidArtifacts }
}
