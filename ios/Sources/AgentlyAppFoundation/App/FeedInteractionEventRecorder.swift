import Foundation
import AgentlySDK
import ForgeIOSRuntime
import OSLog

actor FeedInteractionEventRecorder {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "UIEvent")
    private let client: AgentlyClient
    private var pending: [String: Task<Void, Never>] = [:]

    init(client: AgentlyClient) {
        self.client = client
    }

    func record(_ interaction: ForgeInteraction, conversationID: String) {
        guard interaction.windowKey?.hasPrefix("feed-") == true else { return }
        let normalizedConversationID = conversationID.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalizedConversationID.isEmpty else { return }
        let identity = interaction.detail["field"]?.stringValue
            ?? interaction.detail["tabId"]?.stringValue
            ?? "event"
        let key = "\(interaction.windowID):\(interaction.kind):\(identity)"
        pending[key]?.cancel()
        pending[key] = Task { [client, weak self] in
            if interaction.kind == "feed.form_changed" {
                try? await Task.sleep(nanoseconds: 450_000_000)
            }
            guard !Task.isCancelled else {
                await self?.finish(key)
                return
            }
            var detail = interaction.detail.mapValues(\.appValue)
            if let dataSourceRef = interaction.dataSourceRef {
                detail["dataSourceRef"] = .string(dataSourceRef)
            }
            let output = try? await client.recordUIEvent(
                RecordUIEventInput(
                    conversationId: normalizedConversationID,
                    windowId: interaction.windowID,
                    windowKey: interaction.windowKey,
                    kind: interaction.kind,
                    detail: detail
                )
            )
            #if DEBUG
            self?.logger.debug("kind=\(interaction.kind, privacy: .public) recorded=\(output?.recorded == true, privacy: .public)")
            #endif
            await self?.finish(key)
        }
    }

    private func finish(_ key: String) {
        pending[key] = nil
    }
}
