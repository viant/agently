import Foundation
import ForgeIOSRuntime

/// Matches Forge web's derived authoring catalog. Raw metadata is retained by
/// the caller; this catalog describes declarations, not prepared or loaded rows.
func nativeReportBuilderMetadataSummary(metadata: ForgeIOSRuntime.JSONValue, form: [String: ForgeIOSRuntime.JSONValue]) -> ForgeIOSRuntime.JSONValue? {
    let root = metadata.objectValue?["view"]?.objectValue?["content"] ?? .object([:])
    var candidates: [(String, [String: ForgeIOSRuntime.JSONValue], String)] = []
    func text(_ value: ForgeIOSRuntime.JSONValue?) -> String { value?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? "" }
    func visit(_ value: ForgeIOSRuntime.JSONValue) {
        guard let content = value.objectValue else { return }
        let dashboard = content["dashboard"]?.objectValue ?? [:]
        let requested = text(form["reportBuilderRef"])
        let ref = requested.isEmpty ? text(dashboard["reportBuilderRef"] ?? content["reportBuilderRef"]) : requested
        let variant = dashboard["reportBuilders"]?.objectValue?[ref]?.objectValue
        if let config = variant?["reportBuilder"]?.objectValue ?? dashboard["reportBuilder"]?.objectValue ?? content["reportBuilder"]?.objectValue {
            let label = text(variant?["label"] ?? config["label"] ?? content["title"] ?? .string(ref))
            candidates.append((ref, config, label))
        }
        for child in content["containers"]?.arrayValue ?? [] { visit(child) }
    }
    visit(root)
    var unique: [(String, [String: ForgeIOSRuntime.JSONValue], String)] = []
    for item in candidates where !unique.contains(where: { $0.0 == item.0 && $0.1 == item.1 }) { unique.append(item) }
    guard unique.count == 1 else { return nil }
    let (ref, config, label) = unique[0]
    func fields(_ options: [ForgeIOSRuntime.JSONValue]) -> [ForgeIOSRuntime.JSONValue] {
        var seen = Set<String>()
        return options.compactMap { raw in
            guard let option = raw.objectValue else { return nil }
            let key = text(option["key"] ?? option["value"])
            guard !key.isEmpty, seen.insert(key).inserted else { return nil }
            let suppliedLabel = text(option["label"])
            var field: [String: ForgeIOSRuntime.JSONValue] = ["key": .string(key), "label": .string(suppliedLabel.isEmpty ? key : suppliedLabel)]
            for name in ["kind", "format", "paramPath", "startParamPath", "endParamPath"] { let value = text(option[name]); field[name] = value.isEmpty ? .null : .string(value) }
            for name in ["required", "multiple"] where option[name] == .bool(true) { field[name] = .bool(true) }
            return .object(field)
        }
    }
    let sources: [ForgeIOSRuntime.JSONValue] = (config["dataSources"]?.arrayValue ?? []).compactMap { raw in
        guard let source = raw.objectValue else { return nil }
        let id = text(source["id"])
        guard !id.isEmpty else { return nil }
        var result: [String: ForgeIOSRuntime.JSONValue] = ["id": .string(id)]
        for name in ["dataSourceRef", "description", "kindLabel"] { let value = text(source[name]); result[name] = value.isEmpty ? .null : .string(value) }
        let label = text(source["label"]); result["label"] = .string(label.isEmpty ? id : label)
        for name in ["capabilities", "scope"] where source[name]?.objectValue != nil { result[name] = source[name] }
        result["fields"] = .array(fields(["columnOptions", "chartFieldOptions", "valueFieldOptions", "secondaryFieldOptions"].flatMap { source[$0]?.arrayValue ?? [] }))
        result["scopeParams"] = .array(fields(source["scopeParamOptions"]?.arrayValue ?? []))
        return .object(result)
    }
    return .object([
        "builderRef": ref.isEmpty ? .null : .string(ref), "label": label.isEmpty ? .null : .string(label),
        "authoringContract": .string("Use dataSources[].id as report block datasetRef. dataSources[].dataSourceRef is the underlying execution source. Dataset scope mode inherit follows the active report filters; relativeDateRange overrides them with its declared window."),
        "dataSources": .array(sources)
    ])
}
