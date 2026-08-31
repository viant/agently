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
    let metadata: [String: BridgeJSONValue]
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
        case metadata
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
        let rpcClient = self.rpcClient
        let clientID = clientIDValue
        let handler = commandHandler
        let finalizeCommand: @MainActor @Sendable (
            String,
            [String: BridgeJSONValue],
            [String: BridgeJSONValue]
        ) async -> Void = { [weak self] method, params, result in
            guard let self else { return }
            self.updateSelectedWindowID(method: method, params: params, result: result)
            await self.publishSnapshotNow()
        }
        pollTask = Task.detached(priority: .userInitiated) {
            while !Task.isCancelled {
                do {
                    _ = try await rpcClient.hello(clientID: clientID)
                    guard let result = try await rpcClient.poll(clientID: clientID, timeoutMs: 20_000),
                          case .object(let params) = result["params"] ?? .null else {
                        continue
                    }
                    let commandID = params["id"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                    let method = params["method"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                    guard !commandID.isEmpty, !method.isEmpty else { continue }
                    let commandParams = params["params"]?.objectValue ?? [:]
                    do {
                        let commandResult = try await handler(method, commandParams)
                        _ = try await rpcClient.respond(
                            commandID: commandID,
                            ok: true,
                            result: .object(commandResult)
                        )
                        await finalizeCommand(method, commandParams, commandResult)
                    } catch {
                        _ = try? await rpcClient.respond(
                            commandID: commandID,
                            ok: false,
                            error: error.localizedDescription
                        )
                    }
                } catch {
                    await rpcClient.resetSession()
                    try? await Task.sleep(nanoseconds: 1_000_000_000)
                }
            }
        }
        snapshotTask = Task {
            await self.runSnapshotLoop()
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

    func publishSnapshotNow() async {
        do {
            _ = try await ensureConnectedRPC()
            try await publishSnapshot(force: true)
        } catch {
            await rpcClient.resetSession()
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

    private func publishSnapshot(force: Bool, requireAck: Bool = false) async throws {
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
        guard let snapshotValue = try decoder.decode(BridgeJSONValue.self, from: fingerprintData).objectValue else {
            return
        }
        let result = try await rpcClient.snapshot(clientID: clientIDValue, data: BridgeJSONValue.object(snapshotValue))
        if requireAck, result == nil {
            throw AgentlySDKError.invalidResponse
        }
        guard result != nil else {
            return
        }
        lastSnapshotFingerprint = fingerprint
    }

    private static func loadOrCreateClientID(defaults: UserDefaults = .standard) -> String {
        let launchOverride = CommandLine.arguments.first { $0.hasPrefix("--uiBridgeClientID=") }?
            .dropFirst("--uiBridgeClientID=".count)
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !launchOverride.isEmpty {
            return launchOverride
        }
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
                metadata: [:],
                inTab: true,
                isModal: false
            )
        )
    }
    let runtimeWindows = await forgeRuntime.windows
    for window in runtimeWindows where !conversationID.isEmpty {
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
                metadata: appleUIBridgeMetadata(await forgeRuntime.windowMetadata(id: window.id)),
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

private func appleUIBridgeMetadata(
    _ metadata: WindowMetadata?
) -> [String: BridgeJSONValue] {
    guard let metadata,
          let data = try? JSONEncoder().encode(metadata),
          let value = try? JSONDecoder.agently().decode(BridgeJSONValue.self, from: data) else {
        return [:]
    }
    return value.objectValue ?? [:]
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
            workspaceSharePct: window.workspaceSharePct,
            workspaceMinHeight: window.workspaceMinHeight,
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
        let windowForm = await forgeRuntime.windowFormJSONValue(windowID: state.id).mapValues(\.appValue)
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
            "parameters": .object(parameters),
            "windowForm": .object(windowForm)
        ]

    case "ui.window.close":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        await forgeRuntime.closeWindow(id: windowID)
        await AppleFeedCanonicalRegistry.shared.clear(forgeRuntime: forgeRuntime, windowID: windowID)
        return ["ok": .bool(true)]

    case "ui.window.setFormData":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let existingWindows = await forgeRuntime.windows
        guard let existingWindow = existingWindows.first(where: { $0.id == windowID }) else {
            throw AgentlySDKError.invalidResponse
        }
        guard let values = params["values"]?.objectValue ?? params["parameters"]?.objectValue,
              !values.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let replace = params["replace"]?.boolValue == true
        let nextValues = mergeBridgeJSONObjects(
            base: existingWindow.parameters,
            override: values.mapValues(\.forgeValue)
        )
        await forgeRuntime.setWindowFormValue(
            windowID: windowID,
            values: nextValues,
            replace: replace
        )
        let windowForm = await forgeRuntime.windowFormJSONValue(windowID: windowID).mapValues(\.appValue)
        return [
            "ok": .bool(true),
            "windowId": .string(windowID),
            "windowForm": .object(windowForm)
        ]

    case "ui.window.activate":
        return ["ok": .bool(true)]

    case "ui.report.getCurrent":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AppleUIBridgeReportError.missingWindowID
        }
        let form = await forgeRuntime.windowFormJSONValue(windowID: windowID)
        return appleReportCurrentResult(windowID: windowID, form: form)

    case "ui.report.run":
        let windowID = params["windowId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !windowID.isEmpty else {
            throw AppleUIBridgeReportError.missingWindowID
        }
        guard await forgeRuntime.windowState(id: windowID) != nil else {
            throw AppleUIBridgeReportError.windowNotFound(windowID)
        }
        let requestID = "native-\(UUID().uuidString)"
        await forgeRuntime.setWindowFormValue(
            windowID: windowID,
            values: [
                "reportRunRequest": .object([
                    "id": .string(requestID),
                    "origin": .string("ui.report.run")
                ])
            ],
            replace: false
        )
        return [
            "ok": .bool(true),
            "windowId": .string(windowID),
            "accepted": .bool(true),
            "materialized": .bool(false),
            "materializationId": .string(requestID),
            "status": .string("running")
        ]

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

    case "ui.feed.get":
        let feedID = params["feedId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let conversationID = params["conversationId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !feedID.isEmpty, !conversationID.isEmpty,
              let refs = params["dataSourceRefs"]?.arrayValue?.compactMap({ $0.stringValue?.nonEmpty }),
              !refs.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let window = try await findAppleFeedWindow(
            forgeRuntime: forgeRuntime,
            feedID: feedID,
            conversationID: conversationID
        )
        let snapshots = try await forgeRuntime.snapshotFeedDataSources(
            windowID: window.id,
            dataSourceRefs: refs
        )
        let dataSources = snapshots.mapValues { snapshot in
            BridgeJSONValue.object([
                "form": .object(snapshot.form.mapValues(\.appValue)),
                "collection": .array(snapshot.collection.map { .object($0.mapValues(\.appValue)) }),
                "selection": .object(snapshot.selection.mapValues(\.appValue))
            ])
        }
        return [
            "conversationId": .string(conversationID),
            "feedId": .string(feedID),
            "dataSources": .object(dataSources)
        ]

    case "ui.feed.update":
        let feedID = params["feedId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let conversationID = params["conversationId"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        guard !feedID.isEmpty, !conversationID.isEmpty,
              let rawOperations = params["operations"]?.arrayValue,
              !rawOperations.isEmpty else {
            throw AgentlySDKError.invalidResponse
        }
        let operations = try rawOperations.enumerated().map { index, value -> ForgeIOSRuntime.FeedPatchOperation in
            guard let object = value.objectValue,
                  let dataSourceRef = object["dataSourceRef"]?.stringValue?.nonEmpty,
                  let op = object["op"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased().nonEmpty,
                  let path = object["path"]?.stringValue else {
                throw AgentlySDKError.invalidResponse
            }
            _ = index
            return ForgeIOSRuntime.FeedPatchOperation(
                dataSourceRef: dataSourceRef,
                op: op,
                path: path,
                value: object["value"]?.forgeValue
            )
        }
        let window = try await findAppleFeedWindow(
            forgeRuntime: forgeRuntime,
            feedID: feedID,
            conversationID: conversationID
        )
        let changed: Set<String>
        if await AppleFeedCanonicalRegistry.shared.contains(forgeRuntime: forgeRuntime, windowID: window.id) {
            changed = try await AppleFeedCanonicalRegistry.shared.apply(
                forgeRuntime: forgeRuntime,
                windowID: window.id,
                operations: operations,
                turnID: params["turnId"]?.stringValue
            )
        } else {
            changed = try await forgeRuntime.applyFeedPatchOperations(
                windowID: window.id,
                operations: operations
            )
        }
        return [
            "ok": .bool(true),
            "feedId": .string(feedID),
            "changedDataSourceRefs": .array(changed.sorted().map(BridgeJSONValue.string))
        ]

    default:
        throw AgentlySDKError.invalidResponse
    }
}

private func findAppleFeedWindow(
    forgeRuntime: ForgeRuntime,
    feedID: String,
    conversationID: String
) async throws -> ForgeRuntime.WindowState {
    let expectedKey = "feed-\(feedID)-\(conversationID)"
    guard let window = await forgeRuntime.windows.first(where: {
        $0.key == expectedKey && $0.conversationID == conversationID
    }) else {
        throw AgentlySDKError.invalidResponse
    }
    return window
}

private enum AppleUIBridgeReportError: LocalizedError {
    case missingWindowID
    case windowNotFound(String)
    case reportNotReady
    case runFailed(String)
    case timedOut

    var errorDescription: String? {
        switch self {
        case .missingWindowID:
            return "windowId is required"
        case .windowNotFound(let id):
            return "report window not found: \(id)"
        case .reportNotReady:
            return "The current report is not ready to run."
        case .runFailed(let message):
            return message.isEmpty ? "The native report run failed." : message
        case .timedOut:
            return "The native report run did not finish in time."
        }
    }
}

private func appleReportCurrentResult(
    windowID: String,
    form: [String: ForgeIOSRuntime.JSONValue]
) -> [String: BridgeJSONValue] {
    let definition = form["reportDefinition"]?.objectValue
    let document = definition?["documentPatch"]?.objectValue
        ?? definition?["reportDocument"]?.objectValue
        ?? form["documentPatch"]?.objectValue
        ?? form["reportDocument"]?.objectValue
    let canRun = !(document?["blocks"]?.arrayValue ?? []).isEmpty
    let materialization = form["reportMaterialization"]?.objectValue
    let status = materialization?["status"]?.stringValue?.lowercased() ?? ""
    return [
        "ok": .bool(true),
        "windowId": .string(windowID),
        "reportId": definition?["id"]?.appValue ?? .null,
        "reportName": document?["title"]?.appValue ?? .null,
        "canRun": .bool(canRun),
        "canSave": .bool(false),
        "hasCompletedRun": .bool(status == "completed"),
        "materialization": materialization.map { .object($0.mapValues(\.appValue)) } ?? .null
    ]
}

private func waitForAppleReportMaterialization(
    windowID: String,
    requestID: String,
    forgeRuntime: ForgeRuntime
) async throws -> [String: BridgeJSONValue] {
    let deadline = Date().addingTimeInterval(300)
    while Date() < deadline {
        try Task.checkCancellation()
        let form = await forgeRuntime.windowFormJSONValue(windowID: windowID)
        guard let materialization = form["reportMaterialization"]?.objectValue,
              materialization["requestId"]?.stringValue == requestID else {
            try await Task.sleep(nanoseconds: 100_000_000)
            continue
        }
        let status = materialization["status"]?.stringValue?.lowercased() ?? ""
        if status == "completed" {
            let referenced = appleReportReferencedDatasetRefs(form)
            let materialized = Set(
                (materialization["datasetRefs"]?.arrayValue ?? [])
                    .compactMap(\.stringValue)
            )
            let missing = referenced.subtracting(materialized).sorted()
            if !missing.isEmpty {
                throw AppleUIBridgeReportError.runFailed(
                    "Report run did not materialize referenced datasets: \(missing.joined(separator: ", "))"
                )
            }
            return [
                "ok": .bool(true),
                "windowId": .string(windowID),
                "materialized": .bool(true),
                "materializationId": .string(requestID),
                "status": .string(status),
                "datasetRefs": materialization["datasetRefs"]?.appValue ?? .array([]),
                "rowCounts": materialization["rowCounts"]?.appValue ?? .object([:])
            ]
        }
        if status == "failed" {
            let message = materialization["errors"]?.arrayValue?
                .compactMap(\.stringValue)
                .joined(separator: "; ") ?? ""
            throw AppleUIBridgeReportError.runFailed(message)
        }
        try await Task.sleep(nanoseconds: 100_000_000)
    }
    throw AppleUIBridgeReportError.timedOut
}

private func appleReportReferencedDatasetRefs(
    _ form: [String: ForgeIOSRuntime.JSONValue]
) -> Set<String> {
    let definition = form["reportDefinition"]?.objectValue
    let document = definition?["documentPatch"]?.objectValue
        ?? definition?["reportDocument"]?.objectValue
    return Set(
        (document?["blocks"]?.arrayValue ?? []).compactMap { block in
            block.objectValue?["datasetRef"]?.stringValue?.trimmingCharacters(in: .whitespacesAndNewlines)
        }.filter { !$0.isEmpty }
    )
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

private func mergeBridgeJSONObjects(
    base: [String: ForgeIOSRuntime.JSONValue],
    override: [String: ForgeIOSRuntime.JSONValue]
) -> [String: ForgeIOSRuntime.JSONValue] {
    var merged = base
    for (key, value) in override {
        if case .object(let baseObject)? = merged[key],
           case .object(let overrideObject) = value {
            merged[key] = .object(mergeBridgeJSONObjects(base: baseObject, override: overrideObject))
        } else {
            merged[key] = value
        }
    }
    return merged
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
        workspaceSharePct: raw["workspaceSharePct"]?.intValue,
        workspaceMinHeight: raw["workspaceMinHeight"]?.intValue,
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
