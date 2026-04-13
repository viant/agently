import Foundation
import AgentlySDK
import OSLog

@MainActor
public final class ElicitationRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "ElicitationRuntime")
    @Published public var pending: PendingElicitation?
    @Published public var isResolving = false
    @Published public var lastError: String?

    private let client: AgentlyClient

    public init(client: AgentlyClient) {
        self.client = client
    }

    public func refresh(conversationID: String?) async {
        guard let conversationID, !conversationID.isEmpty else {
            pending = nil
            isResolving = false
            return
        }
        do {
            let rows = try await client.listPendingElicitations(
                ListPendingElicitationsInput(conversationID: conversationID)
            )
            pending = rows.first.map {
                let details = Self.decodePendingElicitation(from: $0.elicitation)
                return PendingElicitation(
                    elicitationID: $0.elicitationID,
                    conversationID: $0.conversationID,
                    message: $0.content,
                    mode: details?.mode,
                    url: details?.url,
                    requestedSchema: details?.requestedSchema
                )
            }
            logger.info("Loaded pending elicitation: \(self.pending?.elicitationID ?? "none", privacy: .public)")
            lastError = nil
        } catch {
            logger.error("Elicitation refresh failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
        }
    }

    public func resolve(
        conversationID: String,
        elicitationID: String,
        action: String = "submit",
        data: [String: JSONValue] = [:]
    ) async {
        isResolving = true
        do {
            logger.info("Resolving elicitation \(elicitationID, privacy: .public) with action \(action, privacy: .public)")
            try await client.resolveElicitation(
                ResolveElicitationInput(
                    conversationID: conversationID,
                    elicitationID: elicitationID,
                    action: action,
                    payload: data
                )
            )
            pending = nil
            lastError = nil
        } catch {
            logger.error("Elicitation resolve failed: \(String(describing: error), privacy: .public)")
            lastError = error.localizedDescription
        }
        isResolving = false
    }

    public func dismiss() {
        guard !isResolving else { return }
        pending = nil
    }

    private static func decodePendingElicitation(from value: JSONValue?) -> PendingElicitation? {
        guard case .object(let object)? = value else { return nil }
        return PendingElicitation(
            elicitationID: jsonString(object["elicitationId"]) ?? "",
            conversationID: jsonString(object["conversationId"]),
            message: jsonString(object["message"]),
            mode: jsonString(object["mode"]),
            url: jsonString(object["url"]),
            requestedSchema: object["requestedSchema"]
        )
    }

    private static func jsonString(_ value: JSONValue?) -> String? {
        guard case .string(let string)? = value else { return nil }
        return string
    }
}
