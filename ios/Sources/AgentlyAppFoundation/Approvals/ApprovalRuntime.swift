import Foundation
import AgentlySDK
import OSLog

@MainActor
public final class ApprovalRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "ApprovalRuntime")
    @Published public var approvals: [PendingToolApproval] = []
    @Published public var decidingApprovalID: String?
    @Published public var lastError: String?

    private let client: AgentlyClient

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func refresh(conversationID: String?) async {
        guard let conversationID, !conversationID.isEmpty else {
            approvals = []
            decidingApprovalID = nil
            return
        }
        do {
            approvals = try await client.listPendingToolApprovals(
                ListPendingToolApprovalsInput(conversationID: conversationID, status: "pending", limit: 20)
            )
            logger.info("Loaded \(self.approvals.count, privacy: .public) pending approval(s)")
            lastError = nil
        } catch {
            logger.error("Approval refresh failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
        }
    }

    public func decide(id: String, action: String, editedFields: [String: JSONValue] = [:]) async {
        decidingApprovalID = id
        do {
            logger.info("Submitting approval decision \(action, privacy: .public) for \(id, privacy: .public)")
            try await client.decideToolApproval(
                DecideToolApprovalInput(id: id, action: action, editedFields: editedFields)
            )
            approvals.removeAll { $0.id == id }
            lastError = nil
        } catch {
            logger.error("Approval decision failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
        }
        decidingApprovalID = nil
    }
}
