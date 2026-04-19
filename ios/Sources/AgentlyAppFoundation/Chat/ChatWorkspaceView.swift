import SwiftUI
import AgentlySDK

struct ChatWorkspaceView: View {
    let metadata: WorkspaceMetadata?
    let selectedAgentID: String?
    let availableAgents: [WorkspaceAgentOption]
    let onSelectAgent: (String?) -> Void

    var body: some View {
        EmptyView()
    }
}
