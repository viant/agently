import SwiftUI
import AgentlySDK
import ForgeIOSRuntime
import ForgeIOSUI

func toolFeedSymbol(_ presentation: FeedPresentation?) -> String {
    switch presentation?.icon?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() {
    case "list", "checklist": return "list.bullet.clipboard"
    case "terminal", "console": return "terminal"
    case "changes", "refresh": return "arrow.triangle.2.circlepath"
    case "chart", "report": return "chart.bar"
    case "database", "data": return "cylinder"
    case "document", "file": return "doc.text"
    case "folder", "explorer": return "folder"
    default: return "wrench.and.screwdriver"
    }
}

func toolFeedAccent(_ presentation: FeedPresentation?) -> Color {
    let raw = presentation?.accent?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let named: [String: Color] = [
        "blue": Color(red: 0.10, green: 0.45, blue: 0.94),
        "orange": Color(red: 0.88, green: 0.54, blue: 0.12),
        "purple": Color(red: 0.49, green: 0.32, blue: 0.85),
        "teal": Color(red: 0.04, green: 0.61, blue: 0.60),
        "pink": Color(red: 0.87, green: 0.36, blue: 0.47),
    ]
    if let color = named[raw.lowercased()] { return color }
    let hex = raw.hasPrefix("#") ? String(raw.dropFirst()) : raw
    if hex.count == 6, let value = UInt64(hex, radix: 16) {
        return Color(
            red: Double((value >> 16) & 0xff) / 255,
            green: Double((value >> 8) & 0xff) / 255,
            blue: Double(value & 0xff) / 255
        )
    }
    return Color(red: 0.35, green: 0.40, blue: 0.85)
}

func visibleToolFeeds(_ feeds: [ActiveFeedState]) -> [ActiveFeedState] {
    feeds
        .filter { $0.developerOnly != true && !($0.feedID ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
        .sorted {
            ($0.title ?? $0.feedID ?? "").localizedCaseInsensitiveCompare($1.title ?? $1.feedID ?? "") == .orderedAscending
        }
}

func mergedToolFeeds(live: [ActiveFeedState], persisted: [ActiveFeedState]) -> [ActiveFeedState] {
    var rows: [String: ActiveFeedState] = [:]
    for feed in persisted + live {
        let id = (feed.feedID ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        if !id.isEmpty { rows[id] = feed }
    }
    return Array(rows.values)
}

func toolFeedSummaryLines(_ value: AgentlySDK.JSONValue?, limit: Int = 8) -> [String] {
    guard let value, limit > 0 else { return [] }
    var lines: [String] = []
    func append(_ text: String) {
        let normalized = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !normalized.isEmpty, lines.count < limit else { return }
        let preview = normalized.count > 480 ? String(normalized.prefix(480)) + "…" : normalized
        if !lines.contains(preview) { lines.append(preview) }
    }
    func walk(_ node: AgentlySDK.JSONValue, depth: Int = 0) {
        guard lines.count < limit, depth < 6 else { return }
        switch node {
        case .string(let text): append(text)
        case .number(let number): append(number.formatted())
        case .bool(let flag): append(flag ? "Yes" : "No")
        case .null: break
        case .array(let values): values.forEach { walk($0, depth: depth + 1) }
        case .object(let object):
            let preferred = ["explanation", "step", "title", "name", "command", "message", "path", "status", "input", "output"]
            for key in preferred {
                guard let child = object[key] else { continue }
                if case .string(let text) = child {
                    if key == "status" {
                        append(text.replacingOccurrences(of: "_", with: " ").capitalized)
                    } else if key == "input" {
                        append("$ " + text)
                    } else {
                        append(text)
                    }
                } else if case .object = child {
                    walk(child, depth: depth + 1)
                } else if case .array = child {
                    walk(child, depth: depth + 1)
                }
            }
            for key in object.keys.sorted() where !preferred.contains(key) {
                guard let child = object[key] else { continue }
                if case .object = child { walk(child, depth: depth + 1) }
                if case .array = child { walk(child, depth: depth + 1) }
            }
        }
    }
    walk(value)
    return lines
}

func decodedToolFeedContainer(_ value: AgentlySDK.JSONValue?) -> ContainerDef? {
    guard let value,
          let data = try? JSONEncoder().encode(value) else { return nil }
    return try? JSONDecoder().decode(ContainerDef.self, from: data)
}

struct ToolFeedFilePreview {
    let container: ContainerDef
    let browser: FileBrowserDef
}

struct ToolFeedAction: Identifiable {
    let id: String
    let item: ToolbarItemDef
    let execution: ExecutionDef
}

func toolFeedActions(in container: ContainerDef?) -> [ToolFeedAction] {
    guard let container else { return [] }
    var result: [ToolFeedAction] = []
    for item in container.toolbar?.items ?? [] {
        let baseID = item.id ?? item.label ?? "action"
        for execution in item.on where execution.event == "onClick" && execution.action == "tool.execute" {
            result.append(ToolFeedAction(id: baseID + "-tool.execute", item: item, execution: execution))
        }
    }
    for child in container.containers { result.append(contentsOf: toolFeedActions(in: child)) }
    return result
}

func toolFeedFilePreview(in container: ContainerDef?) -> ToolFeedFilePreview? {
    guard let container else { return nil }
    if let browser = container.fileBrowser, browser.preview != nil {
        return ToolFeedFilePreview(container: container, browser: browser)
    }
    for child in container.containers {
        if let match = toolFeedFilePreview(in: child) { return match }
    }
    return nil
}

func toolFeedRows(
    data: AgentlySDK.JSONValue?,
    dataSources: AgentlySDK.JSONValue?,
    dataSourceRef: String?
) -> [[String: ForgeIOSRuntime.JSONValue]] {
    guard let data else { return [] }
    let ref = dataSourceRef?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    func select(_ path: String, from root: AgentlySDK.JSONValue) -> AgentlySDK.JSONValue? {
        var selected = root
        for segment in path.split(separator: ".").map(String.init) {
            guard case .object(let object) = selected, let next = object[segment] else { return nil }
            selected = next
        }
        return selected
    }
    var visiting = Set<String>()
    func resolve(_ name: String, sources: [String: AgentlySDK.JSONValue]) -> AgentlySDK.JSONValue? {
        guard !visiting.contains(name), case .object(let definition) = sources[name] else { return nil }
        visiting.insert(name)
        defer { visiting.remove(name) }
        if case .string(let sourcePath) = definition["source"] { return select(sourcePath, from: data) }
        guard case .string(let parentName) = definition["dataSourceRef"],
              let parent = resolve(parentName, sources: sources) else { return nil }
        let parentRoot: AgentlySDK.JSONValue
        if case .array(let values) = parent, values.count == 1 { parentRoot = values[0] }
        else { parentRoot = parent }
        let selector: String
        if case .object(let selectors) = definition["selectors"], case .string(let value) = selectors["data"] { selector = value }
        else { selector = "output" }
        return select(selector, from: parentRoot)
    }
    var selected = data
    if !ref.isEmpty, let dataSources, case .object(let sources) = dataSources {
        selected = resolve(ref, sources: sources) ?? data
    }
    guard let encoded = try? JSONEncoder().encode(selected),
          let bridged = try? JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: encoded) else { return [] }
    switch bridged {
    case .array(let values): return values.compactMap(\.objectValue)
    case .object(let row): return [row]
    default: return []
    }
}

struct ToolFeedsSection: View {
    let feeds: [ActiveFeedState]
    let conversationID: String?
    let client: AgentlyClient

    @State private var selectedFeedID = ""
    @State private var payload: FeedDataResponse?
    @State private var isLoading = false
    @State private var errorMessage: String?
    @State private var isFeedSheetPresented = false

    private var visibleFeeds: [ActiveFeedState] { visibleToolFeeds(feeds) }
    private var selectedFeed: ActiveFeedState? { visibleFeeds.first { $0.feedID == selectedFeedID } ?? visibleFeeds.first }
    private var effectiveConversationID: String {
        conversationID?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? visibleFeeds.first?.conversationID?.trimmingCharacters(in: .whitespacesAndNewlines)
            ?? ""
    }

    var body: some View {
        if !visibleFeeds.isEmpty, !effectiveConversationID.isEmpty {
            VStack(alignment: .leading, spacing: 10) {
                HStack {
                    Text("Tool feeds").font(.headline)
                    Spacer()
                    Button("Open") { isFeedSheetPresented = true }
                        .font(.caption.weight(.semibold))
                }
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 8) {
                        ForEach(visibleFeeds) { feed in
                            Button {
                                selectedFeedID = feed.feedID ?? ""
                                isFeedSheetPresented = true
                            } label: {
                                HStack(spacing: 5) {
                                    Image(systemName: toolFeedSymbol(feed.presentation))
                                    Text(feed.title ?? feed.feedID ?? "Feed")
                                    if (feed.itemCount ?? 0) > 0 {
                                        Text("\(feed.itemCount ?? 0)").font(.caption2.weight(.bold))
                                    }
                                }
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(toolFeedAccent(feed.presentation))
                                .padding(.horizontal, 10)
                                .padding(.vertical, 6)
                                .background(toolFeedAccent(feed.presentation).opacity(selectedFeedID == feed.feedID ? 0.16 : 0.08), in: Capsule())
                                .overlay(Capsule().stroke(toolFeedAccent(feed.presentation).opacity(0.22), lineWidth: 1))
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
            .padding(12)
            .background(Color.secondary.opacity(0.045), in: RoundedRectangle(cornerRadius: 16))
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(Color.secondary.opacity(0.12), lineWidth: 1))
            .padding(.horizontal)
            .task(id: "\(effectiveConversationID)#\(selectedFeedID)#\(visibleFeeds.map { $0.updatedAt ?? 0 })#\(isFeedSheetPresented)") {
                if selectedFeedID.isEmpty { selectedFeedID = visibleFeeds.first?.feedID ?? "" }
                guard isFeedSheetPresented, !selectedFeedID.isEmpty else { return }
                isLoading = true
                errorMessage = nil
                do {
                    payload = try await client.getFeedData(feedID: selectedFeedID, conversationID: effectiveConversationID)
                } catch {
                    payload = nil
                    errorMessage = "Unable to load this feed."
                }
                isLoading = false
            }
            .onChange(of: visibleFeeds.map { $0.feedID ?? "" }) { _, ids in
                if !ids.contains(selectedFeedID) { selectedFeedID = ids.first ?? "" }
            }
            .sheet(isPresented: $isFeedSheetPresented) {
                NavigationStack {
                    ScrollView {
                        VStack(alignment: .leading, spacing: 14) {
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 8) {
                                    ForEach(visibleFeeds) { feed in
                                        Button(feed.title ?? feed.feedID ?? "Feed") {
                                            selectedFeedID = feed.feedID ?? ""
                                        }
                                        .buttonStyle(.borderedProminent)
                                        .tint(toolFeedAccent(feed.presentation).opacity(selectedFeedID == feed.feedID ? 1 : 0.55))
                                    }
                                }
                            }
                            feedDetailContent
                        }
                        .padding()
                    }
                    .navigationTitle("Tool feeds")
                    .toolbar {
                        ToolbarItem(placement: .confirmationAction) {
                            Button("Done") { isFeedSheetPresented = false }
                        }
                    }
                }
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
            }
        }
    }

    @ViewBuilder
    private var feedDetailContent: some View {
        if isLoading {
            ProgressView().controlSize(.small)
        } else if let resolved = toolFeedFilePreview(in: decodedToolFeedContainer(payload?.ui)) {
            VStack(alignment: .leading, spacing: 12) {
                FilePreviewBrowser(
                    rows: toolFeedRows(
                        data: payload?.data ?? selectedFeed?.data,
                        dataSources: payload?.dataSources,
                        dataSourceRef: resolved.browser.dataSourceRef ?? resolved.container.dataSourceRef
                    ),
                    config: resolved.browser,
                    loadText: { uri in try await client.downloadWorkspaceFile(uri: uri) },
                    loadPreview: { uri in
                        guard let tool = resolved.browser.preview?.tool, !tool.isEmpty else {
                            throw NSError(domain: "ToolFeedPreview", code: 1)
                        }
                        let raw = try await client.executeTool(
                            name: tool,
                            args: ["url": .string(uri)],
                            conversationID: effectiveConversationID
                        )
                        guard let data = raw.data(using: .utf8),
                              let value = try? JSONDecoder().decode(AgentlySDK.JSONValue.self, from: data),
                              case .object(let object) = value else {
                            return FilePreviewContent()
                        }
                        func text(_ key: String) -> String {
                            if case .string(let value) = object[key] { return value }
                            return ""
                        }
                        return FilePreviewContent(
                            current: text("current"), previous: text("previous"), diff: text("diff")
                        )
                    }
                )
                ToolFeedActionBar(
                    actions: toolFeedActions(in: decodedToolFeedContainer(payload?.ui)),
                    client: client,
                    conversationID: effectiveConversationID
                )
            }
        } else if let container = decodedToolFeedContainer(payload?.ui),
                  let terminal = container.terminal {
            TerminalRenderer(
                container: container,
                terminal: terminal,
                rows: toolFeedRows(
                    data: payload?.data ?? selectedFeed?.data,
                    dataSources: payload?.dataSources,
                    dataSourceRef: terminal.dataSourceRef ?? container.dataSourceRef
                )
            )
        } else {
            let lines = toolFeedSummaryLines(payload?.data ?? selectedFeed?.data)
            if lines.isEmpty {
                if let errorMessage {
                    Text(errorMessage).font(.footnote).foregroundStyle(Color.red)
                } else {
                    Text("No feed details are available yet.").font(.footnote).foregroundStyle(.secondary)
                }
            } else {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                        HStack(alignment: .firstTextBaseline, spacing: 8) {
                            Image(systemName: "checkmark")
                                .font(.caption2.weight(.bold))
                                .foregroundStyle(Color.green)
                            Text(line).font(.body)
                        }
                    }
                    if errorMessage != nil {
                        Text("Showing the latest saved feed; live refresh is temporarily unavailable.")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
    }

}

private struct ToolFeedActionBar: View {
    let actions: [ToolFeedAction]
    let client: AgentlyClient
    let conversationID: String
    @State private var busyID: String?
    @State private var errorMessage: String?

    var body: some View {
        if !actions.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 8) {
                    Spacer()
                    ForEach(actions) { action in
                        actionButton(action)
                    }
                }
                if let errorMessage { Text(errorMessage).font(.caption).foregroundStyle(Color.red) }
            }
        }
    }

    @ViewBuilder private func actionButton(_ action: ToolFeedAction) -> some View {
        let button = Button {
            Task { await execute(action) }
        } label: {
            HStack(spacing: 6) {
                if busyID == action.id { ProgressView().controlSize(.small) }
                Text(action.item.label ?? action.item.id ?? "Action")
            }
        }
        .tint(action.item.intent == "primary" ? Color.blue : Color.secondary)
        .disabled(busyID != nil)
        if action.item.appearance == "minimal" { button.buttonStyle(.borderless) }
        else { button.buttonStyle(.borderedProminent) }
    }

    private func execute(_ action: ToolFeedAction) async {
        guard case .object(let target) = action.execution.target,
              case .string(let name) = target["name"], !name.isEmpty else { return }
        busyID = action.id
        errorMessage = nil
        do {
            let args: [String: AgentlySDK.JSONValue]
            if let value = target["arguments"],
               let data = try? JSONEncoder().encode(value),
               let decoded = try? JSONDecoder().decode([String: AgentlySDK.JSONValue].self, from: data) { args = decoded }
            else { args = [:] }
            _ = try await client.executeTool(name: name, args: args, conversationID: conversationID)
        } catch { errorMessage = error.localizedDescription }
        busyID = nil
    }
}
