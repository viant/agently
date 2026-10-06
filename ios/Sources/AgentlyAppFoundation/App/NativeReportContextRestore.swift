import Foundation
import AgentlySDK
import ForgeIOSRuntime

struct NativeReportContextRestoration {
    let form: [String: AgentlySDK.JSONValue]
    let admission: NativeReportAdmission
}

func selectedNativeReportConfiguration(_ metadata: ForgeIOSRuntime.JSONValue, builderRef: String) -> (configuration: [String: ForgeIOSRuntime.JSONValue], primaryRef: String)? {
    var candidates: [([String: ForgeIOSRuntime.JSONValue], String)] = []
    func visit(_ value: ForgeIOSRuntime.JSONValue, inheritedRef: String = "") {
        if let object = value.objectValue {
            let ref = object["dataSourceRef"]?.stringValue ?? inheritedRef
            if let selected = object["reportBuilders"]?.objectValue?[builderRef]?.objectValue,
               let config = selected["reportBuilder"]?.objectValue {
                candidates.append((config, selected["dataSourceRef"]?.stringValue ?? ref))
            }
            if object["id"] == .string(builderRef),
               let config = object["dashboard"]?.objectValue?["reportBuilder"]?.objectValue ?? object["reportBuilder"]?.objectValue {
                candidates.append((config, ref))
            }
            for (key, child) in object where key != "reportBuilders" { visit(child, inheritedRef: ref) }
        } else if case .array(let children) = value { for child in children { visit(child, inheritedRef: inheritedRef) } }
    }
    visit(metadata)
    var unique: [([String: ForgeIOSRuntime.JSONValue], String)] = []
    for candidate in candidates where !unique.contains(where: { $0.0 == candidate.0 && $0.1 == candidate.1 }) { unique.append(candidate) }
    guard unique.count == 1, !unique[0].1.isEmpty else { return nil }
    return (unique[0].0, unique[0].1)
}

func restoreNativeReportContext(_ form: [String: AgentlySDK.JSONValue], run: ReportRun, conversationID: String, metadata: AgentlySDK.JSONValue, windowID: String) throws -> NativeReportContextRestoration {
    guard run.status == "completed", run.conversationId == conversationID,
          let requested = run.requestedParams?.forgeValue.objectValue,
          let namespace = requested[nativeReportAdmissionKey],
          let effective = run.effectiveParams?.forgeValue.objectValue,
          requested.filter({ $0.key != nativeReportAdmissionKey }) == effective,
          let builder = form["reportBuilderRef"]?.forgeValue.stringValue, builder == run.builderRef,
          let stateKey = namespace.objectValue?["stateKey"]?.stringValue else { throw NativeReportPersistenceError.invalidIdentity }
    let rawForm = form.mapValues(\.forgeValue)
    guard let state = reportPreparationValue(rawForm, path: stateKey)?.objectValue,
          let document = nativeReportSelectedDocument(rawForm, stateKey: stateKey),
          let selected = selectedNativeReportConfiguration(metadata.forgeValue, builderRef: builder) else { throw NativeReportPersistenceError.invalidIdentity }
    let admission = try restoredNativeReportAdmission(namespace: namespace, primaryRequest: effective, authorState: state, document: document, configuration: selected.configuration, conversationID: conversationID, windowID: windowID, builderRef: builder, stateKey: stateKey, primaryDataSourceRef: selected.primaryRef, prefillIdentity: nativeReportPrefillIdentity(rawForm))
    let invocation = nativeReportInvocation(admission)
    guard let spec = run.reportSpec?.forgeValue, let fill = run.reportFill?.forgeValue,
          fill.objectValue?["parameters"] == invocation.objectValue?["parameters"] else { throw NativeReportPersistenceError.invalidArtifacts }
    try validateNativeReportCompilerIdentity(spec, invocation: invocation)
    guard case .array(let filled) = fill.objectValue?["datasets"] else { throw NativeReportPersistenceError.invalidArtifacts }
    var rows: [String: [[String: ForgeIOSRuntime.JSONValue]]] = [:]
    for item in filled {
        guard let dataset = item.objectValue, let id = dataset["id"]?.stringValue, rows[id] == nil,
              case .array(let rawRows) = dataset["rows"] else { throw NativeReportPersistenceError.invalidArtifacts }
        let objects = rawRows.compactMap(\.objectValue)
        guard objects.count == rawRows.count else { throw NativeReportPersistenceError.invalidArtifacts }
        rows[id] = objects
    }
    try validateNativeReportFillIdentity(fill, spec: spec, admission: admission, rows: rows)
    var hydrated = form
    hydrated.removeValue(forKey: "reportRunRequest")
    hydrated["executeOnOpen"] = .bool(false)
    hydrated["reportStaticDatasets"] = ForgeIOSRuntime.JSONValue.array(admission.datasets.map { dataset in .object([
        "id": .string(dataset.id), "dataSourceRef": .string(dataset.dataSourceRef), "request": .object(dataset.request), "rows": .array((rows[dataset.id] ?? []).map(ForgeIOSRuntime.JSONValue.object))
    ]) }).appValue
    hydrated["reportMaterialization"] = .object(["id": .string(run.reportRunId), "requestId": .string(run.reportRunId), "reportRunId": .string(run.reportRunId), "status": .string("completed"), "materialized": .bool(true), "contextStatus": .string("active"), "active": .bool(true)])
    return NativeReportContextRestoration(form: hydrated, admission: admission)
}
