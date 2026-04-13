import Foundation
import AgentlySDK

enum ApprovalMetadataSupport {
    static func parsedApprovalMeta(_ approval: PendingToolApproval) -> ApprovalMeta? {
        guard let metaValue = approval.metadata,
              case .object(let rootObject) = metaValue else { return nil }
        let candidates: [AppJSONValue] = {
            var values: [AppJSONValue] = [metaValue]
            if case .object(let nestedApproval)? = rootObject["approval"] {
                values.append(.object(nestedApproval))
            }
            return values
        }()

        for candidate in candidates {
            if let direct = decodeApprovalMetaCandidate(candidate), hasMeaningfulApprovalMeta(direct) {
                return direct
            }
            if case .object(let object) = candidate,
               looksLikeApprovalMeta(object),
               let normalized = normalizeApprovalMeta(object) {
                return normalized
            }
        }
        return nil
    }

    static func extractToolApprovalMeta(_ requestedSchema: [String: AppJSONValue]?) -> ApprovalMeta? {
        guard let requestedSchema else { return nil }
        let properties = schemaProperties(in: requestedSchema)
        let rawMeta = readSchemaConst(properties, "_approvalMeta")
        if !rawMeta.isEmpty,
           let meta = normalizeToolApprovalMeta(rawMeta) {
            return meta
        }
        guard readSchemaConst(properties, "_type") == "tool_approval" else {
            return nil
        }
        return ApprovalMeta(
            type: "tool_approval",
            toolName: nonBlank(readSchemaFieldValue(properties, "_toolName")),
            title: nonBlank(readSchemaFieldValue(properties, "_title")) ?? "Approval Required",
            acceptLabel: nonBlank(readSchemaFieldValue(properties, "_acceptLabel")) ?? "Allow",
            rejectLabel: nonBlank(readSchemaFieldValue(properties, "_rejectLabel")) ?? "Decline",
            cancelLabel: nonBlank(readSchemaFieldValue(properties, "_cancelLabel")) ?? "Cancel"
        )
    }

    static func approvalDataSourceRef(_ meta: ApprovalMeta) -> String {
        let trimmed = meta.forge?.dataSource?.trimmingCharacters(in: .whitespacesAndNewlines)
        return (trimmed?.isEmpty == false ? trimmed! : "approvalForm")
    }

    private static func decodeApprovalMetaCandidate(_ candidate: AppJSONValue) -> ApprovalMeta? {
        guard let data = try? JSONEncoder.agently().encode(candidate) else { return nil }
        return try? JSONDecoder.agently().decode(ApprovalMeta.self, from: data)
    }

    private static func hasMeaningfulApprovalMeta(_ meta: ApprovalMeta) -> Bool {
        !((meta.toolName ?? "").isEmpty &&
          (meta.title ?? "").isEmpty &&
          (meta.message ?? "").isEmpty &&
          (meta.editors ?? []).isEmpty &&
          meta.forge == nil)
    }

    private static func looksLikeApprovalMeta(_ candidate: [String: AppJSONValue]) -> Bool {
        let value: [String: AppJSONValue]
        if case .object(let nestedApproval)? = candidate["approval"] {
            value = nestedApproval
        } else {
            value = candidate
        }
        return value["toolName"] != nil ||
            value["title"] != nil ||
            value["message"] != nil ||
            value["editors"] != nil ||
            value["forge"] != nil
    }

    private static func normalizeApprovalMeta(_ candidate: [String: AppJSONValue]) -> ApprovalMeta? {
        let value: [String: AppJSONValue]
        if case .object(let nestedApproval)? = candidate["approval"] {
            value = nestedApproval
        } else {
            value = candidate
        }
        let type = primitiveString(value["type"]) ?? ""
        if !type.isEmpty && type != "tool_approval" {
            return nil
        }
        return ApprovalMeta(
            type: "tool_approval",
            toolName: nonBlank(primitiveString(value["toolName"])),
            title: nonBlank(primitiveString(value["title"])),
            message: nonBlank(primitiveString(value["message"])),
            acceptLabel: nonBlank(primitiveString(value["acceptLabel"])),
            rejectLabel: nonBlank(primitiveString(value["rejectLabel"])),
            cancelLabel: nonBlank(primitiveString(value["cancelLabel"])),
            forge: normalizeApprovalForgeView(value["forge"]),
            editors: decodeApprovalEditors(value["editors"])
        )
    }

    private static func normalizeApprovalForgeView(_ candidate: AppJSONValue?) -> ApprovalForgeView? {
        guard case .object(let object)? = candidate else { return nil }
        return ApprovalForgeView(
            windowRef: nonBlank(primitiveString(object["windowRef"])),
            containerRef: nonBlank(primitiveString(object["containerRef"])),
            dataSource: nonBlank(primitiveString(object["dataSource"])),
            callbacks: decodeApprovalCallbacks(object["callbacks"])
        )
    }

    private static func decodeApprovalCallbacks(_ candidate: AppJSONValue?) -> [ApprovalCallback] {
        guard case .array(let values)? = candidate else { return [] }
        return values.compactMap { value in
            guard case .object(let object) = value else { return nil }
            return ApprovalCallback(
                event: nonBlank(primitiveString(object["event"])),
                handler: nonBlank(primitiveString(object["handler"])),
                args: objectValue(object["args"])
            )
        }
    }

    private static func decodeApprovalEditors(_ candidate: AppJSONValue?) -> [ApprovalEditor] {
        guard case .array(let values)? = candidate else { return [] }
        return values.compactMap { value in
            guard case .object(let object) = value,
                  let name = nonBlank(primitiveString(object["name"])) else {
                return nil
            }
            return ApprovalEditor(
                name: name,
                kind: nonBlank(primitiveString(object["kind"])) ?? "checkbox_list",
                path: nonBlank(primitiveString(object["path"])),
                label: nonBlank(primitiveString(object["label"])),
                description: nonBlank(primitiveString(object["description"])),
                options: decodeApprovalOptions(object["options"])
            )
        }
    }

    private static func decodeApprovalOptions(_ candidate: AppJSONValue?) -> [ApprovalOption] {
        guard case .array(let values)? = candidate else { return [] }
        return values.compactMap { value in
            guard case .object(let object) = value,
                  let id = nonBlank(primitiveString(object["id"])),
                  let label = nonBlank(primitiveString(object["label"])) else {
                return nil
            }
            return ApprovalOption(
                id: id,
                label: label,
                description: nonBlank(primitiveString(object["description"])),
                item: object["item"],
                selected: primitiveBool(object["selected"]) ?? true
            )
        }
    }

    private static func normalizeToolApprovalMeta(_ raw: String) -> ApprovalMeta? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty,
              let data = trimmed.data(using: .utf8) else {
            return nil
        }
        return try? JSONDecoder.agently().decode(ApprovalMeta.self, from: data)
    }

    private static func schemaProperties(in schema: [String: AppJSONValue]) -> [String: AppJSONValue] {
        guard case .object(let properties)? = schema["properties"] else { return [:] }
        return properties
    }

    private static func readSchemaConst(_ properties: [String: AppJSONValue], _ key: String) -> String {
        nonBlank(readSchemaFieldValue(properties, key)) ?? ""
    }

    private static func readSchemaFieldValue(_ properties: [String: AppJSONValue], _ key: String) -> String? {
        guard case .object(let field)? = properties[key] else { return nil }
        if let value = nonBlank(primitiveString(field["const"])) {
            return value
        }
        if let value = nonBlank(primitiveString(field["default"])) {
            return value
        }
        if case .array(let values)? = field["enum"] {
            return values.compactMap(primitiveString).first
        }
        return nil
    }

    private static func primitiveString(_ value: AppJSONValue?) -> String? {
        guard case .string(let string)? = value else { return nil }
        return string
    }

    private static func primitiveBool(_ value: AppJSONValue?) -> Bool? {
        guard case .bool(let bool)? = value else { return nil }
        return bool
    }

    private static func objectValue(_ value: AppJSONValue?) -> [String: AppJSONValue]? {
        guard case .object(let object)? = value else { return nil }
        return object
    }

    private static func nonBlank(_ value: String?) -> String? {
        guard let trimmed = value?.trimmingCharacters(in: .whitespacesAndNewlines),
              !trimmed.isEmpty else {
            return nil
        }
        return trimmed
    }
}
