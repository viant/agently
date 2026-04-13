import Foundation
import SwiftUI

public enum ComposerMediaCapability: String, Sendable {
    case photo
    case camera
    case voice
}

public struct ComposerMediaController: Sendable {
    public let availableCapabilities: [ComposerMediaCapability]

    public init(availableCapabilities: [ComposerMediaCapability] = [.photo, .camera, .voice]) {
        self.availableCapabilities = availableCapabilities
    }
}

#if os(iOS) && canImport(Speech) && canImport(AVFoundation)
import AVFoundation
import Speech

@MainActor
public final class ComposerVoiceInputRuntime: ObservableObject {
    @Published public private(set) var isRecording = false
    @Published public private(set) var liveTranscript = ""
    @Published public var errorMessage: String?

    private let audioEngine = AVAudioEngine()
    private let recognizer = SFSpeechRecognizer(locale: Locale.current) ?? SFSpeechRecognizer(locale: Locale(identifier: "en-US"))
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private var onCommit: ((String) -> Void)?

    public init() {}

    public func toggleDictation(onCommit: @escaping (String) -> Void) {
        if isRecording {
            stopDictation(commit: true)
            return
        }
        self.onCommit = onCommit
        Task {
            await startDictation()
        }
    }

    private func startDictation() async {
        errorMessage = nil
        liveTranscript = ""

        guard await requestSpeechAuthorization() else {
            errorMessage = "Speech recognition permission is required for voice input."
            return
        }
        guard await requestMicrophoneAuthorization() else {
            errorMessage = "Microphone permission is required for voice input."
            return
        }
        guard let recognizer, recognizer.isAvailable else {
            errorMessage = "Speech recognition is currently unavailable on this device."
            return
        }

        stopDictation(commit: false)

        do {
            let audioSession = AVAudioSession.sharedInstance()
            try audioSession.setCategory(.record, mode: .measurement, options: [.duckOthers])
            try audioSession.setActive(true, options: .notifyOthersOnDeactivation)

            let request = SFSpeechAudioBufferRecognitionRequest()
            request.shouldReportPartialResults = true
            recognitionRequest = request

            let inputNode = audioEngine.inputNode
            let format = inputNode.outputFormat(forBus: 0)
            inputNode.removeTap(onBus: 0)
            inputNode.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
                self?.recognitionRequest?.append(buffer)
            }

            audioEngine.prepare()
            try audioEngine.start()
            isRecording = true

            recognitionTask = recognizer.recognitionTask(with: request) { [weak self] result, error in
                Task { @MainActor in
                    guard let self else { return }
                    if let result {
                        self.liveTranscript = result.bestTranscription.formattedString
                        if result.isFinal {
                            self.stopDictation(commit: true)
                            return
                        }
                    }
                    if let error {
                        self.errorMessage = error.localizedDescription
                        self.stopDictation(commit: false)
                    }
                }
            }
        } catch {
            errorMessage = error.localizedDescription
            stopDictation(commit: false)
        }
    }

    private func stopDictation(commit: Bool) {
        if audioEngine.isRunning {
            audioEngine.stop()
        }
        audioEngine.inputNode.removeTap(onBus: 0)
        recognitionRequest?.endAudio()
        recognitionTask?.cancel()
        recognitionTask = nil
        recognitionRequest = nil
        isRecording = false

        if commit {
            let transcript = liveTranscript.trimmingCharacters(in: .whitespacesAndNewlines)
            if !transcript.isEmpty {
                onCommit?(transcript)
            }
        }

        onCommit = nil

        do {
            try AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
        } catch {
            // Deactivation failures should not block composer use.
        }
    }

    private func requestSpeechAuthorization() async -> Bool {
        await withCheckedContinuation { continuation in
            SFSpeechRecognizer.requestAuthorization { status in
                continuation.resume(returning: status == .authorized)
            }
        }
    }

    private func requestMicrophoneAuthorization() async -> Bool {
        await withCheckedContinuation { continuation in
            AVAudioApplication.requestRecordPermission { granted in
                continuation.resume(returning: granted)
            }
        }
    }
}
#else
@MainActor
public final class ComposerVoiceInputRuntime: ObservableObject {
    @Published public private(set) var isRecording = false
    @Published public private(set) var liveTranscript = ""
    @Published public var errorMessage: String?

    public init() {}

    public func toggleDictation(onCommit: @escaping (String) -> Void) {
        errorMessage = "Voice input is unavailable in this build."
    }
}
#endif
