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
    @State private var lastAutoPresentedLookupSignature = ""
    @FocusState private var isEditorFocused: Bool
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
                .font(.body)
                .focused($isEditorFocused)
                .disabled(isSending)
                .autocorrectionDisabled(true)
                #if os(iOS)
                .textInputAutocapitalization(.never)
                #endif
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .background(
                    RoundedRectangle(cornerRadius: 18)
                        .fill(composerInputBackground)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 18)
                        .stroke(composerInputStroke, lineWidth: isEditorFocused ? 1.5 : 1)
                )
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
        .onAppear {
            presentFirstRequiredLookupIfNeeded()
        }
        .onChange(of: unresolvedRequiredLookupSignature) { _, _ in
            presentFirstRequiredLookupIfNeeded()
        }
        .onChange(of: isSending) { _, newValue in
            if newValue {
                isEditorFocused = false
                requestAgentlyPlatformKeyboardDismissal()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: .agentlyKeyboardDismissalRequested)) { _ in
            isEditorFocused = false
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
        let minimum: CGFloat
        let maximum: CGFloat
        switch density {
        case .compact:
            minimum = horizontalSizeClass == .compact ? 96 : 108
            maximum = horizontalSizeClass == .compact ? 220 : 250
        case .regular:
            minimum = horizontalSizeClass == .compact ? 110 : 130
            maximum = horizontalSizeClass == .compact ? 240 : 280
        }
        let lineHeight: CGFloat = 23
        let contentHeight = CGFloat(estimatedEditorLineCount) * lineHeight + 28
        return min(max(minimum, contentHeight), maximum)
    }

    private var estimatedEditorLineCount: Int {
        let charactersPerLine: Int
        switch density {
        case .compact:
            charactersPerLine = horizontalSizeClass == .compact ? 30 : 42
        case .regular:
            charactersPerLine = horizontalSizeClass == .compact ? 34 : 56
        }
        let lines = runtime.query
            .components(separatedBy: .newlines)
            .map { line in
                max(1, Int(ceil(Double(line.count) / Double(charactersPerLine))))
            }
            .reduce(0, +)
        return max(3, lines)
    }

    private var composerInputBackground: Color {
        if isEditorFocused {
            return Color(red: 0.94, green: 0.98, blue: 0.94)
        }
        return Color.agentlySecondarySystemBackground.opacity(0.5)
    }

    private var composerInputStroke: Color {
        if isEditorFocused {
            return Color(red: 0.42, green: 0.76, blue: 0.50).opacity(0.9)
        }
        return Color.secondary.opacity(0.16)
    }

    @ViewBuilder
    private var composerLookupSection: some View {
        if density == .compact {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(runtime.lookupOccurrences) { occurrence in
                    compactLookupButton(occurrence)
                }
            }
        } else {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(runtime.lookupOccurrences) { occurrence in
                        lookupChip(occurrence)
                    }
                }
            }
        }
    }

    private func lookupChip(_ occurrence: ComposerLookupOccurrence) -> some View {
        let selection = runtime.selectionForLookup(occurrence)
        return Button {
            activeLookupOccurrence = occurrence
        } label: {
            HStack(spacing: 8) {
                Text(selection?.label ?? occurrence.title)
                    .font(.footnote.weight(selection == nil ? .semibold : .medium))
                    .foregroundStyle(selection == nil ? Color.accentColor : Color.primary)
                    .lineLimit(1)
                Image(systemName: selection == nil ? "chevron.down.circle.fill" : "chevron.down")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(selection == nil ? Color.accentColor : Color.secondary)
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

    private func compactLookupButton(_ occurrence: ComposerLookupOccurrence) -> some View {
        let selection = runtime.selectionForLookup(occurrence)
        return Button {
            activeLookupOccurrence = occurrence
        } label: {
            HStack(spacing: 10) {
                Image(systemName: selection == nil ? "magnifyingglass" : "checkmark.circle.fill")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(selection == nil ? Color.accentColor : Color.green)
                    .frame(width: 24)
                VStack(alignment: .leading, spacing: 2) {
                    Text(selection?.label ?? "Select \(occurrence.title)")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)
                    Text(occurrence.required && selection == nil ? "Required" : occurrence.title)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer(minLength: 8)
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(Color.secondary)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color.accentColor.opacity(selection == nil ? 0.10 : 0.06), in: RoundedRectangle(cornerRadius: 8))
            .overlay(
                RoundedRectangle(cornerRadius: 8)
                    .stroke(Color.accentColor.opacity(selection == nil ? 0.26 : 0.12), lineWidth: 1)
            )
        }
        .buttonStyle(.plain)
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
        Button(sendButtonTitle, action: handleSendTap)
            .buttonStyle(.borderedProminent)
            .disabled(isSending || runtime.query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
    }

    private var sendButtonTitle: String {
        if isSending {
            return "Sending"
        }
        if let occurrence = firstUnresolvedRequiredLookup {
            return "Select \(occurrence.title)"
        }
        return "Send"
    }

    private var firstUnresolvedRequiredLookup: ComposerLookupOccurrence? {
        runtime.lookupOccurrences.first { occurrence in
            occurrence.required && runtime.selectionForLookup(occurrence) == nil
        }
    }

    private var unresolvedRequiredLookupSignature: String {
        runtime.lookupOccurrences
            .filter { occurrence in
                occurrence.required && runtime.selectionForLookup(occurrence) == nil
            }
            .map(\.key)
            .joined(separator: "|")
    }

    private func handleSendTap() {
        if let occurrence = firstUnresolvedRequiredLookup {
            activeLookupOccurrence = occurrence
            return
        }
        isEditorFocused = false
        requestAgentlyPlatformKeyboardDismissal()
        onSend()
    }

    private func presentFirstRequiredLookupIfNeeded() {
        guard density == .compact else { return }
        let signature = unresolvedRequiredLookupSignature
        guard !signature.isEmpty,
              signature != lastAutoPresentedLookupSignature,
              activeLookupOccurrence == nil,
              let occurrence = firstUnresolvedRequiredLookup else {
            return
        }
        lastAutoPresentedLookupSignature = signature
        activeLookupOccurrence = occurrence
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
        ?? row["id"]?.stringValue
        ?? "Select"
}

private func composerLookupRowSecondaryText(row: [String: JSONValue]) -> String? {
    let group = row["groupName"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let identifier = row["entityId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        ?? row["id"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        ?? ""
    let parts = [group, identifier].filter { !$0.isEmpty }
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
