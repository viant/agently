import Foundation
import ForgeIOSRuntime

/// Exposes only the exact completed datasets visible in this report, under their
/// logical IDs. Several datasets can share one physical transport.
func completedReportDataSourceSnapshots(_ cache: NativeReportCompletedDatasets?) -> [String: BridgeJSONValue] {
    guard let cache else { return [:] }
    var snapshots: [String: BridgeJSONValue] = [:]
    for raw in cache.datasets {
        guard let dataset = raw.objectValue, let id = dataset["id"]?.stringValue,
              let physicalRef = dataset["dataSourceRef"]?.stringValue, let request = dataset["request"]?.objectValue,
              case .array(let rows) = dataset["rows"] else { return [:] }
        snapshots[id] = .object([
            "dataSourceRef": .string(physicalRef),
            "input": .object(["parameters": .object(request.mapValues(\.appValue))]),
            "filter": .object((request["filters"]?.objectValue ?? [:]).mapValues(\.appValue)),
            "control": .object(["loading": .bool(false), "resolved": .bool(true)]),
            "form": .object([:]), "selection": .array([]),
            "collection": .array(rows.map(\.appValue)),
            "metrics": .object(["rowCount": .number(Double(rows.count))])
        ])
    }
    return snapshots
}

func registeredReportDataSourceSnapshots(runtime: ForgeRuntime, windowID: String, physicalRefs: [String], admission: NativeReportAdmission?) async -> [String: BridgeJSONValue] {
    func encoded(_ snapshot: RegisteredDataSourceSnapshot, physicalRef: String) -> BridgeJSONValue {
        let input = snapshot.input
        var inputValue: [String: BridgeJSONValue] = ["parameters": .object(input.parameters.mapValues(\.appValue)), "filter": .object(input.filter.mapValues(\.appValue)), "fetch": .bool(input.fetch), "refresh": .bool(input.refresh)]
        if let page = input.page { inputValue["page"] = .number(Double(page)) }
        let control: [String: BridgeJSONValue] = ["loading": .bool(snapshot.control.loading), "inactive": .bool(snapshot.control.inactive), "error": snapshot.control.error.map(BridgeJSONValue.string) ?? .null, "warnings": .array(snapshot.control.warnings.map(BridgeJSONValue.string)), "resolved": .bool(snapshot.collection != nil)]
        return .object([
            "dataSourceRef": .string(physicalRef), "input": .object(inputValue),
            "filter": .object(input.filter.mapValues(\.appValue)), "control": .object(control),
            "form": .object(snapshot.form.mapValues(\.appValue)),
            "selection": .object(["selected": snapshot.selection.selected.map { .object($0.mapValues(\.appValue)) } ?? .null, "selection": .array(snapshot.selection.selection.map { .object($0.mapValues(\.appValue)) }), "rowIndex": .number(Double(snapshot.selection.rowIndex))]),
            "collection": snapshot.collection.map { .array($0.map { .object($0.mapValues(\.appValue)) }) } ?? .null,
            "metrics": .object(snapshot.metrics.mapValues(\.appValue))
        ])
    }
    var result: [String: BridgeJSONValue] = [:]
    for ref in Set(physicalRefs).sorted() {
        if let snapshot = await runtime.registeredDataSourceSnapshot(windowID: windowID, dataSourceRef: ref) {
            result[ref] = encoded(snapshot, physicalRef: ref)
        }
    }
    if let admission {
        for dataset in admission.datasets where result[dataset.id] == nil && !physicalRefs.contains(dataset.id) {
            guard let snapshot = await runtime.registeredDataSourceSnapshot(windowID: windowID, dataSourceRef: "reportDocument:\(dataset.id)"), snapshot.input.parameters == dataset.request else { continue }
            result[dataset.id] = encoded(snapshot, physicalRef: dataset.dataSourceRef)
        }
    }
    return result
}
