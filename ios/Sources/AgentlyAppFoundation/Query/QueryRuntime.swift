import Foundation
import AgentlySDK
import OSLog

@MainActor
public final class QueryRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "QueryRuntime")
    @Published public var isSending: Bool = false
    @Published public var lastError: String?

    private let client: AgentlyClient
    private var acceptedWhileSending = false

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func send(
        conversationID: String?,
        agentID: String?,
        query: String,
        attachments: [QueryAttachment] = [],
        context: [String: JSONValue] = [:]
    ) async -> QueryOutput? {
        isSending = true
        acceptedWhileSending = false
        defer {
            isSending = false
            acceptedWhileSending = false
        }
        do {
            logger.info("Submitting query request")
            lastError = nil
            return try await client.query(
                QueryInput(
                    conversationID: conversationID,
                    agentID: agentID,
                    query: query,
                    attachments: attachments,
                    context: context
                )
            )
        } catch {
            if acceptedWhileSending {
                logger.info("Ignoring query transport error after workspace acceptance: \(String(describing: error), privacy: .public)")
                lastError = nil
                return QueryOutput()
            }
            logger.error("Query request failed: \(String(describing: error), privacy: .public)")
            lastError = visibleQueryError(error)
            return nil
        }
    }

    public func markAccepted() {
        guard isSending else { return }
        acceptedWhileSending = true
        isSending = false
        lastError = nil
    }
}

internal func visibleQueryError(_ error: Error) -> String {
    let detail = error.localizedDescription.trimmingCharacters(in: .whitespacesAndNewlines)
    let diagnostic = "\(String(describing: error)) \(detail)".lowercased()
    if diagnostic.contains("api key is required") {
        return "The workspace model is not configured. Ask an administrator to add the model API key, then try again."
    }
    if diagnostic.contains("eof") || diagnostic.contains("connection ended") {
        return "The connection ended before the report finished loading. Refresh to try again."
    }
    if diagnostic.contains("/v1/agent/query") || diagnostic.contains("failed to stream") {
        return "The assistant could not start this request. Try again, or contact the workspace administrator if it continues."
    }
    return detail.isEmpty ? "The assistant could not start this request. Try again." : detail
}
