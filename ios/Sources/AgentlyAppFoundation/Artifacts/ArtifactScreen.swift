import Foundation
import SwiftUI
import ForgeIOSUI
#if os(iOS) && canImport(QuickLook)
import QuickLook
#endif
#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

public struct ArtifactPreview: Sendable, Equatable, Identifiable {
    public let id: String
    public let name: String
    public let contentType: String?
    public let uri: String?
    public let localFilePath: String?
    public let conversationID: String?
    public let generatedFileID: String?
    public let fileID: String?
    public let sourceLabel: String
    public let text: String?

    public init(
        id: String,
        name: String,
        contentType: String? = nil,
        uri: String? = nil,
        localFilePath: String? = nil,
        conversationID: String? = nil,
        generatedFileID: String? = nil,
        fileID: String? = nil,
        sourceLabel: String = "Conversation",
        text: String? = nil
    ) {
        self.id = id
        self.name = name
        self.contentType = contentType
        self.uri = uri
        self.localFilePath = localFilePath
        self.conversationID = conversationID
        self.generatedFileID = generatedFileID
        self.fileID = fileID
        self.sourceLabel = sourceLabel
        self.text = text
    }
}

public struct ArtifactListView: View {
    let previews: [ArtifactPreview]
    let onSelect: (ArtifactPreview) -> Void

    public init(previews: [ArtifactPreview], onSelect: @escaping (ArtifactPreview) -> Void) {
        self.previews = previews
        self.onSelect = onSelect
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Artifacts")
                .font(.headline)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 10) {
                    ForEach(previews) { preview in
                        Button {
                            onSelect(preview)
                        } label: {
                            VStack(alignment: .leading, spacing: 6) {
                                Text(preview.name)
                                    .font(.subheadline.weight(.semibold))
                                    .lineLimit(1)
                                Text(preview.contentType ?? preview.sourceLabel)
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                            .frame(width: 180, alignment: .leading)
                            .padding(12)
                            .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }
}

public struct ArtifactScreen: View {
    let preview: ArtifactPreview
    #if os(iOS) && canImport(QuickLook)
    @State private var quickLookURL: URL?
    #endif

    public init(preview: ArtifactPreview) {
        self.preview = preview
    }

    public var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                Text(preview.name)
                    .font(.title3.weight(.semibold))
                LabeledContent("Source", value: preview.sourceLabel)
                if let contentType = preview.contentType, !contentType.isEmpty {
                    LabeledContent("Type", value: contentType)
                }
                if let uri = preview.uri, !uri.isEmpty {
                    LabeledContent("URI", value: uri)
                }
                if let localFilePath = preview.localFilePath, !localFilePath.isEmpty {
                    HStack(spacing: 12) {
                        ShareLink(
                            item: URL(fileURLWithPath: localFilePath),
                            preview: SharePreview(preview.name)
                        ) {
                            Label("Share Downloaded File", systemImage: "square.and.arrow.up")
                        }
                        .buttonStyle(.borderedProminent)

                        #if os(iOS) && canImport(QuickLook)
                        Button {
                            quickLookURL = URL(fileURLWithPath: localFilePath)
                        } label: {
                            Label("Quick Look", systemImage: "doc.text.magnifyingglass")
                        }
                        .buttonStyle(.bordered)
                        #endif
                    }
                }
                Divider()
                if let localFilePath = preview.localFilePath,
                   preview.isImageArtifact,
                   let image = loadPlatformArtifactImage(from: localFilePath) {
                    ArtifactImageView(image: image, name: preview.name)
                } else if let textContent = preview.displayTextContent {
                    ArtifactTextContentView(
                        content: textContent,
                        name: preview.name
                    )
                } else {
                    Text("Preview content is not loaded yet for this artifact.")
                        .foregroundStyle(.secondary)
                }
            }
            .padding()
        }
        .navigationTitle("Artifact")
        #if os(iOS) && canImport(QuickLook)
        .quickLookPreview($quickLookURL)
        #endif
    }
}

private extension ArtifactPreview {
    var displayTextContent: ArtifactTextContent? {
        guard let text, !text.isEmpty else { return nil }
        if isMarkdownArtifact {
            return .markdown(text)
        }
        if isCSVArtifact, let table = parsedCSVRows(from: text) {
            return .table(table)
        }
        if isJSONArtifact {
            return .codeBlock(prettyPrintedJSON(text) ?? text)
        }
        return .plain(text)
    }

    var isImageArtifact: Bool {
        let type = contentType?.lowercased() ?? ""
        let path = localFilePath?.lowercased() ?? name.lowercased()
        return type.starts(with: "image/") ||
            path.hasSuffix(".png") ||
            path.hasSuffix(".jpg") ||
            path.hasSuffix(".jpeg") ||
            path.hasSuffix(".gif") ||
            path.hasSuffix(".webp") ||
            path.hasSuffix(".heic")
    }

    var isMarkdownArtifact: Bool {
        let type = contentType?.lowercased() ?? ""
        let path = localFilePath?.lowercased() ?? name.lowercased()
        return type.contains("markdown") ||
            path.hasSuffix(".md") ||
            path.hasSuffix(".markdown")
    }

    var isJSONArtifact: Bool {
        let type = contentType?.lowercased() ?? ""
        let path = localFilePath?.lowercased() ?? name.lowercased()
        return type.contains("json") ||
            path.hasSuffix(".json")
    }

    var isCSVArtifact: Bool {
        let type = contentType?.lowercased() ?? ""
        let path = localFilePath?.lowercased() ?? name.lowercased()
        return type.contains("csv") ||
            path.hasSuffix(".csv")
    }

    func prettyPrintedJSON(_ source: String) -> String? {
        guard let data = source.data(using: .utf8),
              let object = try? JSONSerialization.jsonObject(with: data),
              let formattedData = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
              let formatted = String(data: formattedData, encoding: .utf8) else {
            return nil
        }
        return formatted
    }

    func parsedCSVRows(from source: String) -> [[String]]? {
        let lines = source
            .split(whereSeparator: \.isNewline)
            .map(String.init)
            .filter { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        guard !lines.isEmpty else { return nil }

        let rows = lines.map(parseCSVLine)
        guard rows.contains(where: { $0.count > 1 }) else { return nil }
        return rows
    }

    func parseCSVLine(_ line: String) -> [String] {
        var values: [String] = []
        var current = ""
        var insideQuotes = false
        var iterator = line.makeIterator()

        while let character = iterator.next() {
            switch character {
            case "\"":
                if insideQuotes {
                    if let next = iterator.next() {
                        if next == "\"" {
                            current.append("\"")
                        } else {
                            insideQuotes = false
                            if next == "," {
                                values.append(current)
                                current = ""
                            } else {
                                current.append(next)
                            }
                        }
                    } else {
                        insideQuotes = false
                    }
                } else {
                    insideQuotes = true
                }
            case "," where !insideQuotes:
                values.append(current)
                current = ""
            default:
                current.append(character)
            }
        }

        values.append(current)
        return values.map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
    }
}

private enum ArtifactTextContent {
    case markdown(String)
    case plain(String)
    case codeBlock(String)
    case table([[String]])
}

private struct ArtifactTextContentView: View {
    let content: ArtifactTextContent
    let name: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Preview")
                .font(.headline)
            switch content {
            case .markdown(let markdown):
                MarkdownRenderer(markdown: markdown)
                    .textSelection(.enabled)
            case .plain(let text):
                Text(text)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .textSelection(.enabled)
            case .codeBlock(let text):
                ScrollView(.horizontal, showsIndicators: true) {
                    Text(text)
                        .font(.system(.body, design: .monospaced))
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .textSelection(.enabled)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            case .table(let rows):
                ArtifactCSVTableView(rows: rows)
            }
        }
        .accessibilityLabel(name)
    }
}

private struct ArtifactCSVTableView: View {
    let rows: [[String]]

    private var header: [String] { rows.first ?? [] }
    private var bodyRows: [[String]] { Array(rows.dropFirst().prefix(50)) }

    var body: some View {
        ScrollView([.horizontal, .vertical], showsIndicators: true) {
            VStack(alignment: .leading, spacing: 0) {
                if !header.isEmpty {
                    ArtifactCSVRowView(cells: header, isHeader: true)
                }
                ForEach(Array(bodyRows.enumerated()), id: \.offset) { _, row in
                    ArtifactCSVRowView(cells: row, isHeader: false)
                }
            }
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .stroke(Color.secondary.opacity(0.18), lineWidth: 1)
            )
        }
        .frame(maxWidth: .infinity, minHeight: 160, maxHeight: 360, alignment: .leading)
    }
}

private struct ArtifactCSVRowView: View {
    let cells: [String]
    let isHeader: Bool

    var body: some View {
        HStack(spacing: 0) {
            ForEach(Array(cells.enumerated()), id: \.offset) { _, cell in
                Text(cell.isEmpty ? " " : cell)
                    .font(isHeader ? .caption.weight(.semibold) : .caption)
                    .frame(width: 180, alignment: .leading)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .background(isHeader ? Color.secondary.opacity(0.12) : Color.clear)
                    .overlay(alignment: .trailing) {
                        Rectangle()
                            .fill(Color.secondary.opacity(0.12))
                            .frame(width: 1)
                    }
            }
        }
        .background(
            Rectangle()
                .fill(Color.secondary.opacity(isHeader ? 0.08 : 0.03))
        )
        .overlay(alignment: .bottom) {
            Rectangle()
                .fill(Color.secondary.opacity(0.12))
                .frame(height: 1)
        }
    }
}

#if canImport(UIKit)
private typealias PlatformArtifactImage = UIImage
#elseif canImport(AppKit)
private typealias PlatformArtifactImage = NSImage
#endif

#if canImport(UIKit) || canImport(AppKit)
private func loadPlatformArtifactImage(from path: String) -> PlatformArtifactImage? {
    #if canImport(UIKit)
    return UIImage(contentsOfFile: path)
    #else
    return NSImage(contentsOfFile: path)
    #endif
}

private struct ArtifactImageView: View {
    let image: PlatformArtifactImage
    let name: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Preview")
                .font(.headline)
            #if canImport(UIKit)
            Image(uiImage: image)
                .resizable()
                .scaledToFit()
            #else
            Image(nsImage: image)
                .resizable()
                .scaledToFit()
            #endif
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityLabel(name)
    }
}
#endif
