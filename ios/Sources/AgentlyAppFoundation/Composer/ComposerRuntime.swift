import Foundation
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

@MainActor
public final class ComposerRuntime: ObservableObject {
    @Published public var query: String = ""
    @Published public var attachments: [ComposerAttachmentDraft] = []
    @Published public var attachmentError: String?

    public init() {}

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
}
