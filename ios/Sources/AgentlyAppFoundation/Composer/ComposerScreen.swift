import PhotosUI
import SwiftUI
import UniformTypeIdentifiers

public struct ComposerScreen: View {
    @ObservedObject private var runtime: ComposerRuntime
    @StateObject private var voiceRuntime = ComposerVoiceInputRuntime()
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    let isSending: Bool
    let onSend: () -> Void
    @State private var isShowingFileImporter = false
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    #if os(iOS)
    @State private var isShowingCameraCapture = false
    #endif

    public init(runtime: ComposerRuntime, isSending: Bool = false, onSend: @escaping () -> Void) {
        self.runtime = runtime
        self.isSending = isSending
        self.onSend = onSend
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            TextEditor(text: $runtime.query)
                .frame(minHeight: horizontalSizeClass == .compact ? 96 : 120)
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
        .padding()
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
        VStack(alignment: .leading, spacing: 10) {
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
                    Label("Camera", systemImage: "camera")
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
                Label("Photos", systemImage: "photo.on.rectangle")
            }
            .buttonStyle(.bordered)
            .disabled(isSending)
            Button {
                isShowingFileImporter = true
            } label: {
                Label("Attach", systemImage: "paperclip")
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
