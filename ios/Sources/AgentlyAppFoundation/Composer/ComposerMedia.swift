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
    @Published public private(set) var isPreparing = false
    @Published public private(set) var liveTranscript = ""
    @Published public var errorMessage: String?

    public var isActive: Bool { isPreparing || isRecording }

    private let audioEngine = AVAudioEngine()
    private let recognizer = SFSpeechRecognizer(locale: Locale(identifier: "en-US"))
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private var onCommit: ((String) -> Void)?
    private var recognitionGeneration: UUID?

    public init() {}

    public func toggleDictation(onCommit: @escaping (String) -> Void) {
        if isActive {
            stopDictation(commit: true)
            return
        }
        stopDictation(commit: false)
        self.onCommit = onCommit
        isPreparing = true
        errorMessage = nil
        liveTranscript = ""
        Task {
            await startDictation()
        }
    }

    private func startDictation() async {
        guard await requestSpeechAuthorization() else {
            isPreparing = false
            errorMessage = "Speech recognition permission is required for voice input."
            onCommit = nil
            return
        }
        guard await requestMicrophoneAuthorization() else {
            isPreparing = false
            errorMessage = "Microphone permission is required for voice input."
            onCommit = nil
            return
        }
        guard let recognizer, recognizer.isAvailable else {
            isPreparing = false
            errorMessage = "Speech recognition is currently unavailable on this device."
            onCommit = nil
            return
        }

        do {
            let audioSession = AVAudioSession.sharedInstance()
            try audioSession.setCategory(.record, mode: .measurement, options: [.duckOthers])
            try audioSession.setActive(true, options: .notifyOthersOnDeactivation)

            let request = SFSpeechAudioBufferRecognitionRequest()
            request.shouldReportPartialResults = true
            request.requiresOnDeviceRecognition = false
            recognitionRequest = request

            let inputNode = audioEngine.inputNode
            let format = inputNode.outputFormat(forBus: 0)
            inputNode.removeTap(onBus: 0)
            inputNode.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
                self?.recognitionRequest?.append(buffer)
            }

            audioEngine.prepare()
            try audioEngine.start()
            isPreparing = false
            isRecording = true

            let generation = UUID()
            recognitionGeneration = generation
            recognitionTask = recognizer.recognitionTask(with: request) { [weak self] result, error in
                Task { @MainActor in
                    guard let self, self.recognitionGeneration == generation else { return }
                    if let result {
                        self.liveTranscript = result.bestTranscription.formattedString
                        if result.isFinal {
                            self.stopDictation(commit: true)
                            return
                        }
                    }
                    if let error {
                        let hasPartialTranscript = !self.liveTranscript
                            .trimmingCharacters(in: .whitespacesAndNewlines)
                            .isEmpty
                        if !hasPartialTranscript {
                            self.errorMessage = self.voiceRecognitionErrorMessage(error)
                        }
                        self.stopDictation(commit: hasPartialTranscript)
                    }
                }
            }
        } catch {
            errorMessage = voiceRecognitionErrorMessage(error)
            stopDictation(commit: false)
        }
    }

    private func stopDictation(commit: Bool) {
        if audioEngine.isRunning {
            audioEngine.stop()
        }
        audioEngine.inputNode.removeTap(onBus: 0)
        recognitionRequest?.endAudio()
        let task = recognitionTask
        recognitionTask = nil
        recognitionGeneration = nil
        recognitionRequest = nil
        task?.cancel()
        isPreparing = false
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

    private func voiceRecognitionErrorMessage(_ error: Error) -> String {
        let nsError = error as NSError
        if nsError.domain == "kAFAssistantErrorDomain", nsError.code == 203 {
            return "I didn't catch that. Tap the microphone to try again."
        }
        return "Voice input stopped. Tap the microphone to try again."
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
    @Published public private(set) var isPreparing = false
    @Published public private(set) var liveTranscript = ""
    @Published public var errorMessage: String?

    public var isActive: Bool { isPreparing || isRecording }

    public init() {}

    public func toggleDictation(onCommit: @escaping (String) -> Void) {
        errorMessage = "Voice input is unavailable in this build."
    }
}
#endif
