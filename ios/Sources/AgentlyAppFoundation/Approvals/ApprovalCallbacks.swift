import Foundation
import AgentlySDK

public enum ApprovalCallbacks {
    public static func resolvedAction(for action: String) -> String {
        action
    }

    public static func execute(
        meta: ApprovalMeta,
        event: String,
        payload: ApprovalCallbackPayload
    ) async -> ApprovalCallbackResult {
        var current = ApprovalCallbackResult(editedFields: payload.editedFields, action: payload.action)
        let callbacks = meta.forge?.callbacks ?? []
        for callback in callbacks {
            guard callback.event?.trimmingCharacters(in: .whitespacesAndNewlines) == event else {
                continue
            }
            let nextPayload = ApprovalCallbackPayload(
                approval: payload.approval,
                editedFields: current.editedFields,
                originalArgs: payload.originalArgs,
                action: current.action
            )
            let result = execute(callback: callback, payload: nextPayload)
            current = mergeCallbackResult(payload: nextPayload, result: result)
        }
        return current
    }

    public static func mergeCallbackResult(
        payload: ApprovalCallbackPayload,
        result: ApprovalCallbackResult
    ) -> ApprovalCallbackResult {
        var editedFields = payload.editedFields
        for (key, value) in result.editedFields {
            editedFields[key] = value
        }
        for (key, value) in result.payload where key != "action" {
            editedFields[key] = value
        }
        let action = result.action
            ?? jsonString(result.payload["action"])
            ?? payload.action
        return ApprovalCallbackResult(
            allow: result.allow,
            message: result.message,
            editedFields: editedFields,
            action: action,
            payload: result.payload
        )
    }

    private static func execute(
        callback: ApprovalCallback,
        payload: ApprovalCallbackPayload
    ) -> ApprovalCallbackResult {
        switch callback.handler?.trimmingCharacters(in: .whitespacesAndNewlines) {
        case "approval.filterEnvNames":
            return filterSelectedArraysToOriginalOrder(payload: payload)
        default:
            return ApprovalCallbackResult(editedFields: payload.editedFields, action: payload.action)
        }
    }

    private static func filterSelectedArraysToOriginalOrder(
        payload: ApprovalCallbackPayload
    ) -> ApprovalCallbackResult {
        var editedFields = payload.editedFields
        for (key, editedValue) in payload.editedFields {
            guard case .array(let selectedValues) = editedValue,
                  case .array(let originalValues)? = payload.originalArgs[key] else {
                continue
            }
            let selectedKeys = Set(selectedValues.map(canonicalJSONKey))
            let orderedSelection = originalValues.filter { selectedKeys.contains(canonicalJSONKey($0)) }
            editedFields[key] = .array(orderedSelection)
        }
        return ApprovalCallbackResult(editedFields: editedFields, action: payload.action)
    }

    private static func jsonString(_ value: JSONValue?) -> String? {
        guard case .string(let string)? = value else { return nil }
        let trimmed = string.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }

    private static func canonicalJSONKey(_ value: JSONValue) -> String {
        switch value {
        case .string(let value):
            return "s:\(value)"
        case .number(let value):
            return "n:\(value)"
        case .bool(let value):
            return "b:\(value)"
        case .null:
            return "z:null"
        case .array(let values):
            return "a:[\(values.map(canonicalJSONKey).joined(separator: ","))]"
        case .object(let values):
            return "o:{\(values.keys.sorted().map { "\($0)=\(canonicalJSONKey(values[$0] ?? .null))" }.joined(separator: ","))}"
        }
    }
}

func buildApprovalDecisionRequest(
    approval: PendingToolApproval,
    action: String,
    editedFields: [String: JSONValue]
) async -> DecideToolApprovalInput {
    let meta = ApprovalMetadataSupport.parsedApprovalMeta(approval)
    let originalArgs: [String: JSONValue]
    if case .object(let arguments)? = approval.arguments {
        originalArgs = arguments
    } else {
        originalArgs = [:]
    }
    let payload = ApprovalCallbackPayload(
        approval: meta,
        editedFields: editedFields,
        originalArgs: originalArgs,
        action: action
    )
    let result: ApprovalCallbackResult
    if let meta {
        result = await ApprovalCallbacks.execute(meta: meta, event: action, payload: payload)
    } else {
        result = ApprovalCallbackResult(editedFields: editedFields, action: action)
    }
    return DecideToolApprovalInput(
        id: approval.id,
        action: result.action ?? action,
        editedFields: result.editedFields
    )
}
