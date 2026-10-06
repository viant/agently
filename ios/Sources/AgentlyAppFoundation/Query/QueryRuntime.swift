import Foundation
import AgentlySDK
import OSLog

@MainActor
public final class QueryRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "QueryRuntime")
    @Published public var isSending: Bool = false
    @Published public var lastError: String?

    private let client: AgentlyClient

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func send(
        conversationID: String?,
        messageID: String? = nil,
        agentID: String?,
        query: String,
        attachments: [QueryAttachment] = [],
        resourceURIs: [String] = [],
        context: [String: JSONValue] = [:]
    ) async -> QueryOutput? {
        isSending = true
        defer {
            isSending = false
        }
        do {
            logger.info("Submitting query request")
            lastError = nil
            return try await client.query(
                QueryInput(
                    conversationID: conversationID,
                    messageID: messageID,
                    agentID: agentID,
                    query: query,
                    attachments: attachments,
                    resourceURIs: resourceURIs.isEmpty ? nil : resourceURIs,
                    context: context
                )
            )
        } catch {
            logger.error("Query request failed: \(String(describing: error), privacy: .public)")
            lastError = visibleQueryError(error)
            return nil
        }
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
