import Foundation
import AgentlySDK
import ForgeIOSRuntime

public struct ChatTranscriptEntry: Identifiable, Sendable, Equatable {
    public let id: String
    public let role: String
    public let markdown: String
    public let renderedParts: [TranscriptCanonicalPart]?
    public let renderedReports: [TranscriptCanonicalReport]?
    public let timestampLabel: String?
    public let statusLabel: String?

    public init(
        id: String,
        role: String,
        markdown: String,
        renderedParts: [TranscriptCanonicalPart]? = nil,
        renderedReports: [TranscriptCanonicalReport]? = nil,
        timestampLabel: String? = nil,
        statusLabel: String? = nil
    ) {
        self.id = id
        self.role = role
        self.markdown = markdown
        self.renderedParts = renderedParts
        self.renderedReports = renderedReports
        self.timestampLabel = timestampLabel
        self.statusLabel = statusLabel
    }
}

public struct OptimisticTurnHandle: Sendable, Equatable {
    public let userEntryID: String
    public let assistantEntryID: String

    public init(userEntryID: String, assistantEntryID: String) {
        self.userEntryID = userEntryID
        self.assistantEntryID = assistantEntryID
    }
}

@MainActor
public final class ChatRuntime: ObservableObject {
    @Published public var transcript: [ChatTranscriptEntry] = []

    public init() {}

    public func beginOptimisticTurn(query text: String) -> OptimisticTurnHandle {
        let userEntryID = UUID().uuidString
        let assistantEntryID = UUID().uuidString
        let timestamp = Self.timestampLabel(for: Date())

        transcript.append(
            ChatTranscriptEntry(
                id: userEntryID,
                role: "user",
                markdown: text,
                timestampLabel: timestamp,
                statusLabel: "Sending"
            )
        )

        return OptimisticTurnHandle(
            userEntryID: userEntryID,
            assistantEntryID: assistantEntryID
        )
    }

    public func appendUserMessage(_ text: String) {
        transcript.append(
            ChatTranscriptEntry(
                id: UUID().uuidString,
                role: "user",
                markdown: text,
                timestampLabel: Self.timestampLabel(for: Date())
            )
        )
    }

    public func markOptimisticTurnAccepted(_ handle: OptimisticTurnHandle) {
        transcript = transcript.map { entry in
            if entry.id == handle.userEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    renderedParts: entry.renderedParts,
                    renderedReports: entry.renderedReports,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            return entry
        }
    }

    public func completeOptimisticTurn(_ handle: OptimisticTurnHandle, response text: String) {
        var resolvedAssistant = false
        transcript = transcript.map { entry in
            if entry.id == handle.userEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    renderedParts: entry.renderedParts,
                    renderedReports: entry.renderedReports,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            if entry.id == handle.assistantEntryID {
                resolvedAssistant = true
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: "assistant",
                    markdown: text.isEmpty ? "(empty response)" : text,
                    renderedParts: nil,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            return entry
        }
        if !resolvedAssistant {
            transcript.append(ChatTranscriptEntry(
                id: handle.assistantEntryID,
                role: "assistant",
                markdown: text.isEmpty ? "(empty response)" : text,
                timestampLabel: Self.timestampLabel(for: Date())
            ))
        }
    }

    public func failOptimisticTurn(_ handle: OptimisticTurnHandle, errorMessage: String? = nil) {
        let normalizedErrorMessage = errorMessage?
            .trimmingCharacters(in: .whitespacesAndNewlines)

        var resolvedAssistant = false
        transcript = transcript.map { entry in
            if entry.id == handle.userEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    renderedParts: entry.renderedParts,
                    renderedReports: entry.renderedReports,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: "Failed"
                )
            }

            if entry.id == handle.assistantEntryID {
                resolvedAssistant = true
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: "assistant",
                    markdown: normalizedErrorMessage?.isEmpty == false
                        ? normalizedErrorMessage!
                        : "The request did not reach a streaming response.",
                    renderedParts: nil,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: "Failed"
                )
            }

            return entry
        }
        if !resolvedAssistant {
            transcript.append(ChatTranscriptEntry(
                id: handle.assistantEntryID,
                role: "assistant",
                markdown: normalizedErrorMessage?.isEmpty == false
                    ? normalizedErrorMessage!
                    : "The request did not reach a streaming response.",
                timestampLabel: Self.timestampLabel(for: Date()),
                statusLabel: "Failed"
            ))
        }
    }

    public func appendAssistantMessage(_ text: String) {
        transcript.append(
            ChatTranscriptEntry(
                id: UUID().uuidString,
                role: "assistant",
                markdown: text,
                timestampLabel: Self.timestampLabel(for: Date())
            )
        )
    }

    public func replaceTranscript(from state: ConversationStateResponse) {
        let turns = state.conversation?.turns ?? []
        var next: [ChatTranscriptEntry] = []
        for turn in turns {
            if let user = turn.user?.content?.trimmingCharacters(in: .whitespacesAndNewlines),
               !user.isEmpty {
                next.append(
                    ChatTranscriptEntry(
                        id: turn.user?.messageID ?? "\(turn.id)-user",
                        role: "user",
                        markdown: user,
                        timestampLabel: Self.timestampLabel(for: turn.createdAt)
                    )
                )
            }

            let assistantMessages = [turn.assistant?.narration, turn.assistant?.final].compactMap { $0 }
            // Narration is live progress, not a second assistant answer. While a
            // turn is active it belongs exclusively in the global progress card;
            // after completion the final message wins. Keep narration only as a
            // compatibility fallback for terminal turns that have no final text.
            let finalMarkdown = sanitizeAssistantTranscriptText(turn.assistant?.final?.content) ?? ""
            let narrationFallback = Self.isPendingStatus(turn.status)
                ? ""
                : (sanitizeAssistantTranscriptText(turn.assistant?.narration?.content) ?? "")
            let assistantMarkdown = finalMarkdown.isEmpty ? narrationFallback : finalMarkdown
            let renderedReports = Self.canonicalAssistantReports(assistantMessages)

            if !assistantMarkdown.isEmpty || renderedReports?.isEmpty == false {
                next.append(
                    ChatTranscriptEntry(
                        id: turn.assistant?.final?.messageID ?? turn.assistant?.narration?.messageID ?? "\(turn.id)-assistant",
                        role: "assistant",
                        markdown: assistantMarkdown,
                        renderedParts: Self.canonicalAssistantParts(assistantMessages),
                        renderedReports: renderedReports,
                        timestampLabel: Self.timestampLabel(for: turn.createdAt)
                    )
                )
            }
        }
        transcript = next
    }

    public func latestAssistantMarkdown(snapshot: ConversationStreamSnapshot) -> String? {
        Self.activeAssistantEntry(from: snapshot)?.markdown
    }

    public func transcriptWithActiveAssistant(
        snapshot: ConversationStreamSnapshot?
    ) -> [ChatTranscriptEntry] {
        Self.transcriptWithActiveAssistant(transcript, snapshot: snapshot)
    }

    public static func transcriptWithActiveAssistant(
        _ transcript: [ChatTranscriptEntry],
        snapshot: ConversationStreamSnapshot?
    ) -> [ChatTranscriptEntry] {
        guard let snapshot,
              let active = activeAssistantEntry(from: snapshot) else {
            return transcript
        }
        return transcript.filter { entry in
            entry.id != active.id &&
            entry.statusLabel != "Waiting" &&
            entry.statusLabel != "Streaming"
        } + [active]
    }

    private static func activeAssistantEntry(from snapshot: ConversationStreamSnapshot) -> ChatTranscriptEntry? {
        guard let activeTurnID = snapshot.activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !activeTurnID.isEmpty else {
            return nil
        }

        guard let message = snapshot.bufferedMessages.reversed().first(where: { message in
            message.role.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "assistant" &&
                message.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == activeTurnID &&
                (message.content?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false ||
                 snapshot.liveExecutionGroupsByID[message.id]?.renderedContent?.reports.isEmpty == false)
        }) else {
            return nil
        }
        let markdown = [message.content]
            .compactMap(sanitizeAssistantTranscriptText)
            .filter { !$0.isEmpty }
            .joined(separator: "\n\n")
        let rendered = snapshot.liveExecutionGroupsByID[message.id]?.renderedContent
        guard !markdown.isEmpty || rendered?.reports.isEmpty == false else {
            return nil
        }
        return ChatTranscriptEntry(
            id: message.id,
            role: "assistant",
            markdown: markdown,
            renderedParts: rendered.map(canonicalParts),
            renderedReports: rendered.map(canonicalReports),
            timestampLabel: Self.timestampLabel(for: message.createdAt),
            statusLabel: Self.statusLabel(for: message.status) ?? "Streaming"
        )
    }

    private static func canonicalAssistantParts(_ messages: [AssistantMessageState]) -> [TranscriptCanonicalPart]? {
        guard messages.contains(where: { $0.renderedContent != nil }) else { return nil }
        var result: [TranscriptCanonicalPart] = []
        for message in messages {
            var messageParts: [TranscriptCanonicalPart]
            if let rendered = message.renderedContent {
                messageParts = canonicalParts(from: rendered)
            } else if let text = sanitizeAssistantTranscriptText(message.content), !text.isEmpty {
                messageParts = [TranscriptCanonicalPart(kind: "markdown", text: text)]
            } else {
                messageParts = []
            }
            guard !messageParts.isEmpty else { continue }
            if !result.isEmpty {
                let first = messageParts.removeFirst()
                if first.kind.lowercased() == "markdown" {
                    let text = String((first.text ?? "").drop(while: { $0.isWhitespace }))
                    result.append(
                        TranscriptCanonicalPart(
                            kind: first.kind,
                            text: "\n\n\(text)",
                            source: first.source,
                            payload: first.payload,
                            data: first.data
                        )
                    )
                } else {
                    result.append(TranscriptCanonicalPart(kind: "markdown", text: "\n\n"))
                    result.append(first)
                }
            }
            result.append(contentsOf: messageParts)
        }
        return result
    }

    private static func canonicalAssistantReports(_ messages: [AssistantMessageState]) -> [TranscriptCanonicalReport]? {
        let reports = messages.flatMap { message in
            message.renderedContent.map(canonicalReports) ?? []
        }
        return reports.isEmpty ? nil : reports
    }

    private static func canonicalParts(from rendered: RenderedContent) -> [TranscriptCanonicalPart] {
        rendered.parts.map { part in
            TranscriptCanonicalPart(
                kind: part.kind,
                text: part.text,
                source: part.source,
                payload: forgeJSONValue(part.payload),
                data: part.data.map {
                    TranscriptCanonicalData(
                        version: $0.version,
                        scope: $0.scope,
                        reportRef: $0.reportRef,
                        sequence: $0.sequence,
                        id: $0.id,
                        format: $0.format,
                        mode: $0.mode,
                        payload: forgeJSONValue($0.payload)
                    )
                }
            )
        }
    }

    private static func canonicalReports(from rendered: RenderedContent) -> [TranscriptCanonicalReport] {
        rendered.reports.compactMap { report in
            guard let source = forgeJSONValue(report.source) else { return nil }
            let dataSources = Dictionary(uniqueKeysWithValues: report.dataSources.map { id, data in
                (id, TranscriptCanonicalData(
                    version: data.version,
                    scope: data.scope,
                    reportRef: data.reportRef,
                    sequence: data.sequence,
                    id: data.id,
                    format: data.format,
                    mode: data.mode,
                    payload: forgeJSONValue(data.payload)
                ))
            })
            return TranscriptCanonicalReport(
                scope: report.scope,
                id: report.id,
                grammar: report.grammar ?? "dashboard-v1",
                status: report.status,
                sequence: report.sequence,
                resetVersion: report.resetVersion,
                source: source,
                dataSources: dataSources
            )
        }
    }

    private static func forgeJSONValue(_ value: AgentlySDK.JSONValue?) -> ForgeIOSRuntime.JSONValue? {
        guard let value,
              let encoded = try? JSONEncoder().encode(value) else {
            return nil
        }
        return try? JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: encoded)
    }

    private static func timestampLabel(for value: Date) -> String {
        DateFormatter.localizedString(from: value, dateStyle: .none, timeStyle: .short)
    }

    private static func timestampLabel(for rawValue: String?) -> String? {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines),
              !rawValue.isEmpty else {
            return nil
        }

        let fractionalFormatter = ISO8601DateFormatter()
        fractionalFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = fractionalFormatter.date(from: rawValue) {
            return timestampLabel(for: date)
        }

        let fallbackFormatter = ISO8601DateFormatter()
        fallbackFormatter.formatOptions = [.withInternetDateTime]
        if let date = fallbackFormatter.date(from: rawValue) {
            return timestampLabel(for: date)
        }

        return nil
    }

    private static func statusLabel(for rawValue: String?) -> String? {
        guard let rawValue = rawValue?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased(),
              !rawValue.isEmpty else {
            return nil
        }
        switch rawValue {
        case "running":
            return "Streaming"
        case "completed":
            return nil
        case "failed":
            return "Failed"
        case "canceled":
            return "Canceled"
        default:
            return rawValue.capitalized
        }
    }

    private static func isPendingStatus(_ rawValue: String?) -> Bool {
        guard let normalized = rawValue?
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .lowercased(), !normalized.isEmpty else {
            return false
        }
        return [
            "queued", "pending", "starting", "started", "running",
            "streaming", "processing", "waiting", "waiting_for_model",
            "waiting_for_tool", "waiting_for_user"
        ].contains(normalized)
    }
}

func sanitizeAssistantTranscriptText(_ value: String?) -> String? {
    guard let value else { return nil }
    return sanitizeVisibleAssistantText(
        TranscriptEnvelope.suppressProgressiveTransport(in: value)
    )
}
