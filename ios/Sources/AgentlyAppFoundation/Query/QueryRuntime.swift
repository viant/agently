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
            lastError = error.localizedDescription
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
