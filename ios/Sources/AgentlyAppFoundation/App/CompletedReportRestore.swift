import Foundation
import AgentlySDK
import var ForgeIOSRuntime.nativeReportAdmissionKey
import func ForgeIOSRuntime.reportPreparationAuthorInputs
import func ForgeIOSRuntime.nativeReportLocalDigest

/// Restore only the authorized completed run whose published requests match this
/// window. The authored document/layout remains the window's current document.
func restoredCompletedReportForm(_ input: [String: JSONValue]?, run: ReportRun, conversationID: String, liveMetadata: JSONValue? = nil) -> [String: JSONValue]? {
    guard run.requestedParams?.objectValue?[nativeReportAdmissionKey] == nil else { return nil }
    guard var form = input, run.status == "completed", run.conversationId == conversationID,
          case .string(let builder) = form["reportBuilderRef"], builder == run.builderRef,
          let definition = form["reportDefinition"]?.objectValue,
          let definitionDocument = definition["documentPatch"]?.objectValue ?? definition["reportDocument"]?.objectValue,
          let spec = run.reportSpec?.objectValue, let fill = run.reportFill?.objectValue,
          let specSource = spec["source"], specSource == fill["source"],
          let nativeDatasets = spec["datasets"]?.restoreArray,
          let filledDatasets = fill["datasets"]?.restoreArray,
          let filters = run.effectiveParams?.objectValue?["filters"]?.objectValue,
          let metadata = form["__agentlyWindowMetadata"] else { return nil }
    var document = definitionDocument
    if let blocks = form["reportBuilder:\(builder)"]?.objectValue?["reportDocumentBlocks"] { document["blocks"] = blocks }
    var scope: [String: JSONValue] = [:]
    if let prefill = form["prefill"]?.objectValue {
        for (key, value) in prefill { scope[key == "from" ? "From" : key == "to" ? "To" : key] = value }
    }
    if let state = form["reportBuilder:\(builder)"]?.objectValue {
        if let values = state["scopeParams"]?.objectValue {
            for (key, value) in values {
                if key == "dateRange", let dates = value.objectValue { scope["From"] = dates["start"]; scope["To"] = dates["end"] }
                else if value != .null { scope[key] = value }
            }
        }
        for groups in state["dynamicGroups"]?.objectValue?.values ?? Dictionary<String, JSONValue>().values {
            for raw in groups.restoreArray ?? [] {
                guard let group = raw.objectValue, case .string(let key) = group["filterId"], group["enabled"] != .bool(false) else { continue }
                let selections = group["selections"]?.restoreArray?.compactMap { $0.objectValue?["value"] } ?? []
                scope[key] = .array(selections)
            }
        }
    }
    // Empty defaults are harmless; every nonempty effective filter must match
    // the current scope, and every current nonempty scope must be represented.
    func empty(_ value: JSONValue) -> Bool { value == .null || value == .string("") || value == .array([]) }
    for (key, value) in filters where !empty(value) { guard scope[key] == value else { return nil } }
    for (key, value) in scope where !empty(value) { guard filters[key] == value else { return nil } }
    var declarations: [String: [String: JSONValue]] = [:]
    var inheritedEmptyDefaults: [String: JSONValue] = [:]
    func collect(_ value: JSONValue) {
        if let object = value.objectValue {
            if object["dataSources"]?.restoreArray != nil,
               let defaults = object["request"]?.objectValue?["baseParameters"]?.objectValue?["filters"]?.objectValue {
                inheritedEmptyDefaults.merge(defaults.filter { empty($0.value) }, uniquingKeysWith: { _, latest in latest })
            }
            if let variants = object["reportBuilders"]?.objectValue, let selected = variants[builder]?.objectValue?["reportBuilder"] { collect(selected); return }
            if let sources = object["dataSources"]?.restoreArray {
                for raw in sources { if let source = raw.objectValue, case .string(let id) = source["id"] { declarations[id] = source } }
            }
            for (key, child) in object where key != "reportBuilders" { collect(child) }
        } else { (value.restoreArray ?? []).forEach(collect) }
    }
    collect(metadata)
    let snapshotDeclarations = declarations
    if let liveMetadata {
        declarations = [:]
        collect(liveMetadata)
        let liveDeclarations = declarations
        declarations = snapshotDeclarations.mapValues { source in
            guard source["request"] == nil, case .string(let id) = source["id"],
                  let live = liveDeclarations[id], source["dataSourceRef"] == live["dataSourceRef"] else { return source }
            var supplemented = source
            supplemented["request"] = live["request"]
            return supplemented
        }
    }
    var refs = Set<String>()
    func referenced(_ value: JSONValue) {
        if let object = value.objectValue {
            if case .string(let ref) = object["datasetRef"] { refs.insert(ref) }
            object.values.forEach(referenced)
        } else { (value.restoreArray ?? []).forEach(referenced) }
    }
    referenced(.object(document))
    guard !refs.isEmpty, !refs.contains("primary") else { return nil }
    var restored: [JSONValue] = []
    for id in refs.sorted() {
        guard let declared = declarations[id], let request = declared["request"]?.objectValue,
              let native = nativeDatasets.compactMap(\.objectValue).first(where: { $0["id"] == .string(id) }),
              let filled = filledDatasets.compactMap(\.objectValue).first(where: { $0["id"] == .string(id) }),
              native["dataSourceRef"] == declared["dataSourceRef"], native["request"] == filled["request"],
              filled["rows"]?.restoreArray != nil else { return nil }
        var effective = request
        if declared["scope"]?.objectValue?["mode"] == .string("inherit") {
            effective["filters"] = .object(filters.merging(request["filters"]?.objectValue ?? [:], uniquingKeysWith: { _, authored in authored }))
        }
        guard JSONValue.object(effective) == native["request"] else { return nil }
        restored.append(.object(["id": .string(id), "dataSourceRef": declared["dataSourceRef"] ?? .null, "rows": filled["rows"] ?? .array([])]))
    }
    var compiledCharts: [String: [String: JSONValue]] = [:]
    func indexCharts(_ value: JSONValue) {
        if let object = value.objectValue {
            if object["kind"] == .string("chartBlock"), case .string(let id) = object["id"] { compiledCharts[id] = object }
            object.values.forEach(indexCharts)
        } else { (value.restoreArray ?? []).forEach(indexCharts) }
    }
    if let blocks = spec["blocks"] { indexCharts(blocks) }
    func restoreChartModel(_ value: JSONValue) -> JSONValue {
        if var object = value.objectValue {
            if object["kind"] == .string("chartBlock"), object["chartModel"] == nil,
               case .string(let id) = object["id"], let native = compiledCharts[id],
               native["datasetRef"] == object["datasetRef"],
               var compiledSpec = native["chartSpec"]?.objectValue, let authoredSpec = object["chartSpec"]?.objectValue {
                if authoredSpec["title"] == nil { compiledSpec.removeValue(forKey: "title") }
                if compiledSpec == authoredSpec { object["chartModel"] = native["chartModel"] }
            }
            return .object(object.mapValues(restoreChartModel))
        }
        if let values = value.restoreArray { return .array(values.map(restoreChartModel)) }
        return value
    }
    if let currentDefinition = form["reportDefinition"] { form["reportDefinition"] = restoreChartModel(currentDefinition) }
    if var builderState = form["reportBuilder:\(builder)"]?.objectValue, let blocks = builderState["reportDocumentBlocks"] {
        builderState["reportDocumentBlocks"] = restoreChartModel(blocks)
        form["reportBuilder:\(builder)"] = .object(builderState)
    }
    form.removeValue(forKey: "reportRunRequest")
    form["executeOnOpen"] = .bool(false)
    form["reportStaticDatasets"] = .array(restored)
    form["reportMaterialization"] = .object(["id": .string(run.reportRunId), "requestId": .string(run.reportRunId), "status": .string("completed"), "materialized": .bool(true)])
    let restoredDocument = restoreChartModel(.object(document))
    form["reportValidatedRestore"] = .object([
        "runId": .string(run.reportRunId), "builderRef": .string(builder), "primaryRequest": run.effectiveParams ?? .null,
        "authorInputs": .string(nativeReportLocalDigest(.object(reportPreparationAuthorInputs(form.filter { $0.key != "__agentlyWindowMetadata" }.mapValues(\.forgeValue))))),
        "documentFingerprint": .string(nativeReportLocalDigest(restoredDocument.forgeValue)),
        "inheritedEmptyDefaults": .object(inheritedEmptyDefaults),
        "datasets": .array(nativeDatasets.filter { value in if case .string(let id) = value.objectValue?["id"] { return refs.contains(id) }; return false })
    ])
    return form
}

private extension JSONValue { var restoreArray: [JSONValue]? { guard case .array(let values) = self else { return nil }; return values } }
