import Foundation
import AgentlySDK
import ForgeIOSRuntime

private let uiBridgeClientIDDefaultsKey = "agently.ios.uiBridge.clientID"
typealias BridgeJSONValue = AgentlySDK.JSONValue

struct AppleUIBridgeWindow: Codable, Sendable {
    let windowID: String
    let windowKey: String
    let windowTitle: String
    let conversationID: String?
    let presentation: String?
    let region: String?
    let parentKey: String?
    let workspaceSharePct: Int?
    let workspaceMinHeight: Int?
    let parameters: [String: BridgeJSONValue]
    let windowForm: [String: BridgeJSONValue]
    let inTab: Bool
    let isModal: Bool

    enum CodingKeys: String, CodingKey {
        case windowID = "windowId"
        case windowKey
        case windowTitle
        case conversationID = "conversationId"
        case presentation
        case region
        case parentKey
        case workspaceSharePct
        case workspaceMinHeight
        case parameters
        case windowForm
        case inTab
        case isModal
    }
}

struct AppleUIBridgeSnapshot: Codable, Sendable {
    let conversationID: String?
    let windows: [AppleUIBridgeWindow]

    enum CodingKeys: String, CodingKey {
        case conversationID = "conversationId"
        case windows
    }
}

@MainActor
final class AppleUIBridgeController {
    private let encoder: JSONEncoder
    private let decoder: JSONDecoder
    private let snapshotProvider: @Sendable (String?) async -> AppleUIBridgeSnapshot
    private let commandHandler: @Sendable (String, [String: BridgeJSONValue]) async throws -> [String: BridgeJSONValue]
    private let clientIDValue: String

    private var rpcClient: UIBridgeRPCClient
    private var selectedWindowID: String?
    private var lastSnapshotFingerprint = ""
    private var isStarted = false
    private var pollTask: Task<Void, Never>?
    private var snapshotTask: Task<Void, Never>?

    init(
        client: AgentlyClient,
        encoder: JSONEncoder = .agently(),
        decoder: JSONDecoder = .agently(),
        snapshotProvider: @escaping @Sendable (String?) async -> AppleUIBridgeSnapshot,
        commandHandler: @escaping @Sendable (String, [String: BridgeJSONValue]) async throws -> [String: BridgeJSONValue]
    ) {
        self.rpcClient = UIBridgeRPCClient(client: client)
        self.encoder = encoder
        self.decoder = decoder
        self.snapshotProvider = snapshotProvider
        self.commandHandler = commandHandler
        self.clientIDValue = Self.loadOrCreateClientID()
    }

    var clientID: String {
        clientIDValue
    }

    func updateClient(_ client: AgentlyClient) {
        self.rpcClient = UIBridgeRPCClient(client: client)
        self.lastSnapshotFingerprint = ""
    }

    func start() {
        guard !isStarted else { return }
        isStarted = true
        let controller = self
        pollTask = Task {
            await controller.runPollLoop()
        }
        snapshotTask = Task {
            await controller.runSnapshotLoop()
        }
    }

    func stop() {
        isStarted = false
        pollTask?.cancel()
        snapshotTask?.cancel()
        pollTask = nil
        snapshotTask = nil
        lastSnapshotFingerprint = ""
    }

    func ensureConnected() async -> String {
        await helloIfNeeded()
        return clientIDValue
    }

    private func runPollLoop() async {
        while isStarted && !Task.isCancelled {
            do {
                _ = try await ensureConnectedRPC()
                try await pollOnce()
            } catch {
                await rpcClient.resetSession()
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        }
    }

    private func runSnapshotLoop() async {
        while isStarted && !Task.isCancelled {
            do {
                try await publishSnapshot(force: false)
            } catch {
                await rpcClient.resetSession()
            }
            try? await Task.sleep(nanoseconds: 1_000_000_000)
        }
    }

    private func helloIfNeeded() async {
        _ = try? await ensureConnectedRPC()
    }

    private func ensureConnectedRPC() async throws -> [String: BridgeJSONValue]? {
        try await rpcClient.hello(clientID: clientIDValue)
    }

    private func pollOnce() async throws {
        guard let result = try await rpcClient.poll(clientID: clientIDValue, timeoutMs: 20_000) else {
            return
        }
        guard case .object(let params) = result["params"] ?? .null else {
            return
        }
        let commandID = params["id"]?.stringValue?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines) ?? ""
        let method = params["method"]?.stringValue?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines) ?? ""
        guard !commandID.isEmpty, !method.isEmpty else {
            return
        }
        let commandParams = params["params"]?.objectValue ?? [:]
        do {
            let result = try await commandHandler(method, commandParams)
            updateSelectedWindowID(method: method, params: commandParams, result: result)
            _ = try await rpcClient.respond(
                commandID: commandID,
                ok: true,
                result: BridgeJSONValue.object(result)
            )
            try await publishSnapshot(force: true)
        } catch {
            _ = try? await rpcClient.respond(commandID: commandID, ok: false, error: error.localizedDescription)
        }
    }

    private func updateSelectedWindowID(method: String, params: [String: BridgeJSONValue], result: [String: BridgeJSONValue]) {
        switch method {
        case "ui.window.open":
            let next = result["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !next.isEmpty {
                selectedWindowID = next
            }
        case "ui.window.activate", "ui.window.selectTab":
            let next = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !next.isEmpty {
                selectedWindowID = next
            }
        case "ui.window.close":
            let closing = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !closing.isEmpty, selectedWindowID == closing {
                selectedWindowID = nil
            }
        default:
            break
        }
    }

    private func publishSnapshot(force: Bool) async throws {
        let snapshot = await snapshotProvider(selectedWindowID)
        let payload = SnapshotPayload(
            clientID: clientIDValue,
            selected: SnapshotSelection(
                windowID: selectedWindowID ?? "chat/new",
                tabID: selectedWindowID ?? "chat/new"
            ),
            conversationID: snapshot.conversationID,
            windows: snapshot.windows
        )
        let fingerprintData = try encoder.encode(payload)
        let fingerprint = String(data: fingerprintData, encoding: .utf8) ?? ""
        if !force, fingerprint == lastSnapshotFingerprint {
            return
        }
        lastSnapshotFingerprint = fingerprint
        guard let snapshotValue = try decoder.decode(BridgeJSONValue.self, from: fingerprintData).objectValue else {
            return
        }
        _ = try await rpcClient.snapshot(clientID: clientIDValue, data: BridgeJSONValue.object(snapshotValue))
    }

    private static func loadOrCreateClientID(defaults: UserDefaults = .standard) -> String {
        let existing = defaults.string(forKey: uiBridgeClientIDDefaultsKey)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !existing.isEmpty {
            return existing
        }
        let generated = "ios-ui-\(UUID().uuidString)"
        defaults.set(generated, forKey: uiBridgeClientIDDefaultsKey)
        return generated
    }
}

private struct SnapshotPayload: Codable {
    let clientID: String
    let selected: SnapshotSelection
    let conversationID: String?
    let windows: [AppleUIBridgeWindow]

    enum CodingKeys: String, CodingKey {
        case clientID = "clientId"
        case selected
        case conversationID = "conversationId"
        case windows
    }
}

private struct SnapshotSelection: Codable {
    let windowID: String
    let tabID: String

    enum CodingKeys: String, CodingKey {
        case windowID = "windowId"
        case tabID = "tabId"
    }
}

func bridgeHostedWorkspaceRestoreState(from payload: [String: BridgeJSONValue]) -> HostedWorkspaceRestoreState? {
    if let items = payload["items"]?.arrayValue, !items.isEmpty {
        let windows = items.compactMap { normalizeBridgeHostedWorkspaceWindow($0.objectValue) }
        if !windows.isEmpty {
            let selectedWindowID = payload["selectedWindowId"]?.stringValue
                ?? windows.last?.windowId
            return HostedWorkspaceRestoreState(
                windows: windows,
                selectedWindowId: selectedWindowID?.isEmpty == false ? selectedWindowID : nil
            )
        }
    }
    if let single = normalizeBridgeHostedWorkspaceWindow(payload) {
        let selectedWindowID = payload["selectedWindowId"]?.stringValue ?? single.windowId
        return HostedWorkspaceRestoreState(
            windows: [single],
            selectedWindowId: selectedWindowID.isEmpty ? nil : selectedWindowID
        )
    }
    return nil
}

func buildAppleUIBridgeSnapshot(
    activeConversationID: String?,
    selectedWindowID: String?,
    forgeRuntime: ForgeRuntime
) async -> AppleUIBridgeSnapshot {
    let conversationID = activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    var windows: [AppleUIBridgeWindow] = []
    if !conversationID.isEmpty {
        windows.append(
            AppleUIBridgeWindow(
                windowID: "chat/new",
                windowKey: "chat/new",
                windowTitle: "Chat",
                conversationID: conversationID,
                presentation: nil,
                region: nil,
                parentKey: nil,
                workspaceSharePct: nil,
                workspaceMinHeight: nil,
                parameters: [:],
                windowForm: [:],
                inTab: true,
                isModal: false
            )
        )
    }
    let runtimeWindows = await forgeRuntime.windows
    for window in runtimeWindows {
        let windowConversationID = window.conversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !conversationID.isEmpty, !windowConversationID.isEmpty, windowConversationID != conversationID {
            continue
        }
        windows.append(
            AppleUIBridgeWindow(
                windowID: window.id,
                windowKey: window.key,
                windowTitle: window.title,
                conversationID: conversationID.isEmpty ? window.conversationID : conversationID,
                presentation: window.presentation,
                region: window.region,
                parentKey: window.parentKey,
                workspaceSharePct: window.workspaceSharePct,
                workspaceMinHeight: window.workspaceMinHeight,
                parameters: window.parameters.mapValues(\.appValue),
                windowForm: await forgeRuntime.windowFormJSONValue(windowID: window.id).mapValues(\.appValue),
                inTab: window.inTab,
                isModal: window.isModal
            )
        )
    }
    return AppleUIBridgeSnapshot(
        conversationID: conversationID.isEmpty ? nil : conversationID,
        windows: windows
    )
}

func hostedWorkspaceRestoreState(
    from snapshot: AppleUIBridgeSnapshot,
    selectedWindowID: String?
) -> HostedWorkspaceRestoreState? {
    let windows = snapshot.windows.compactMap { window -> WorkspaceWindowSnapshot? in
        let presentation = window.presentation?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        let region = window.region?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? ""
        let parentKey = window.parentKey?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard presentation == "hosted", region == "chat.top", parentKey == "chat/new" else {
            return nil
        }
        return WorkspaceWindowSnapshot(
            windowId: window.windowID,
            conversationId: window.conversationID,
            windowKey: window.windowKey,
            windowTitle: window.windowTitle,
            presentation: window.presentation,
            region: window.region,
            parentKey: window.parentKey,
            inTab: window.inTab,
            parameters: window.parameters,
            windowForm: window.windowForm.isEmpty ? nil : window.windowForm
        )
    }
    guard !windows.isEmpty else {
        return nil
    }
    let selected = selectedWindowID?.trimmingCharacters(in: .whitespacesAndNewlines)
    let selectedWindowId = windows.contains(where: { $0.windowId == selected }) ? selected : windows.last?.windowId
    return HostedWorkspaceRestoreState(
        windows: windows,
        selectedWindowId: selectedWindowId?.isEmpty == false ? selectedWindowId : nil
    )
}

func handleAppleUIBridgeCommand(
    method: String,
    params: [String: BridgeJSONValue],
    forgeRuntime: ForgeRuntime,
    baseURL: String
) async throws -> [String: BridgeJSONValue] {
    switch method {
    case "ui.window.open":
        let windowKey = params["windowKey"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowKey.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let windowTitle = params["windowTitle"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty ?? windowKey
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        let parameters = params["parameters"]?.objectValue ?? [:]
        let options = params["options"]?.objectValue ?? [:]
        let presentation = options["presentation"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        let region = options["region"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        let parentKey = options["parentKey"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        let conversationID = options["conversationId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).nonEmpty
        let replaceHostedRegion = options["replaceHostedRegion"]?.boolValue == true

        if replaceHostedRegion, presentation?.lowercased() == "hosted", let region {
            let existingWindows = await forgeRuntime.windows
            for existing in existingWindows where
                existing.id != windowID &&
                existing.presentation?.lowercased() == "hosted" &&
                existing.region?.lowercased() == region.lowercased() &&
                existing.parentKey == parentKey &&
                existing.conversationID == conversationID {
                await forgeRuntime.closeWindow(id: existing.id)
            }
        }

        let state = await forgeRuntime.openWindow(
            key: windowKey,
            title: windowTitle,
            id: windowID,
            inTab: true,
            parameters: parameters.mapValues(\.forgeValue),
            conversationID: conversationID,
            presentation: presentation,
            region: region,
            workspaceSharePct: options["workspaceSharePct"]?.intValue,
            workspaceMinHeight: options["workspaceMinHeight"]?.intValue,
            parentKey: parentKey,
            isModal: false
        )
        return [
            "ok": .bool(true),
            "selectedWindowId": .string(state.id),
            "windowId": .string(state.id),
            "windowKey": .string(windowKey),
            "windowTitle": .string(windowTitle),
            "conversationId": conversationID.map(BridgeJSONValue.string) ?? .null,
            "presentation": presentation.map(BridgeJSONValue.string) ?? .null,
            "region": region.map(BridgeJSONValue.string) ?? .null,
            "parentKey": parentKey.map(BridgeJSONValue.string) ?? .null,
            "parameters": .object(parameters)
        ]

    case "ui.window.close":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        await forgeRuntime.closeWindow(id: windowID)
        return ["ok": .bool(true)]

    case "ui.window.activate":
        return ["ok": .bool(true)]

    case "ui.data.fetch":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let metadata = await forgeRuntime.windowMetadata(id: windowID)
        let refs = params["dataSourceRef"]?.stringValue?.nonEmpty.map { [$0] } ?? metadata.defaultDataSourceRefs
        for ref in refs {
            await forgeRuntime.refreshDataSourceCollection(
                windowID: windowID,
                dataSourceRef: ref,
                baseURL: baseURL
            )
        }
        return ["ok": .bool(true)]

    default:
        throw AgentlySDKError.invalidResponse
    }
}

private extension WindowMetadata? {
    var defaultDataSourceRefs: [String] {
        guard let self else { return [] }
        var refs: [String] = []
        for container in self.view?.content?.containers ?? [] {
            if let ref = container.dataSourceRef?.trimmingCharacters(in: .whitespacesAndNewlines), !ref.isEmpty {
                refs.append(ref)
            }
        }
        refs.append(contentsOf: self.dataSources.keys)
        return Array(Set(refs)).sorted()
    }
}

private extension BridgeJSONValue {
    var stringValue: String? {
        guard case .string(let value) = self else { return nil }
        return value
    }

    var objectValue: [String: BridgeJSONValue]? {
        guard case .object(let value) = self else { return nil }
        return value
    }

    var boolValue: Bool? {
        guard case .bool(let value) = self else { return nil }
        return value
    }

    var intValue: Int? {
        guard case .number(let value) = self else { return nil }
        return Int(value)
    }

    var arrayValue: [BridgeJSONValue]? {
        guard case .array(let value) = self else { return nil }
        return value
    }
}

private func normalizeBridgeHostedWorkspaceWindow(_ raw: [String: BridgeJSONValue]?) -> WorkspaceWindowSnapshot? {
    guard let raw else { return nil }
    let presentation = raw["presentation"]?.stringValue?.lowercased() ?? ""
    let region = raw["region"]?.stringValue?.lowercased() ?? ""
    let parentKey = raw["parentKey"]?.stringValue ?? ""
    let windowID = raw["windowId"]?.stringValue ?? ""
    let windowKey = raw["windowKey"]?.stringValue ?? ""
    guard !windowID.isEmpty, !windowKey.isEmpty else { return nil }
    guard presentation == "hosted", region == "chat.top", parentKey == "chat/new" else { return nil }
    return WorkspaceWindowSnapshot(
        windowId: windowID,
        conversationId: raw["conversationId"]?.stringValue,
        windowKey: windowKey,
        windowTitle: raw["windowTitle"]?.stringValue?.isEmpty == false ? raw["windowTitle"]?.stringValue : windowKey,
        presentation: raw["presentation"]?.stringValue,
        region: raw["region"]?.stringValue,
        parentKey: parentKey,
        inTab: true,
        parameters: raw["parameters"]?.objectValue,
        windowForm: raw["windowForm"]?.objectValue
    )
}

private extension String {
    var nonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
