import SwiftUI
import AgentlySDK

public struct HistoryScreen: View {
    let conversations: [Conversation]
    let activeConversationID: String?
    let onSelectConversation: (String) -> Void

    public init(
        conversations: [Conversation],
        activeConversationID: String?,
        onSelectConversation: @escaping (String) -> Void
    ) {
        self.conversations = conversations
        self.activeConversationID = activeConversationID
        self.onSelectConversation = onSelectConversation
    }

    public var body: some View {
        List(conversations) { conversation in
            Button {
                onSelectConversation(conversation.id)
            } label: {
                HStack {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(conversation.title ?? "Untitled")
                        if let summary = conversation.summary {
                            Text(summary)
                                .font(.footnote)
                                .foregroundStyle(.secondary)
                                .lineLimit(2)
                        }
                    }
                    Spacer()
                    if conversation.id == activeConversationID {
                        Image(systemName: "checkmark.circle.fill")
                            .foregroundStyle(.tint)
                    }
                }
            }
            .buttonStyle(.plain)
        }
        .navigationTitle("History")
    }
}
