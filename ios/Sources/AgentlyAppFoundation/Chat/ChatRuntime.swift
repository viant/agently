import Foundation
import AgentlySDK
import ForgeIOSRuntime

public struct ChatTranscriptEntry: Identifiable, Sendable, Equatable {
    public let id: String
    public let role: String
    public let markdown: String
    public let turnID: String?
    public let renderedParts: [TranscriptCanonicalPart]?
    public let renderedReports: [TranscriptCanonicalReport]?
    public let diagnosticMessages: [String]
    public let timestampLabel: String?
    public let statusLabel: String?

    public init(
        id: String,
        role: String,
        markdown: String,
        turnID: String? = nil,
        renderedParts: [TranscriptCanonicalPart]? = nil,
        renderedReports: [TranscriptCanonicalReport]? = nil,
        diagnosticMessages: [String] = [],
        timestampLabel: String? = nil,
        statusLabel: String? = nil
    ) {
        self.id = id
        self.role = role
        self.markdown = markdown
        self.turnID = turnID
        self.renderedParts = renderedParts
        self.renderedReports = renderedReports
        self.diagnosticMessages = diagnosticMessages
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
                    turnID: entry.turnID,
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
                    turnID: entry.turnID,
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
                    turnID: entry.turnID,
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
                        turnID: turn.turnID,
                        timestampLabel: Self.timestampLabel(for: turn.createdAt)
                    )
                )
            }

            let assistantMessages = [turn.assistant?.narration, turn.assistant?.final].compactMap { $0 }
            // Narration is the active assistant bubble. The global progress card
            // carries only compact status metadata; final content replaces it.
            let finalMarkdown = sanitizeAssistantTranscriptText(turn.assistant?.final?.content) ?? ""
            let narrationFallback = sanitizeAssistantTranscriptText(turn.assistant?.narration?.content) ?? ""
            let assistantMarkdown = finalMarkdown.isEmpty ? narrationFallback : finalMarkdown
            let renderedReports = Self.canonicalAssistantReports(assistantMessages)
            let diagnosticMessages = assistantMessages.flatMap { message in
                message.renderedContent?.diagnostics.map { $0.message.trimmingCharacters(in: .whitespacesAndNewlines) } ?? []
            }.filter { !$0.isEmpty }

            if !assistantMarkdown.isEmpty || renderedReports?.isEmpty == false || !diagnosticMessages.isEmpty {
                next.append(
                    ChatTranscriptEntry(
                        id: turn.assistant?.final?.messageID ?? turn.assistant?.narration?.messageID ?? "\(turn.id)-assistant",
                        role: "assistant",
                        markdown: assistantMarkdown,
                        turnID: turn.turnID,
                        renderedParts: Self.canonicalAssistantParts(assistantMessages),
                        renderedReports: renderedReports,
                        diagnosticMessages: diagnosticMessages,
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
              let activeTurnID = snapshot.activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !activeTurnID.isEmpty,
              let active = assistantEntry(from: snapshot, turnID: activeTurnID, streaming: true) else {
            return transcript
        }
        return transcript.filter { entry in
            entry.id != active.id && !(
                isTransientAssistantEntry(entry) &&
                (entry.turnID?.isEmpty != false || entry.turnID == activeTurnID)
            )
        } + [active]
    }

    @discardableResult
    public func commitAssistantTurn(
        from snapshot: ConversationStreamSnapshot,
        turnID: String
    ) -> Bool {
        let normalizedTurnID = turnID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let completed = Self.assistantEntry(
            from: snapshot,
            turnID: normalizedTurnID,
            streaming: false
        ) else {
            return false
        }
        let replacementIndexes = transcript.indices.filter { index in
            let entry = transcript[index]
            return entry.role.lowercased() == "assistant" && (
                entry.id == completed.id || entry.turnID == normalizedTurnID ||
                ((entry.turnID?.isEmpty ?? true) && Self.isTransientAssistantEntry(entry))
            )
        }
        let insertionIndex = replacementIndexes.first ?? transcript.count
        for index in replacementIndexes.reversed() {
            transcript.remove(at: index)
        }
        transcript.insert(completed, at: min(insertionIndex, transcript.count))
        return true
    }

    private static func activeAssistantEntry(from snapshot: ConversationStreamSnapshot) -> ChatTranscriptEntry? {
        guard let activeTurnID = snapshot.activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !activeTurnID.isEmpty else {
            return nil
        }
        return assistantEntry(from: snapshot, turnID: activeTurnID, streaming: true)
    }

    private struct AssistantTurnFragment {
        let message: BufferedStreamMessage
        let markdown: String
        let rendered: RenderedContent?
    }

    private static func assistantEntry(
        from snapshot: ConversationStreamSnapshot,
        turnID: String,
        streaming: Bool
    ) -> ChatTranscriptEntry? {
        guard !turnID.isEmpty else { return nil }
        let fragments = snapshot.bufferedMessages.compactMap { message -> AssistantTurnFragment? in
            guard message.role.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "assistant",
                  message.turnID?.trimmingCharacters(in: .whitespacesAndNewlines) == turnID else {
                return nil
            }
            let markdown = sanitizeAssistantTranscriptText(message.content)
                ?? sanitizeAssistantTranscriptText(message.narration)
                ?? ""
            let rendered = snapshot.liveExecutionGroupsByID[message.id]?.renderedContent
            guard !markdown.isEmpty || rendered?.reports.isEmpty == false || rendered?.diagnostics.isEmpty == false else { return nil }
            return AssistantTurnFragment(message: message, markdown: markdown, rendered: rendered)
        }
        guard let latest = fragments.last else { return nil }

        let hasCanonicalContent = fragments.contains { fragment in
            fragment.rendered?.parts.isEmpty == false || fragment.rendered?.reports.isEmpty == false || fragment.rendered?.diagnostics.isEmpty == false
        }
        var renderedParts: [TranscriptCanonicalPart]? = nil
        if hasCanonicalContent {
            var parts: [TranscriptCanonicalPart] = []
            for fragment in fragments {
                var fragmentParts = fragment.rendered.map(canonicalParts) ?? []
                if fragmentParts.isEmpty, !fragment.markdown.isEmpty {
                    fragmentParts = [TranscriptCanonicalPart(kind: "markdown", text: fragment.markdown)]
                }
                guard !fragmentParts.isEmpty else { continue }
                if !parts.isEmpty {
                    let first = fragmentParts.removeFirst()
                    if first.kind.lowercased() == "markdown" {
                        let text = String((first.text ?? "").drop(while: { $0.isWhitespace }))
                        parts.append(TranscriptCanonicalPart(
                            kind: first.kind,
                            text: "\n\n\(text)",
                            source: first.source,
                            payload: first.payload,
                            data: first.data
                        ))
                    } else {
                        parts.append(TranscriptCanonicalPart(kind: "markdown", text: "\n\n"))
                        parts.append(first)
                    }
                }
                parts.append(contentsOf: fragmentParts)
            }
            renderedParts = parts
        }

        var reports: [TranscriptCanonicalReport] = []
        var reportIndexes: [String: Int] = [:]
        for report in fragments.flatMap({ $0.rendered.map(canonicalReports) ?? [] }) {
            let identity = "\(report.scope)\u{0}\(report.id)"
            if let index = reportIndexes[identity] {
                reports[index] = report
            } else {
                reportIndexes[identity] = reports.count
                reports.append(report)
            }
        }
        let diagnosticMessages = fragments.flatMap { fragment in
            fragment.rendered?.diagnostics.map { $0.message.trimmingCharacters(in: .whitespacesAndNewlines) } ?? []
        }.filter { !$0.isEmpty }
        return ChatTranscriptEntry(
            id: latest.message.id,
            role: "assistant",
            markdown: fragments.map(\.markdown).filter { !$0.isEmpty }.joined(separator: "\n\n"),
            turnID: turnID,
            renderedParts: renderedParts,
            renderedReports: reports.isEmpty ? nil : reports,
            diagnosticMessages: diagnosticMessages,
            timestampLabel: Self.timestampLabel(for: latest.message.createdAt),
            statusLabel: streaming ? (Self.statusLabel(for: latest.message.status) ?? "Streaming") : nil
        )
    }

    private static func isTransientAssistantEntry(_ entry: ChatTranscriptEntry) -> Bool {
        guard entry.role.lowercased() == "assistant" else { return false }
        return ["waiting", "streaming", "sending"].contains(entry.statusLabel?.lowercased() ?? "")
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
