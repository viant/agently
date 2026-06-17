import SwiftUI
import ForgeIOSUI
#if canImport(UIKit)
import UIKit
#elseif canImport(AppKit)
import AppKit
#endif

public struct TranscriptScreen: View {
    let items: [ChatTranscriptEntry]
    let onReusePrompt: ((String) -> Void)?
    let onReuseAndSendPrompt: ((String) -> Void)?

    public init(
        items: [ChatTranscriptEntry],
        onReusePrompt: ((String) -> Void)? = nil,
        onReuseAndSendPrompt: ((String) -> Void)? = nil
    ) {
        self.items = items
        self.onReusePrompt = onReusePrompt
        self.onReuseAndSendPrompt = onReuseAndSendPrompt
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
        }
    }

    private var transcriptStack: some View {
        LazyVStack(alignment: .leading, spacing: 12) {
            ForEach(items) { item in
                TranscriptBubble(
                    item: item,
                    onReusePrompt: onReusePrompt,
                    onReuseAndSendPrompt: onReuseAndSendPrompt
                )
                .id(item.id)
            }
        }
    }
}

public struct TranscriptContentHeightPreferenceKey: PreferenceKey {
    public static var defaultValue: CGFloat = 0

    public static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

private struct TranscriptBubble: View {
    let item: ChatTranscriptEntry
    let onReusePrompt: ((String) -> Void)?
    let onReuseAndSendPrompt: ((String) -> Void)?
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
            TranscriptMessageContent(markdown: item.markdown)
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
