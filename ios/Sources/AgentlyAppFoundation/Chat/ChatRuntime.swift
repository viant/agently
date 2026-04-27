import Foundation
import AgentlySDK

public struct ChatTranscriptEntry: Identifiable, Sendable, Equatable {
    public let id: String
    public let role: String
    public let markdown: String
    public let timestampLabel: String?
    public let statusLabel: String?

    public init(
        id: String,
        role: String,
        markdown: String,
        timestampLabel: String? = nil,
        statusLabel: String? = nil
    ) {
        self.id = id
        self.role = role
        self.markdown = markdown
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

        transcript.append(
            ChatTranscriptEntry(
                id: assistantEntryID,
                role: "assistant",
                markdown: "(waiting for response)",
                timestampLabel: timestamp,
                statusLabel: "Waiting"
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
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            if entry.id == handle.assistantEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: "Streaming"
                )
            }

            return entry
        }
    }

    public func completeOptimisticTurn(_ handle: OptimisticTurnHandle, response text: String) {
        transcript = transcript.map { entry in
            if entry.id == handle.userEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            if entry.id == handle.assistantEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: "assistant",
                    markdown: text.isEmpty ? "(empty response)" : text,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: nil
                )
            }

            return entry
        }
    }

    public func failOptimisticTurn(_ handle: OptimisticTurnHandle, errorMessage: String? = nil) {
        let normalizedErrorMessage = errorMessage?
            .trimmingCharacters(in: .whitespacesAndNewlines)

        transcript = transcript.map { entry in
            if entry.id == handle.userEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: entry.role,
                    markdown: entry.markdown,
                    timestampLabel: entry.timestampLabel,
                    statusLabel: "Failed"
                )
            }

            if entry.id == handle.assistantEntryID {
                return ChatTranscriptEntry(
                    id: entry.id,
                    role: "assistant",
                    markdown: normalizedErrorMessage?.isEmpty == false
                        ? normalizedErrorMessage!
                        : "The request did not reach a streaming response.",
                    timestampLabel: entry.timestampLabel,
                    statusLabel: "Failed"
                )
            }

            return entry
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

            let assistantMarkdown = [turn.assistant?.narration?.content, turn.assistant?.final?.content]
                .compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
                .joined(separator: "\n\n")

            if !assistantMarkdown.isEmpty {
                next.append(
                    ChatTranscriptEntry(
                        id: turn.assistant?.final?.messageID ?? turn.assistant?.narration?.messageID ?? "\(turn.id)-assistant",
                        role: "assistant",
                        markdown: assistantMarkdown,
                        timestampLabel: Self.timestampLabel(for: turn.createdAt)
                    )
                )
            }
        }
        transcript = next
    }

    public func applyStreaming(snapshot: ConversationStreamSnapshot) {
        var next = transcript.filter { entry in
            !snapshot.bufferedMessages.contains(where: { $0.id == entry.id }) &&
            !snapshot.liveExecutionGroupsByID.keys.contains(entry.id) &&
            entry.statusLabel != "Waiting"
        }

        let streamEntries = streamingEntries(from: snapshot)

        next.append(contentsOf: streamEntries)
        transcript = next
    }

    private func streamingEntries(from snapshot: ConversationStreamSnapshot) -> [ChatTranscriptEntry] {
        if !snapshot.liveExecutionGroupsByID.isEmpty {
            return snapshot.liveExecutionGroupsByID.values
                .sorted { lhs, rhs in
                    if lhs.sequence != rhs.sequence {
                        return (lhs.sequence ?? 0) < (rhs.sequence ?? 0)
                    }
                    if lhs.createdAt != rhs.createdAt {
                        return (lhs.createdAt ?? "") < (rhs.createdAt ?? "")
                    }
                    return lhs.pageID < rhs.pageID
                }
                .map { group in
                    let markdown = [group.narration, group.content]
                        .compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }
                        .filter { !$0.isEmpty }
                        .joined(separator: "\n\n")
                    return ChatTranscriptEntry(
                        id: group.pageID,
                        role: "assistant",
                        markdown: markdown.isEmpty ? "(waiting for response)" : markdown,
                        timestampLabel: Self.timestampLabel(for: group.createdAt),
                        statusLabel: Self.statusLabel(for: group.status)
                    )
                }
        }

        return snapshot.bufferedMessages.map { message -> ChatTranscriptEntry in
            let markdown = [message.narration, message.content]
                .compactMap { $0?.trimmingCharacters(in: .whitespacesAndNewlines) }
                .filter { !$0.isEmpty }
                .joined(separator: "\n\n")
            return ChatTranscriptEntry(
                id: message.id,
                role: "assistant",
                markdown: markdown.isEmpty ? "(waiting for response)" : markdown,
                timestampLabel: Self.timestampLabel(for: Date()),
                statusLabel: Self.statusLabel(for: message.status)
            )
        }
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
}
