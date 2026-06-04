import CoreGraphics
import AgentlySDK

struct HostedWorkspaceLayoutPlan: Equatable {
    let workspaceHeight: CGFloat
    let transcriptHeight: CGFloat
}

func resolveDefaultHostedWorkspaceDisplayMode(isRegularWidth: Bool) -> HostedWorkspaceDisplayMode {
    isRegularWidth ? .expanded : .standard
}

func resolveActiveHostedWorkspaceWindow(
    restoreState: HostedWorkspaceRestoreState?,
    conversationState: ConversationStateResponse?
) -> WorkspaceWindowSnapshot? {
    let restore = restoreState ?? conversationState.flatMap { deriveHostedWorkspaceRestoreState(from: $0) }
    let selectedID = restore?.selectedWindowId?.trimmingCharacters(in: .whitespacesAndNewlines)
    if let selectedID, !selectedID.isEmpty,
       let matched = restore?.windows.first(where: { $0.windowId == selectedID }) {
        return matched
    }
    return restore?.windows.last
}

func resolveTranscriptHeight(
    availableHeight: CGFloat,
    isRegularWidth: Bool,
    measuredHeight: CGFloat,
    prefersWorkspacePriority: Bool = false
) -> CGFloat {
    let fallback: CGFloat = if isRegularWidth {
        prefersWorkspacePriority ? 72 : 110
    } else {
        160
    }
    guard measuredHeight > 0 else {
        return fallback
    }
    let paddedMeasured = measuredHeight + 20
    let maxAllowed = if isRegularWidth {
        prefersWorkspacePriority ? max(availableHeight * 0.18, 72) : max(availableHeight * 0.28, 110)
    } else {
        max(availableHeight * 0.42, 160)
    }
    return min(max(paddedMeasured, fallback), maxAllowed)
}

func resolveHostedWorkspaceLayoutPlan(
    availableHeight: CGFloat,
    showsHostedWorkspace: Bool,
    displayMode: HostedWorkspaceDisplayMode,
    isRegularWidth: Bool,
    transcriptMeasuredHeight: CGFloat,
    hostedWorkspaceMeasuredHeight: CGFloat,
    hostedWindowContentMeasuredHeight: CGFloat,
    activeWindow: WorkspaceWindowSnapshot?
) -> HostedWorkspaceLayoutPlan {
    guard showsHostedWorkspace else {
        return HostedWorkspaceLayoutPlan(workspaceHeight: 0, transcriptHeight: availableHeight)
    }

    let transcriptHeight = resolveTranscriptHeight(
        availableHeight: availableHeight,
        isRegularWidth: isRegularWidth,
        measuredHeight: transcriptMeasuredHeight,
        prefersWorkspacePriority: isRegularWidth && displayMode == .expanded
    )

    let hintedShare = activeWindow?.workspaceSharePct.flatMap { pct -> CGFloat? in
        guard pct > 0 else { return nil }
        return min(max(CGFloat(pct) / 100, 0.2), 0.95)
    }
    let minimumWorkspaceHeight = max(CGFloat(activeWindow?.workspaceMinHeight ?? 0), 220)

    let workspaceShare: CGFloat = switch displayMode {
    case .expanded: max(hintedShare ?? 0.74, 0.84)
    case .minimized: 0.14
    default: hintedShare ?? 0.74
    }

    let workspaceHeight = resolveHostedWorkspaceHeight(
        availableHeight: availableHeight,
        displayMode: displayMode,
        share: workspaceShare,
        minimumTranscriptHeight: transcriptHeight,
        minimumWorkspaceHeight: minimumWorkspaceHeight,
        hostedWorkspaceMeasuredHeight: hostedWorkspaceMeasuredHeight,
        hostedWindowContentMeasuredHeight: hostedWindowContentMeasuredHeight
    )

    let finalTranscriptHeight = max(availableHeight - workspaceHeight - 16, transcriptHeight)
    return HostedWorkspaceLayoutPlan(
        workspaceHeight: workspaceHeight,
        transcriptHeight: finalTranscriptHeight
    )
}

func resolveHostedWorkspaceHeight(
    availableHeight: CGFloat,
    displayMode: HostedWorkspaceDisplayMode,
    share: CGFloat,
    minimumTranscriptHeight: CGFloat,
    minimumWorkspaceHeight: CGFloat,
    hostedWorkspaceMeasuredHeight: CGFloat,
    hostedWindowContentMeasuredHeight: CGFloat
) -> CGFloat {
    if displayMode == .minimized {
        return 88
    }
    let maxAllowedByTranscript = max(availableHeight - minimumTranscriptHeight - 16, minimumWorkspaceHeight)
    let maxAllowed = displayMode == .expanded
        ? maxAllowedByTranscript
        : min(maxAllowedByTranscript, max(availableHeight * share, minimumWorkspaceHeight))
    let effectiveMeasuredHeight = max(hostedWorkspaceMeasuredHeight, hostedWindowContentMeasuredHeight + 88)
    guard effectiveMeasuredHeight > 0 else {
        return displayMode == .expanded ? maxAllowed : min(maxAllowed, 420)
    }
    return min(max(effectiveMeasuredHeight, minimumWorkspaceHeight), maxAllowed)
}
