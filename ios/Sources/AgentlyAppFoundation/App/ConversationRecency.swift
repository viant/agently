import Foundation
import AgentlySDK

private let appleConversationDateFormatters: [DateFormatter] = {
    let patterns = [
        "yyyy-MM-dd HH:mm:ss.SSSSSS Z zzz",
        "yyyy-MM-dd HH:mm:ss.SSSSSS Z",
        "yyyy-MM-dd HH:mm:ss.SSS Z zzz",
        "yyyy-MM-dd HH:mm:ss.SSS Z",
        "yyyy-MM-dd HH:mm:ss Z zzz",
        "yyyy-MM-dd HH:mm:ss Z"
    ]
    return patterns.map { pattern in
        let formatter = DateFormatter()
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = pattern
        return formatter
    }
}()

func parseConversationActivityDate(_ rawValue: String?) -> Date? {
    guard let rawValue else { return nil }
    let trimmed = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else { return nil }
    let sanitized = trimmed.replacingOccurrences(
        of: #"\s+m=\+.*$"#,
        with: "",
        options: .regularExpression
    )

    let fractionalFormatter = ISO8601DateFormatter()
    fractionalFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let date = fractionalFormatter.date(from: sanitized) {
        return date
    }

    let fallbackFormatter = ISO8601DateFormatter()
    fallbackFormatter.formatOptions = [.withInternetDateTime]
    if let date = fallbackFormatter.date(from: sanitized) {
        return date
    }

    for formatter in appleConversationDateFormatters {
        if let date = formatter.date(from: sanitized) {
            return date
        }
    }
    return nil
}

func sortedRecentConversations(_ conversations: [Conversation]) -> [Conversation] {
    conversations.sorted(by: isConversation(_:newerThan:))
}

func mergeConversationIntoRecentList(
    _ conversations: [Conversation],
    conversation: Conversation
) -> [Conversation] {
    var merged = conversations.filter { $0.id != conversation.id }
    merged.append(conversation)
    return sortedRecentConversations(merged)
}

private func isConversation(_ lhs: Conversation, newerThan rhs: Conversation) -> Bool {
    let lhsDate = parseConversationActivityDate(lhs.lastActivity ?? lhs.createdAt)
    let rhsDate = parseConversationActivityDate(rhs.lastActivity ?? rhs.createdAt)

    if let lhsDate, let rhsDate {
        if lhsDate != rhsDate {
            return lhsDate > rhsDate
        }
    } else if lhsDate != nil {
        return true
    } else if rhsDate != nil {
        return false
    }

    let lhsTitle = lhs.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let rhsTitle = rhs.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    if lhsTitle != rhsTitle {
        return lhsTitle.localizedCaseInsensitiveCompare(rhsTitle) == .orderedAscending
    }

    return lhs.id < rhs.id
}
