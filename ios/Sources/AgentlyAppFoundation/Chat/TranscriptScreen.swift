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
                .padding()
            }
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
}

private struct TranscriptBubble: View {
    let item: ChatTranscriptEntry
    let onReusePrompt: ((String) -> Void)?
    let onReuseAndSendPrompt: ((String) -> Void)?

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
            Text(item.markdown.isEmpty ? "(empty response)" : item.markdown)
                .frame(maxWidth: .infinity, alignment: .leading)
                .textSelection(.enabled)
        } else {
            MarkdownRenderer(markdown: item.markdown)
                .textSelection(.enabled)
        }
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
