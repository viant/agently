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
    @State private var isCompactComposerExpanded = false
    @State private var editorSelectionUTF16Offset = 0
    @State private var dictationInsertionUTF16Offset = 0
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
            if density == .compact && canShowCollapsedCompactComposer && !isCompactComposerExpanded {
                collapsedCompactComposer
            } else {
            composerInput
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
        .onChange(of: unresolvedRequiredLookupSignature) { _, _ in
            if !unresolvedRequiredLookupSignature.isEmpty {
                isCompactComposerExpanded = true
            }
        }
        .onChange(of: runtime.query) { oldValue, newValue in
            if !newValue.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                isCompactComposerExpanded = true
            }
            let newLength = (newValue as NSString).length
            if oldValue.isEmpty || editorSelectionUTF16Offset > newLength {
                editorSelectionUTF16Offset = newLength
            }
        }
        .onChange(of: runtime.attachments.count) { _, count in
            if count > 0 {
                isCompactComposerExpanded = true
            }
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
        .agentlyLookupPresentation(item: $activeLookupOccurrence) { occurrence in
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
                .agentlyInlineTitleMode()
                .agentlyLookupSearchable(text: $lookupSearchText)
                .task(id: occurrence.key) {
                    lookupSearchText = ""
                    await reloadLookupRows(for: occurrence)
                }
                .task(id: "\(occurrence.key)#\(lookupSearchText)") {
                    await reloadLookupRows(for: occurrence)
                }
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("Close") {
                            activeLookupOccurrence = nil
                        }
                    }
                    if runtime.selectionForLookup(occurrence) != nil {
                        ToolbarItem(placement: .primaryAction) {
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

    private var canShowCollapsedCompactComposer: Bool {
        runtime.query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            && runtime.attachments.isEmpty
            && runtime.lookupOccurrences.isEmpty
            && !voiceRuntime.isActive
            && !isSending
    }

    private var collapsedCompactComposer: some View {
        HStack(spacing: 8) {
            Button {
                isCompactComposerExpanded = true
                isEditorFocused = true
            } label: {
                Text("Message")
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.vertical, 8)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Compose message")
            .accessibilityIdentifier("agently-composer-expand")

            Button {
                isCompactComposerExpanded = true
                beginVoiceInput()
            } label: {
                AppleToolbarActionIcon(
                    systemImage: "waveform",
                    color: Color(red: 0.87, green: 0.36, blue: 0.48),
                    isLoading: voiceRuntime.isPreparing
                )
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Voice input")
        }
    }

    private var composerInput: some View {
        VStack(alignment: .leading, spacing: 8) {
            if !runtime.lookupOccurrences.isEmpty {
                composerLookupSection
            }
            ZStack(alignment: .topLeading) {
                #if os(iOS)
                ComposerQueryEditor(
                    text: $runtime.query,
                    selectionUTF16Offset: $editorSelectionUTF16Offset,
                    occurrences: runtime.lookupOccurrences,
                    isDisabled: isSending,
                    isFocused: $isEditorFocused
                )
                #else
                TextEditor(text: $runtime.query)
                    .font(.body)
                    .focused($isEditorFocused)
                    .disabled(isSending)
                    .accessibilityIdentifier("agently-composer-editor")
                    .autocorrectionDisabled(true)
                #endif
                if visibleEditorText.isEmpty {
                    Text(runtime.lookupOccurrences.isEmpty ? "Message" : "Add details")
                        .font(.body)
                        .foregroundStyle(.secondary)
                        .padding(.top, 1)
                        .allowsHitTesting(false)
                }
            }
            .frame(height: editorHeight)

            if voiceRuntime.isActive {
                Button(action: beginVoiceInput) {
                    HStack(spacing: 10) {
                        Image(systemName: voiceRuntime.isRecording ? "waveform" : "mic")
                            .foregroundStyle(Color.accentColor)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(voiceRuntime.liveTranscript.isEmpty
                                ? (voiceRuntime.isPreparing ? "Starting microphone…" : "Listening…")
                                : voiceRuntime.liveTranscript)
                                .font(.footnote.weight(.semibold))
                                .foregroundStyle(.primary)
                                .lineLimit(3)
                            Text("Speak naturally · tap to stop")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Spacer(minLength: 4)
                        Image(systemName: "stop.circle.fill")
                            .foregroundStyle(.red)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 8)
                    .background(Color.accentColor.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Stop voice input")
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(
            RoundedRectangle(cornerRadius: 18)
                .fill(composerInputBackground)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 18)
                .stroke(composerInputStroke, lineWidth: isEditorFocused ? 1.5 : 1)
        )
    }

    private var visibleEditorText: String {
        ComposerEditorProjection(
            source: runtime.query,
            occurrences: runtime.lookupOccurrences
        ).display
    }

    private var editorHeight: CGFloat {
        composerEditorHeight(
            query: runtime.query,
            density: density,
            horizontalSizeClass: horizontalSizeClass
        )
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
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 8) {
                ForEach(runtime.lookupOccurrences) { occurrence in
                    lookupChip(occurrence)
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
        .accessibilityIdentifier("agently-composer-lookup-\(occurrence.key)")
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
                    Text(composerLookupControlLabel(title: occurrence.title, selection: selection))
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
        actionRow
    }

    private var actionRow: some View {
        HStack(spacing: 10) {
            Spacer(minLength: 0)
            actionButtons
            sendButton
        }
    }

    private var actionButtons: some View {
        Group {
            if hasDraftContent {
                Button {
                    runtime.clearDraft()
                    editorSelectionUTF16Offset = 0
                    isEditorFocused = false
                    requestAgentlyPlatformKeyboardDismissal()
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "xmark.circle.fill",
                        color: Color(red: 0.74, green: 0.29, blue: 0.35)
                    )
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear composer")
                .accessibilityIdentifier("agently-composer-clear")
                .disabled(isSending)
            }
            if density == .compact {
                Button {
                    isEditorFocused = false
                    requestAgentlyPlatformKeyboardDismissal()
                    isCompactComposerExpanded = false
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "chevron.down",
                        color: Color(red: 0.36, green: 0.40, blue: 0.48)
                    )
                }
                .buttonStyle(.plain)
                .disabled(!canShowCollapsedCompactComposer)
                .accessibilityLabel("Collapse composer")
                .accessibilityIdentifier("agently-composer-collapse")
            }
            #if os(iOS)
            if isEditorFocused {
                Button {
                    isEditorFocused = false
                    requestAgentlyPlatformKeyboardDismissal()
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "keyboard.chevron.compact.down",
                        color: Color(red: 0.36, green: 0.40, blue: 0.48)
                    )
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Hide keyboard")
                .accessibilityIdentifier("agently-composer-hide-keyboard")
            }
            #endif
            PhotosPicker(
                selection: $selectedPhotoItems,
                maxSelectionCount: 5,
                matching: .images
            ) {
                AppleToolbarActionIcon(
                    systemImage: "photo.on.rectangle.fill",
                    color: Color(red: 0.10, green: 0.45, blue: 0.95)
                )
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Add photos")
            .disabled(isSending)
            #if os(iOS)
            if UIImagePickerController.isSourceTypeAvailable(.camera) {
                Button {
                    isShowingCameraCapture = true
                } label: {
                    AppleToolbarActionIcon(
                        systemImage: "camera.fill",
                        color: Color(red: 0.88, green: 0.54, blue: 0.12)
                    )
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Take photo")
                .disabled(isSending)
            }
            #endif
            Button {
                isShowingFileImporter = true
            } label: {
                AppleToolbarActionIcon(
                    systemImage: "paperclip",
                    color: Color(red: 0.49, green: 0.32, blue: 0.88)
                )
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Attach file")
            .disabled(isSending)
            Button {
                beginVoiceInput()
            } label: {
                AppleToolbarActionIcon(
                    systemImage: voiceRuntime.isActive ? "stop.fill" : "waveform",
                    color: Color(red: 0.87, green: 0.36, blue: 0.48),
                    isLoading: voiceRuntime.isPreparing
                )
            }
            .buttonStyle(.plain)
            .accessibilityLabel(voiceRuntime.isActive ? "Stop voice input" : "Voice input")
            .disabled(isSending)
        }
    }

    private var hasDraftContent: Bool {
        !runtime.query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ||
            !runtime.attachments.isEmpty ||
            !runtime.lookupSelections.isEmpty
    }

    private var sendButton: some View {
        Button(action: handleSendTap) {
            AppleToolbarActionIcon(
                systemImage: firstUnresolvedRequiredLookup == nil ? "arrow.up" : "magnifyingglass",
                color: Color(red: 0.22, green: 0.23, blue: 0.86),
                isLoading: isSending
            )
        }
            .buttonStyle(.plain)
            .accessibilityLabel(sendButtonTitle)
            .accessibilityIdentifier("agently-composer-send")
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
        NotificationCenter.default.post(name: .agentlyComposerCommitRequested, object: nil)
        isEditorFocused = false
        requestAgentlyPlatformKeyboardDismissal()
        onSend()
    }

    private func beginVoiceInput() {
        if !voiceRuntime.isActive {
            dictationInsertionUTF16Offset = editorSelectionUTF16Offset
            isEditorFocused = false
            requestAgentlyPlatformKeyboardDismissal()
            isCompactComposerExpanded = true
        }
        voiceRuntime.toggleDictation { recognizedText in
            editorSelectionUTF16Offset = runtime.insertRecognizedText(
                recognizedText,
                atUTF16Offset: dictationInsertionUTF16Offset
            )
        }
    }

}

internal func composerEditorHeight(
    query: String,
    density: ComposerScreenDensity,
    horizontalSizeClass: UserInterfaceSizeClass?
) -> CGFloat {
    let horizontalCompact = horizontalSizeClass == .compact
    let minimum: CGFloat
    let maximum: CGFloat
    switch density {
    case .compact:
        minimum = horizontalCompact ? 54 : 58
        maximum = horizontalCompact ? 220 : 250
    case .regular:
        minimum = horizontalCompact ? 72 : 82
        maximum = horizontalCompact ? 240 : 280
    }
    let lineHeight: CGFloat = 23
    let contentHeight = CGFloat(estimatedComposerEditorLineCount(
        query: query,
        density: density,
        horizontalCompact: horizontalCompact
    )) * lineHeight + 28
    return min(max(minimum, contentHeight), maximum)
}

private func estimatedComposerEditorLineCount(
    query: String,
    density: ComposerScreenDensity,
    horizontalCompact: Bool
) -> Int {
    let charactersPerLine: Int
    switch density {
    case .compact:
        charactersPerLine = horizontalCompact ? 30 : 42
    case .regular:
        charactersPerLine = horizontalCompact ? 34 : 56
    }
    let trimmedQuery = query.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmedQuery.isEmpty else {
        return 1
    }
    let lines = trimmedQuery
        .components(separatedBy: .newlines)
        .map { line in
            max(1, Int(ceil(Double(line.count) / Double(charactersPerLine))))
        }
        .reduce(0, +)
    return max(1, lines)
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
