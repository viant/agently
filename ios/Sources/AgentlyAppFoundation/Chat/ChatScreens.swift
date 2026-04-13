import SwiftUI
import AgentlySDK
import ForgeIOSRuntime

public struct ChatScreens: View {
    @ObservedObject private var runtime: AppRuntime
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    public init(runtime: AppRuntime) {
        self.runtime = runtime
    }

    public var body: some View {
        Group {
            if horizontalSizeClass == .compact {
                PhoneWorkspaceScreen(
                    metadata: runtime.state.workspaceMetadata,
                    selectedAgentID: runtime.selectedAgentOption?.id,
                    availableAgents: runtime.availableAgentOptions,
                    transcript: runtime.chatRuntime.transcript,
                    artifacts: runtime.state.artifacts,
                    composerRuntime: runtime.composerRuntime,
                    isSending: runtime.isQueryBusy,
                    isLoadingConversation: runtime.state.isLoadingConversation,
                    isLoadingArtifacts: runtime.state.isLoadingArtifacts,
                    queryError: runtime.queryRuntime.lastError,
                    activeTurnID: runtime.state.activeTurnID,
                    isStoppingTurn: runtime.state.isStoppingTurn,
                    streamError: runtime.state.streamErrorMessage,
                    approvals: runtime.approvalRuntime.approvals,
                    decidingApprovalID: runtime.approvalRuntime.decidingApprovalID,
                    approvalError: runtime.approvalRuntime.lastError,
                    pendingElicitation: runtime.elicitationRuntime.pending,
                    isResolvingElicitation: runtime.elicitationRuntime.isResolving,
                    elicitationError: runtime.elicitationRuntime.lastError,
                    artifactError: runtime.state.artifactErrorMessage,
                    onSend: { Task { await runtime.sendCurrentQuery() } },
                    onCancelTurn: { Task { await runtime.cancelActiveTurn() } },
                    onRetryStreaming: { Task { await runtime.retryLiveUpdates() } },
                    onSelectArtifact: { artifact in
                        runtime.selectArtifact(artifact)
                    },
                    onDecision: { approval, action, editedFields in
                        Task {
                            await runtime.decideApproval(approval, action: action,
                                                         editedFields: editedFields)
                        }
                    },
                    onResolveElicitation: { action, payload in
                        Task {
                            await runtime.resolvePendingElicitation(action: action, payload: payload)
                        }
                    },
                    onDismissElicitation: {
                        runtime.elicitationRuntime.dismiss()
                    },
                    onSelectAgent: { agentID in
                        runtime.selectPreferredAgent(agentID)
                    },
                    forgeRuntime: runtime.state.forgeRuntime
                )
            } else {
                WorkspaceScreen(
                    metadata: runtime.state.workspaceMetadata,
                    selectedAgentID: runtime.selectedAgentOption?.id,
                    availableAgents: runtime.availableAgentOptions,
                    transcript: runtime.chatRuntime.transcript,
                    artifacts: runtime.state.artifacts,
                    composerRuntime: runtime.composerRuntime,
                    isSending: runtime.isQueryBusy,
                    isLoadingConversation: runtime.state.isLoadingConversation,
                    isLoadingArtifacts: runtime.state.isLoadingArtifacts,
                    queryError: runtime.queryRuntime.lastError,
                    activeTurnID: runtime.state.activeTurnID,
                    isStoppingTurn: runtime.state.isStoppingTurn,
                    streamError: runtime.state.streamErrorMessage,
                    approvals: runtime.approvalRuntime.approvals,
                    decidingApprovalID: runtime.approvalRuntime.decidingApprovalID,
                    approvalError: runtime.approvalRuntime.lastError,
                    pendingElicitation: runtime.elicitationRuntime.pending,
                    isResolvingElicitation: runtime.elicitationRuntime.isResolving,
                    elicitationError: runtime.elicitationRuntime.lastError,
                    artifactError: runtime.state.artifactErrorMessage,
                    onSend: { Task { await runtime.sendCurrentQuery() } },
                    onCancelTurn: { Task { await runtime.cancelActiveTurn() } },
                    onRetryStreaming: { Task { await runtime.retryLiveUpdates() } },
                    onSelectArtifact: { artifact in
                        runtime.selectArtifact(artifact)
                    },
                    onDecision: { approval, action, editedFields in
                        Task {
                            await runtime.decideApproval(approval, action: action,
                                                         editedFields: editedFields)
                        }
                    },
                    onResolveElicitation: { action, payload in
                        Task {
                            await runtime.resolvePendingElicitation(action: action, payload: payload)
                        }
                    },
                    onDismissElicitation: {
                        runtime.elicitationRuntime.dismiss()
                    },
                    onSelectAgent: { agentID in
                        runtime.selectPreferredAgent(agentID)
                    },
                    forgeRuntime: runtime.state.forgeRuntime
                )
            }
        }
        .sheet(item: Binding(
            get: { runtime.state.selectedArtifact },
            set: { if $0 == nil { runtime.selectArtifact(nil) } }
        )) { preview in
            NavigationStack {
                ArtifactScreen(preview: preview)
            }
        }
    }
}
