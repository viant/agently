import Foundation
import AgentlySDK
import UniformTypeIdentifiers

public struct ComposerAttachmentDraft: Identifiable, Sendable {
    public let id: String
    public let name: String
    public let mimeType: String
    public let data: Data
    public let source: String

    public init(id: String = UUID().uuidString, name: String, mimeType: String, data: Data, source: String) {
        self.id = id
        self.name = name
        self.mimeType = mimeType
        self.data = data
        self.source = source
    }
}

public struct ComposerLookupOccurrence: Identifiable, Sendable {
    public let key: String
    public let name: String
    public let title: String
    public let required: Bool
    public let displayRange: Range<String.Index>
    public let entry: LookupRegistryEntry

    public var id: String { key }
}

public struct ComposerLookupSelection: Equatable, Sendable {
    public let token: String
    public let label: String
}

@MainActor
public final class ComposerRuntime: ObservableObject {
    @Published public var query: String = ""
    @Published public var attachments: [ComposerAttachmentDraft] = []
    @Published public var attachmentError: String?
    @Published public private(set) var lookupRegistry: [LookupRegistryEntry] = []
    @Published public private(set) var lookupSelections: [String: ComposerLookupSelection] = [:]

    public var lookupContextKind: String = "chat-composer"
    public var lookupContextID: String = ""

    public typealias LookupRegistryLoader = @Sendable (String, String) async throws -> [LookupRegistryEntry]
    public typealias LookupRowsLoader = @Sendable (LookupRegistryEntry, String) async throws -> [[String: JSONValue]]

    private var lookupRegistryLoader: LookupRegistryLoader?
    private var lookupRowsLoader: LookupRowsLoader?

    public init() {}

    public var lookupOccurrences: [ComposerLookupOccurrence] {
        parseComposerLookupOccurrences(query: query, registry: lookupRegistry)
    }

    public func configureLookupSupport(
        contextKind: String = "chat-composer",
        contextID: String,
        registryLoader: LookupRegistryLoader?,
        rowsLoader: LookupRowsLoader?
    ) async {
        lookupContextKind = contextKind
        lookupContextID = contextID
        lookupRegistryLoader = registryLoader
        lookupRowsLoader = rowsLoader
        await refreshLookupRegistry()
    }

    public func refreshLookupRegistry() async {
        guard let lookupRegistryLoader else {
            lookupRegistry = []
            lookupSelections = [:]
            return
        }
        let trimmedContextID = lookupContextID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedContextID.isEmpty else {
            lookupRegistry = []
            lookupSelections = [:]
            return
        }
        do {
            lookupRegistry = try await lookupRegistryLoader(lookupContextKind, trimmedContextID)
            pruneLookupSelections()
        } catch {
            lookupRegistry = []
            lookupSelections = [:]
        }
    }

    public func loadLookupRows(for occurrence: ComposerLookupOccurrence, query: String) async throws -> [[String: JSONValue]] {
        guard let lookupRowsLoader else { return [] }
        return try await lookupRowsLoader(occurrence.entry, query)
    }

    public func setLookupSelection(for occurrence: ComposerLookupOccurrence, row: [String: JSONValue]) {
        let token = LookupTokens.serializeToken(entry: occurrence.entry, resolved: row.mapValues(\.anyValue))
        let label = LookupTokens.parseTokens(token).first?.label ?? occurrence.title
        lookupSelections[occurrence.key] = ComposerLookupSelection(token: token, label: label)
    }

    public func clearLookupSelection(for occurrence: ComposerLookupOccurrence) {
        lookupSelections.removeValue(forKey: occurrence.key)
    }

    public func selectionForLookup(_ occurrence: ComposerLookupOccurrence) -> ComposerLookupSelection? {
        lookupSelections[occurrence.key]
    }

    public func resolvedQuery() throws -> String {
        let occurrences = lookupOccurrences
        if occurrences.isEmpty {
            return query
        }
        var stored = query
        for occurrence in occurrences.reversed() {
            guard let selection = lookupSelections[occurrence.key] else {
                if occurrence.required {
                    throw ComposerLookupError.unresolvedRequired(occurrence.title)
                }
                continue
            }
            stored.replaceSubrange(occurrence.displayRange, with: selection.token)
        }
        return LookupTokens.flattenStored(stored, registry: lookupRegistry)
    }

    public func addAttachment(_ attachment: ComposerAttachmentDraft) {
        attachments.append(attachment)
        attachmentError = nil
    }

    public func removeAttachment(id: String) {
        attachments.removeAll { $0.id == id }
    }

    public func appendRecognizedText(_ text: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        if query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            query = trimmed
        } else {
            query += "\n" + trimmed
        }
    }

    public func clearAttachments() {
        attachments = []
    }

    public func importAttachment(from url: URL) {
        let hasSecurityScope = url.startAccessingSecurityScopedResource()
        defer {
            if hasSecurityScope {
                url.stopAccessingSecurityScopedResource()
            }
        }

        do {
            let resourceValues = try url.resourceValues(forKeys: [.contentTypeKey, .nameKey])
            let fileData = try Data(contentsOf: url)
            let fileName = resourceValues.name ?? url.lastPathComponent
            let type = resourceValues.contentType
            addAttachment(
                ComposerAttachmentDraft(
                    name: fileName.isEmpty ? "Attachment" : fileName,
                    mimeType: type?.preferredMIMEType ?? "application/octet-stream",
                    data: fileData,
                    source: type?.identifier ?? "file"
                )
            )
        } catch {
            attachmentError = error.localizedDescription
        }
    }

    public func addPhotoAttachment(data: Data, contentType: UTType?, suggestedIndex: Int) {
        let mimeType = contentType?.preferredMIMEType ?? "image/jpeg"
        let fileExtension = contentType?.preferredFilenameExtension ?? "jpg"
        let fileName = "Photo-\(suggestedIndex).\(fileExtension)"
        addAttachment(
            ComposerAttachmentDraft(
                name: fileName,
                mimeType: mimeType,
                data: data,
                source: contentType?.identifier ?? "photo-library"
            )
        )
    }

    private func pruneLookupSelections() {
        let validKeys = Set(lookupOccurrences.map(\.key))
        lookupSelections = lookupSelections.filter { validKeys.contains($0.key) }
    }
}

public enum ComposerLookupError: LocalizedError {
    case unresolvedRequired(String)

    public var errorDescription: String? {
        switch self {
        case .unresolvedRequired(let title):
            return "Resolve required lookup \(title) before sending."
        }
    }
}

private func parseComposerLookupOccurrences(
    query: String,
    registry: [LookupRegistryEntry]
) -> [ComposerLookupOccurrence] {
    guard !query.isEmpty, !registry.isEmpty else { return [] }
    let registryByName = Dictionary(uniqueKeysWithValues: registry.map { ($0.name.lowercased(), $0) })
    let pattern = #"/([a-zA-Z][a-zA-Z0-9_-]*)\b"#
    guard let regex = try? NSRegularExpression(pattern: pattern) else { return [] }
    let nsRange = NSRange(query.startIndex..<query.endIndex, in: query)
    var counters: [String: Int] = [:]
    var results: [ComposerLookupOccurrence] = []
    for match in regex.matches(in: query, range: nsRange) {
        guard let nameRange = Range(match.range(at: 1), in: query),
              let rawRange = Range(match.range(at: 0), in: query) else {
            continue
        }
        let rawName = String(query[nameRange]).lowercased()
        guard let entry = registryByName[rawName] else {
            continue
        }
        let occurrence = counters[rawName, default: 0]
        counters[rawName] = occurrence + 1
        results.append(
            ComposerLookupOccurrence(
                key: "\(rawName)#\(occurrence)",
                name: rawName,
                title: composerLookupTitle(for: entry),
                required: entry.required ?? false,
                displayRange: rawRange,
                entry: entry
            )
        )
    }
    return results
}

private func composerLookupTitle(for entry: LookupRegistryEntry) -> String {
    let explicit = entry.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    if !explicit.isEmpty {
        return explicit
    }
    return entry.name
        .replacingOccurrences(of: "_", with: " ")
        .replacingOccurrences(of: "-", with: " ")
        .split(separator: " ")
        .map { token in
            token.prefix(1).uppercased() + token.dropFirst().lowercased()
        }
        .joined(separator: " ")
}

private extension JSONValue {
    var anyValue: Any {
        switch self {
        case .string(let value):
            return value
        case .number(let value):
            return value
        case .bool(let value):
            return value
        case .object(let value):
            return value.mapValues(\.anyValue)
        case .array(let value):
            return value.map(\.anyValue)
        case .null:
            return NSNull()
        }
    }
}
