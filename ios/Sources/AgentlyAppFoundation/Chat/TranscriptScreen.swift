import SwiftUI
import ForgeIOSUI
import AgentlySDK
#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

struct AssistantDestinationLink: View {
    let title: String
    let supportingText: String
    let systemImage: String
    let accessibilityIdentifier: String
    let onOpen: () -> Void

    var body: some View {
        Button(action: onOpen) {
            HStack(spacing: 10) {
                Image(systemName: systemImage)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(Color.accentColor)
                    .frame(width: 34, height: 34)
                    .background(Color.accentColor.opacity(0.11), in: RoundedRectangle(cornerRadius: 10))
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.primary)
                    Text(supportingText)
                        .font(.caption2)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer(minLength: 4)
                Image(systemName: "arrow.right")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(Color.accentColor)
            }
            .padding(10)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(Color.accentColor.opacity(0.055), in: RoundedRectangle(cornerRadius: 13))
            .overlay(RoundedRectangle(cornerRadius: 13).stroke(Color.accentColor.opacity(0.16), lineWidth: 1))
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Open \(title)")
        .accessibilityIdentifier(accessibilityIdentifier)
    }
}

func assistantDestinationSystemImage(_ value: String?) -> String {
    switch value?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "chart": return "chart.xyaxis.line"
    case "document": return "doc.text"
    case "application": return "app"
    default: return "rectangle.topthird.inset.filled"
    }
}

public struct TranscriptScreen: View {
    private static let initialRenderCount = 40
    private static let renderBatchSize = 40
    let items: [ChatTranscriptEntry]
	let client: AgentlyClient?
    let conversationID: String?
    let onReusePrompt: ((String) -> Void)?
    let onReuseAndSendPrompt: ((String) -> Void)?
    let workspaceAttachment: HostedWorkspaceAttachment?
    let onOpenWorkspace: (() -> Void)?
    @State private var visibleItemCount: Int

    public init(
        items: [ChatTranscriptEntry],
		client: AgentlyClient? = nil,
        conversationID: String? = nil,
        onReusePrompt: ((String) -> Void)? = nil,
        onReuseAndSendPrompt: ((String) -> Void)? = nil,
        workspaceAttachment: HostedWorkspaceAttachment? = nil,
        onOpenWorkspace: (() -> Void)? = nil
    ) {
        self.items = items
        self.client = client
        self.conversationID = conversationID
        self.onReusePrompt = onReusePrompt
        self.onReuseAndSendPrompt = onReuseAndSendPrompt
        self.workspaceAttachment = workspaceAttachment
        self.onOpenWorkspace = onOpenWorkspace
        _visibleItemCount = State(initialValue: min(items.count, Self.initialRenderCount))
    }

    public var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                transcriptStack
                .padding(.horizontal, 10)
                .padding(.vertical, 12)
                .background(
                    GeometryReader { proxy in
                        Color.clear
                            .preference(key: TranscriptContentHeightPreferenceKey.self, value: proxy.size.height)
                    }
                )
            }
            .agentlyScrollDismissesKeyboard()
            .contentShape(Rectangle())
            .simultaneousGesture(TapGesture().onEnded {
                requestAgentlyPlatformKeyboardDismissal()
            })
            .simultaneousGesture(DragGesture(minimumDistance: 3).onChanged { _ in
                requestAgentlyPlatformKeyboardDismissal()
            })
            .onChange(of: items.last?.id) { _, newValue in
                guard let newValue else { return }
                withAnimation(.easeOut(duration: 0.2)) {
                    proxy.scrollTo(newValue, anchor: .bottom)
                }
            }
            .onAppear {
                guard let lastID = items.last?.id else { return }
                proxy.scrollTo(lastID, anchor: .bottom)
            }
            .onChange(of: items.first?.id) { _, _ in
                visibleItemCount = min(items.count, Self.initialRenderCount)
            }
        }
    }

    private var transcriptStack: some View {
        let start = transcriptWindowStart(
            totalItemCount: items.count,
            visibleItemCount: visibleItemCount
        )
        let visibleItems = items.dropFirst(start)
        let attachmentItemID = workspaceAttachment.flatMap { attachment in
            items.last(where: { $0.role != "user" && $0.turnID == attachment.turnID })?.id
        }
        return LazyVStack(alignment: .leading, spacing: 12) {
            if start > 0 {
                Button {
                    visibleItemCount = min(items.count, visibleItemCount + Self.renderBatchSize)
                } label: {
                    Label(
                        "Show \(min(Self.renderBatchSize, start)) earlier messages",
                        systemImage: "clock.arrow.circlepath"
                    )
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .accessibilityIdentifier("transcript-show-earlier")
            }
            ForEach(visibleItems) { item in
                TranscriptBubble(
                    item: item,
					client: client,
                    conversationID: conversationID,
                    onReusePrompt: onReusePrompt,
                    onReuseAndSendPrompt: onReuseAndSendPrompt,
                    workspaceAttachment: item.id == attachmentItemID ? workspaceAttachment : nil,
                    onOpenWorkspace: onOpenWorkspace
                )
                .id(item.id)
            }
        }
    }
}

internal func transcriptWindowStart(totalItemCount: Int, visibleItemCount: Int) -> Int {
    max(0, totalItemCount - max(0, visibleItemCount))
}

public struct TranscriptContentHeightPreferenceKey: PreferenceKey {
    public static var defaultValue: CGFloat = 0

    public static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

private struct TranscriptBubble: View {
    let item: ChatTranscriptEntry
	let client: AgentlyClient?
    let conversationID: String?
    let onReusePrompt: ((String) -> Void)?
    let onReuseAndSendPrompt: ((String) -> Void)?
    let workspaceAttachment: HostedWorkspaceAttachment?
    let onOpenWorkspace: (() -> Void)?
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass
    @State private var isExpanded = false

    var body: some View {
        VStack(alignment: item.role == "user" ? .trailing : .leading, spacing: 6) {
            HStack(spacing: 8) {
                if item.role != "user" {
                    roleLabel
                }
                if let statusLabel = item.statusLabel, !statusLabel.isEmpty {
                    Text(statusLabel)
                        .font(.caption2.weight(.semibold))
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .background(statusTint.opacity(0.14), in: Capsule())
                        .foregroundStyle(statusTint)
                }
                if item.role == "user" {
                    roleLabel
                }
            }
            .frame(maxWidth: .infinity, alignment: item.role == "user" ? .trailing : .leading)

            transcriptContent

            if let workspaceAttachment, item.role != "user", let onOpenWorkspace {
                transcriptWorkspaceAttachment(workspaceAttachment, onOpen: onOpenWorkspace)
            }

            if let timestampLabel = item.timestampLabel {
                Text(timestampLabel)
                    .font(.caption2)
                    .foregroundStyle(.tertiary)
                    .frame(maxWidth: .infinity, alignment: item.role == "user" ? .trailing : .leading)
            }
        }
        .padding()
        .frame(maxWidth: .infinity, alignment: item.role == "user" ? .trailing : .leading)
        .background(bubbleTint.opacity(item.role == "user" ? 0.18 : 0.08), in: RoundedRectangle(cornerRadius: 14))
        .contentShape(RoundedRectangle(cornerRadius: 14))
        .simultaneousGesture(TapGesture().onEnded {
            requestAgentlyPlatformKeyboardDismissal()
        })
        .onTapGesture {
            guard shouldOfferExpansion else { return }
            withAnimation(.easeInOut(duration: 0.18)) {
                isExpanded.toggle()
            }
        }
        .accessibilityHint(shouldOfferExpansion ? "Double tap to \(isExpanded ? "collapse" : "show full") message text." : "")
        .contextMenu {
            Button {
                copyMessageToPasteboard()
            } label: {
                Label("Copy Message", systemImage: "doc.on.doc")
            }
            if item.role == "user" {
                Button {
                    onReusePrompt?(item.markdown)
                } label: {
                    Label("Reuse Prompt", systemImage: "arrow.uturn.backward")
                }
                if onReuseAndSendPrompt != nil {
                    Button {
                        onReuseAndSendPrompt?(item.markdown)
                    } label: {
                        Label("Reuse And Send", systemImage: "paperplane")
                    }
                }
            }
        }
    }

    private var roleLabel: some View {
        Text(item.role == "user" ? "You" : "Assistant")
            .font(.caption.weight(.semibold))
            .foregroundStyle(.secondary)
    }

    private func transcriptWorkspaceAttachment(
        _ attachment: HostedWorkspaceAttachment,
        onOpen: @escaping () -> Void
    ) -> some View {
        AssistantDestinationLink(
            title: attachment.presentation.badgeLabel,
            supportingText: attachment.presentation.supportingText,
            systemImage: attachment.presentation.badgeSymbolName,
            accessibilityIdentifier: "transcript-workspace-attachment",
            onOpen: onOpen
        )
    }

    @ViewBuilder
    private var transcriptContent: some View {
        if item.role == "user" {
            VStack(alignment: .leading, spacing: 6) {
                Text(item.markdown.isEmpty ? "(empty response)" : item.markdown)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .lineLimit(isExpanded ? nil : 4)
                    .truncationMode(.tail)
                    .transcriptTextSelection(allowsInlineTextSelection)
                if shouldOfferExpansion {
                    Button {
                        withAnimation(.easeInOut(duration: 0.18)) {
                            isExpanded.toggle()
                        }
                    } label: {
                        Label(
                            isExpanded ? "Show less" : "Show full text",
                            systemImage: isExpanded ? "chevron.up" : "text.justify.left"
                        )
                        .font(.caption.weight(.semibold))
                    }
                    .buttonStyle(.plain)
                    .foregroundStyle(Color.accentColor)
                    .accessibilityIdentifier(isExpanded ? "transcript-collapse-message" : "transcript-expand-message")
                }
            }
        } else {
            TranscriptMessageContent(
                markdown: item.markdown,
                renderedParts: item.renderedParts,
                renderedReports: item.renderedReports,
				client: client,
                conversationID: conversationID
            )
                .transcriptTextSelection(allowsInlineTextSelection)
        }
    }

    private var allowsInlineTextSelection: Bool {
        horizontalSizeClass != .compact
    }

    private var shouldOfferExpansion: Bool {
        guard item.role == "user" else { return false }
        let text = item.markdown.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return false }
        let explicitLineCount = text.components(separatedBy: .newlines).count
        let estimatedWrappedLineCount = text
            .components(separatedBy: .newlines)
            .map { max(1, Int(ceil(Double($0.count) / 38.0))) }
            .reduce(0, +)
        return explicitLineCount > 4 || estimatedWrappedLineCount > 4
    }

    private var bubbleTint: Color {
        item.role == "user" ? .blue : .secondary
    }

    private var statusTint: Color {
        switch item.statusLabel {
        case "Failed":
            return .red
        case "Canceled":
            return .orange
        default:
            return .blue
        }
    }

    private func copyMessageToPasteboard() {
        #if canImport(UIKit)
        UIPasteboard.general.string = item.markdown
        #elseif canImport(AppKit)
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(item.markdown, forType: .string)
        #endif
    }
}

private struct TranscriptTextSelectionModifier: ViewModifier {
    let isEnabled: Bool

    @ViewBuilder
    func body(content: Content) -> some View {
        if isEnabled {
            content.textSelection(.enabled)
        } else {
            content
        }
    }
}

private extension View {
    func transcriptTextSelection(_ isEnabled: Bool) -> some View {
        modifier(TranscriptTextSelectionModifier(isEnabled: isEnabled))
    }
}
