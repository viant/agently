import PhotosUI
import SwiftUI
import UniformTypeIdentifiers
import AgentlySDK

public enum ComposerScreenDensity {
    case regular
    case compact
}

public struct ComposerScreen: View {
    @ObservedObject private var runtime: ComposerRuntime
    @StateObject private var voiceRuntime = ComposerVoiceInputRuntime()
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    let isSending: Bool
    let onSend: () -> Void
    let density: ComposerScreenDensity
    @State private var isShowingFileImporter = false
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    @State private var activeLookupOccurrence: ComposerLookupOccurrence?
    @State private var lookupSearchText: String = ""
    @State private var lookupRows: [[String: JSONValue]] = []
    @State private var lookupErrorMessage: String?
    @State private var lookupRowsLoading = false
    #if os(iOS)
    @State private var isShowingCameraCapture = false
    #endif

    public init(
        runtime: ComposerRuntime,
        isSending: Bool = false,
        density: ComposerScreenDensity = .regular,
        onSend: @escaping () -> Void
    ) {
        self.runtime = runtime
        self.isSending = isSending
        self.density = density
        self.onSend = onSend
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: density == .compact ? 8 : 10) {
            if !runtime.lookupOccurrences.isEmpty {
                composerLookupSection
            }
            TextEditor(text: $runtime.query)
                .frame(height: editorHeight)
            if voiceRuntime.isRecording {
                Label(
                    voiceRuntime.liveTranscript.isEmpty ? "Listening..." : voiceRuntime.liveTranscript,
                    systemImage: "waveform"
                )
                .font(.footnote)
                .foregroundStyle(.secondary)
                .lineLimit(3)
            }
            if let attachmentError = runtime.attachmentError, !attachmentError.isEmpty {
                Text(attachmentError)
                    .font(.footnote)
                    .foregroundStyle(.red)
            }
            if let voiceError = voiceRuntime.errorMessage, !voiceError.isEmpty {
                Text(voiceError)
                    .font(.footnote)
                    .foregroundStyle(.red)
            }
            if !runtime.attachments.isEmpty {
                ScrollView(.horizontal) {
                    HStack(spacing: 8) {
                        ForEach(runtime.attachments) { attachment in
                            HStack(spacing: 6) {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(attachment.name)
                                        .font(.caption.weight(.semibold))
                                        .lineLimit(1)
                                    Text(attachment.mimeType)
                                        .font(.caption2)
                                        .foregroundStyle(.secondary)
                                        .lineLimit(1)
                                }
                                Button {
                                    runtime.removeAttachment(id: attachment.id)
                                } label: {
                                    Image(systemName: "xmark.circle.fill")
                                        .foregroundStyle(.secondary)
                                }
                                .buttonStyle(.plain)
                                .disabled(isSending)
                            }
                            .padding(.horizontal, 10)
                            .padding(.vertical, 6)
                            .background(Color.secondary.opacity(0.08), in: Capsule())
                        }
                    }
                }
            }
            actionSection
        }
        .padding(density == .compact ? 10 : 16)
        .fileImporter(
            isPresented: $isShowingFileImporter,
            allowedContentTypes: [.item],
            allowsMultipleSelection: true
        ) { result in
            switch result {
            case .success(let urls):
                urls.forEach { runtime.importAttachment(from: $0) }
            case .failure(let error):
                runtime.attachmentError = error.localizedDescription
            }
        }
        .onChange(of: selectedPhotoItems) { (_: [PhotosPickerItem], newItems: [PhotosPickerItem]) in
            guard !newItems.isEmpty else { return }
            Task {
                for (index, item) in newItems.enumerated() {
                    do {
                        guard let photoData = try await item.loadTransferable(type: Data.self) else {
                            runtime.attachmentError = "The selected photo could not be loaded."
                            continue
                        }
                        runtime.addPhotoAttachment(
                            data: photoData,
                            contentType: item.supportedContentTypes.first,
                            suggestedIndex: runtime.attachments.count + index + 1
                        )
                    } catch {
                        runtime.attachmentError = error.localizedDescription
                    }
                }
                selectedPhotoItems = []
            }
        }
        .fullScreenCover(item: $activeLookupOccurrence) { occurrence in
            NavigationStack {
                List {
                    if let lookupErrorMessage, !lookupErrorMessage.isEmpty {
                        Text(lookupErrorMessage)
                            .foregroundStyle(.red)
                    }
                    if lookupRowsLoading {
                        HStack(spacing: 10) {
                            ProgressView()
                            Text("Loading \(occurrence.title.lowercased())…")
                                .foregroundStyle(.secondary)
                        }
                    }
                    ForEach(Array(lookupRows.enumerated()), id: \.offset) { _, row in
                        Button {
                            runtime.setLookupSelection(for: occurrence, row: row)
                            activeLookupOccurrence = nil
                        } label: {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(composerLookupRowLabel(row: row, entry: occurrence.entry))
                                    .foregroundStyle(.primary)
                                    .multilineTextAlignment(.leading)
                                if let secondary = composerLookupRowSecondaryText(row: row), !secondary.isEmpty {
                                    Text(secondary)
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                }
                .navigationTitle(occurrence.title)
                .navigationBarTitleDisplayMode(.inline)
                .searchable(text: $lookupSearchText, placement: .navigationBarDrawer(displayMode: .always))
                .task(id: occurrence.key) {
                    lookupSearchText = ""
                    await reloadLookupRows(for: occurrence)
                }
                .task(id: "\(occurrence.key)#\(lookupSearchText)") {
                    await reloadLookupRows(for: occurrence)
                }
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Close") {
                            activeLookupOccurrence = nil
                        }
                    }
                    if runtime.selectionForLookup(occurrence) != nil {
                        ToolbarItem(placement: .topBarTrailing) {
                            Button("Clear") {
                                runtime.clearLookupSelection(for: occurrence)
                                activeLookupOccurrence = nil
                            }
                        }
                    }
                }
            }
        }
        #if os(iOS)
        .sheet(isPresented: $isShowingCameraCapture) {
            CameraCaptureSheet(
                onImageCaptured: { image in
                    if let imageData = image.jpegData(compressionQuality: 0.9) {
                        runtime.addPhotoAttachment(
                            data: imageData,
                            contentType: .jpeg,
                            suggestedIndex: runtime.attachments.count + 1
                        )
                    } else {
                        runtime.attachmentError = "The captured photo could not be encoded."
                    }
                    isShowingCameraCapture = false
                },
                onCancel: {
                    isShowingCameraCapture = false
                }
            )
        }
        #endif
    }

    private var editorHeight: CGFloat {
        switch density {
        case .compact:
            return horizontalSizeClass == .compact ? 74 : 84
        case .regular:
            return horizontalSizeClass == .compact ? 96 : 120
        }
    }

    private var composerLookupSection: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(runtime.lookupOccurrences) { occurrence in
                    let selection = runtime.selectionForLookup(occurrence)
                    Button {
                        activeLookupOccurrence = occurrence
                    } label: {
                        HStack(spacing: 8) {
                            Text(selection?.label ?? occurrence.title)
                                .font(.footnote.weight(selection == nil ? .semibold : .medium))
                                .foregroundStyle(selection == nil ? Color.accentColor : Color.primary)
                                .lineLimit(1)
                            Image(systemName: selection == nil ? "chevron.down.circle.fill" : "chevron.down")
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(selection == nil ? Color.accentColor : .secondary)
                        }
                        .padding(.horizontal, 12)
                        .padding(.vertical, 7)
                        .background(
                            Capsule()
                                .fill(selection == nil ? Color.accentColor.opacity(0.12) : Color.secondary.opacity(0.08))
                        )
                        .overlay(
                            Capsule()
                                .stroke(selection == nil ? Color.accentColor.opacity(0.28) : Color.black.opacity(0.06), lineWidth: 1)
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    @MainActor
    private func reloadLookupRows(for occurrence: ComposerLookupOccurrence) async {
        lookupRowsLoading = true
        lookupErrorMessage = nil
        do {
            lookupRows = try await runtime.loadLookupRows(for: occurrence, query: lookupSearchText)
        } catch {
            lookupRows = []
            lookupErrorMessage = error.localizedDescription
        }
        lookupRowsLoading = false
    }

    @ViewBuilder
    private var actionSection: some View {
        ViewThatFits(in: .horizontal) {
            actionRow
            compactActionColumn
        }
    }

    private var actionRow: some View {
        HStack {
            actionButtons
            if isSending {
                sendingIndicator
            }
            Spacer(minLength: 12)
            sendButton
        }
    }

    private var compactActionColumn: some View {
        VStack(alignment: .leading, spacing: density == .compact ? 8 : 10) {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack {
                    actionButtons
                    if isSending {
                        sendingIndicator
                    }
                }
            }
            HStack {
                Spacer()
                sendButton
            }
        }
    }

    private var actionButtons: some View {
        Group {
            #if os(iOS)
            if UIImagePickerController.isSourceTypeAvailable(.camera) {
                Button {
                    isShowingCameraCapture = true
                } label: {
                    Label(density == .compact ? "Cam" : "Camera", systemImage: "camera")
                }
                .buttonStyle(.bordered)
                .disabled(isSending)
            }
            #endif
            PhotosPicker(
                selection: $selectedPhotoItems,
                maxSelectionCount: 5,
                matching: .images
            ) {
                Label(density == .compact ? "Photos" : "Photos", systemImage: "photo.on.rectangle")
            }
            .buttonStyle(.bordered)
            .disabled(isSending)
            Button {
                isShowingFileImporter = true
            } label: {
                Label(density == .compact ? "Attach" : "Attach", systemImage: "paperclip")
            }
            .buttonStyle(.bordered)
            .disabled(isSending)
            Button {
                voiceRuntime.toggleDictation { recognizedText in
                    runtime.appendRecognizedText(recognizedText)
                }
            } label: {
                Label(
                    voiceRuntime.isRecording ? "Stop" : "Voice",
                    systemImage: voiceRuntime.isRecording ? "stop.circle" : "waveform"
                )
            }
            .buttonStyle(.bordered)
            .tint(voiceRuntime.isRecording ? .red : .accentColor)
            .disabled(isSending)
        }
    }

    private var sendingIndicator: some View {
        HStack(spacing: 6) {
            ProgressView()
                .controlSize(.small)
            Text("Sending...")
                .font(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var sendButton: some View {
        Button(isSending ? "Sending" : "Send", action: onSend)
            .buttonStyle(.borderedProminent)
            .disabled(isSending || runtime.query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
    }
}

private func composerLookupRowLabel(row: [String: JSONValue], entry: LookupRegistryEntry) -> String {
    let template = entry.token?.display?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        ?? entry.display?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        ?? "${name}"
    let rendered = composerLookupApplyTemplate(template, row: row)
    if !rendered.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
        return rendered
    }
    return row["name"]?.stringValue
        ?? row["adOrderName"]?.stringValue
        ?? row["id"]?.stringValue
        ?? "Select"
}

private func composerLookupRowSecondaryText(row: [String: JSONValue]) -> String? {
    let campaign = row["campaignName"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let identifier = row["adOrderId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        ?? row["id"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        ?? ""
    let parts = [campaign, identifier].filter { !$0.isEmpty }
    return parts.isEmpty ? nil : parts.joined(separator: " • ")
}

private func composerLookupApplyTemplate(_ template: String, row: [String: JSONValue]) -> String {
    let pattern = #"\$\{(\w+)\}"#
    guard let regex = try? NSRegularExpression(pattern: pattern) else {
        return template
    }
    var result = template
    let matches = regex.matches(in: template, range: NSRange(template.startIndex..., in: template))
    for match in matches.reversed() {
        guard let keyRange = Range(match.range(at: 1), in: result),
              let replacementRange = Range(match.range(at: 0), in: result) else {
            continue
        }
        let key = String(result[keyRange])
        let replacement = row[key]?.stringValue ?? row[key]?.numberStringValue ?? ""
        result.replaceSubrange(replacementRange, with: replacement)
    }
    return result
}

private extension JSONValue {
    var stringValue: String? {
        switch self {
        case .string(let value):
            return value
        default:
            return nil
        }
    }

    var numberStringValue: String? {
        switch self {
        case .number(let value):
            if value.rounded(.towardZero) == value {
                return String(Int(value))
            }
            return String(value)
        default:
            return nil
        }
    }
}

private extension String {
    var nonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
