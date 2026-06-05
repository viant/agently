import Foundation

private let hiddenRouterPayloadKeys: Set<String> = [
    "appendToolBundles",
    "suggestedProfileId",
    "templateId",
    "classification",
    "prompting",
    "directAction",
    "scope",
    "clarificationNeeded",
    "clarificationQuestion"
]

func sanitizeVisibleAssistantText(_ raw: String?) -> String? {
    let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    guard !trimmed.isEmpty else { return nil }
    let stripped = stripHiddenRouterPayload(from: trimmed)
    return stripped.isEmpty ? nil : stripped
}

private func stripHiddenRouterPayload(from text: String) -> String {
    if isHiddenRouterPayloadJSON(text) {
        return ""
    }

    guard let start = text.firstIndex(of: "{"),
          let end = text.lastIndex(of: "}"),
          start < end else {
        return text
    }

    let candidate = String(text[start...end]).trimmingCharacters(in: .whitespacesAndNewlines)
    guard isHiddenRouterPayloadJSON(candidate) else {
        return text
    }

    let prefix = String(text[..<start]).trimmingCharacters(in: .whitespacesAndNewlines)
    return prefix
}

private func isHiddenRouterPayloadJSON(_ text: String) -> Bool {
    guard let data = text.data(using: .utf8),
          let object = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] else {
        return false
    }
    return !hiddenRouterPayloadKeys.isDisjoint(with: object.keys)
}
