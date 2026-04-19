import Foundation
import AgentlySDK
import Dispatch
import OSLog
import Combine

@MainActor
public final class AppRuntime: ObservableObject {
    private let logger = Logger(subsystem: "com.viant.agently.ios", category: "AppRuntime")
    @Published public var state: AppState
    @Published public var authRuntime: AuthRuntime
    @Published public var chatRuntime = ChatRuntime()
    @Published public var composerRuntime = ComposerRuntime()
    @Published public var queryRuntime: QueryRuntime
    @Published public var approvalRuntime: ApprovalRuntime
    @Published public var elicitationRuntime: ElicitationRuntime
    @Published public var settingsRuntime: SettingsRuntime

    private let settingsStore: AppSettingsStore
    private let clientFactory: @Sendable (String) -> AgentlyClient
    private let streamTracker = ConversationStreamTracker()
    private var streamTask: Task<Void, Never>?
    private var bootstrapTimeoutTask: Task<Void, Never>?
    private var observationCancellables: Set<AnyCancellable> = []

    public init(
        client: AgentlyClient,
        startupBaseURL: String,
        settingsStore: AppSettingsStore = AppSettingsStore(),
        clientFactory: @escaping @Sendable (String) -> AgentlyClient
    ) {
        let state = AppState(client: client, bootstrapBaseURL: startupBaseURL)
        self.state = state
        self.settingsStore = settingsStore
        self.clientFactory = clientFactory
        self.settingsRuntime = SettingsRuntime(store: settingsStore)
        self.authRuntime = AuthRuntime(client: client)
        self.queryRuntime = QueryRuntime(client: client)
        self.approvalRuntime = ApprovalRuntime(client: client)
        self.elicitationRuntime = ElicitationRuntime(client: client)
        bindChildObjectChanges()
    }

    private func bindChildObjectChanges() {
        observationCancellables.removeAll()

        let publishers: [ObservableObjectPublisher] = [
            state.objectWillChange,
            authRuntime.objectWillChange,
            chatRuntime.objectWillChange,
            composerRuntime.objectWillChange,
            queryRuntime.objectWillChange,
            approvalRuntime.objectWillChange,
            elicitationRuntime.objectWillChange,
            settingsRuntime.objectWillChange
        ]

        for publisher in publishers {
            publisher
                .sink { [weak self] _ in
                    self?.objectWillChange.send()
                }
                .store(in: &observationCancellables)
        }
    }

    public func bootstrap() async {
        logger.info("Bootstrap started for base URL: \(self.displayBaseURL, privacy: .public)")
        bootstrapTimeoutTask?.cancel()
        state.authState = .checking
        state.bootstrapErrorMessage = nil
        state.isRefreshingConversations = true
        bootstrapTimeoutTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(10))
            guard let self, !Task.isCancelled, self.state.authState == .checking else { return }
            self.state.bootstrapErrorMessage = "Timed out reaching \(self.displayBaseURL). Check the API base URL or confirm the backend is running."
            self.state.authState = .connectionFailed
        }
        do {
            state.workspaceMetadata = try await state.client.getWorkspaceMetadata(state.metadataTargetContext)
            state.conversations = try await state.client.listConversations().rows
            reconcilePreferredAgentSelection()
            logger.info("Bootstrap succeeded with \(self.state.conversations.count, privacy: .public) conversations")
            bootstrapTimeoutTask?.cancel()
            state.authState = .signedIn
            await authRuntime.refreshConnectionContext(expectSignedIn: true)
            let restoredConversationID = settingsStore.loadActiveConversationID()
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let selectedConversationID: String?
            if !restoredConversationID.isEmpty,
               state.conversations.contains(where: { $0.id == restoredConversationID }) {
                selectedConversationID = restoredConversationID
            } else {
                selectedConversationID = state.conversations.first?.id
            }
            if let selectedConversationID {
                await selectConversation(selectedConversationID)
            } else {
                settingsStore.saveActiveConversationID(nil)
            }
        } catch {
            logger.error("Bootstrap failed: \(String(describing: error), privacy: .public)")
            bootstrapTimeoutTask?.cancel()
            state.workspaceMetadata = nil
            state.conversations = []
            state.activeConversationID = nil
            state.activeTurnID = nil
            state.artifacts = []
            state.selectedArtifact = nil
            state.artifactErrorMessage = nil
            state.streamErrorMessage = nil
            state.isStoppingTurn = false
            settingsStore.saveActiveConversationID(nil)
            state.bootstrapErrorMessage = bootstrapErrorMessage(for: error)
            let authRequired = isAuthenticationError(error)
            state.authState = authRequired ? .required : .connectionFailed
            if authRequired {
                await authRuntime.refreshConnectionContext()
            }
        }
        state.isRefreshingConversations = false
    }

    public func applySettingsAndReload() async {
        logger.info("Applying settings and rebuilding runtime client")
        settingsRuntime.save()
        rebuildClient()
        streamTask?.cancel()
        chatRuntime.transcript = []
        state.conversations = []
        state.activeConversationID = nil
        state.activeTurnID = nil
        state.artifacts = []
        state.selectedArtifact = nil
        state.artifactErrorMessage = nil
        state.streamErrorMessage = nil
        state.isStoppingTurn = false
        state.isRefreshingConversations = false
        state.isLoadingConversation = false
        state.isLoadingArtifacts = false
        settingsStore.saveActiveConversationID(nil)
        await bootstrap()
    }

    public func selectConversation(_ conversationID: String) async {
        guard !conversationID.isEmpty else { return }
        logger.info("Selecting conversation \(conversationID, privacy: .public)")
        streamTask?.cancel()
        state.activeConversationID = conversationID
        state.activeTurnID = nil
        state.streamErrorMessage = nil
        state.isStoppingTurn = false
        state.selectedArtifact = nil
        state.artifacts = []
        state.artifactErrorMessage = nil
        chatRuntime.transcript = []
        settingsStore.saveActiveConversationID(conversationID)
        await loadConversationState(conversationID: conversationID)
        startStreaming(conversationID: conversationID)
    }

    public func startNewConversation() {
        streamTask?.cancel()
        state.activeConversationID = nil
        state.activeTurnID = nil
        state.selectedArtifact = nil
        state.artifacts = []
        state.artifactErrorMessage = nil
        state.streamErrorMessage = nil
        state.isStoppingTurn = false
        state.isLoadingConversation = false
        state.isLoadingArtifacts = false
        chatRuntime.transcript = []
        composerRuntime.query = ""
        composerRuntime.clearAttachments()
        approvalRuntime.approvals = []
        approvalRuntime.decidingApprovalID = nil
        approvalRuntime.lastError = nil
        elicitationRuntime.pending = nil
        elicitationRuntime.isResolving = false
        elicitationRuntime.lastError = nil
        settingsStore.saveActiveConversationID(nil)
    }

    public func refreshConversationList() async {
        state.isRefreshingConversations = true
        do {
            state.conversations = try await state.client.listConversations().rows
            logger.info("Conversation list refreshed with \(self.state.conversations.count, privacy: .public) rows")
            state.bootstrapErrorMessage = nil
            if let activeConversationID = state.activeConversationID,
               !state.conversations.contains(where: { $0.id == activeConversationID }) {
                startNewConversation()
            }
        } catch {
            logger.error("Conversation list refresh failed: \(String(describing: error), privacy: .public)")
            state.bootstrapErrorMessage = error.localizedDescription
        }
        state.isRefreshingConversations = false
    }

    public func sendCurrentQuery() async {
        let text = composerRuntime.query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        logger.info("Sending query with \(self.composerRuntime.attachments.count, privacy: .public) attachment(s)")
        state.streamErrorMessage = nil
        do {
            let conversationID = try await ensureConversationForOutgoingMessage()
            let attachments = try await uploadDraftAttachments(conversationID: conversationID)
            let optimisticTurn = chatRuntime.beginOptimisticTurn(query: text)
            if let response = await queryRuntime.send(
                conversationID: conversationID,
                agentID: selectedAgentID,
                query: text,
                attachments: attachments
            ) {
                chatRuntime.markOptimisticTurnAccepted(optimisticTurn)
                if let conversationID = response.conversationID, !conversationID.isEmpty {
                    state.activeConversationID = conversationID
                    settingsStore.saveActiveConversationID(conversationID)
                    await refreshConversationList()
                    await loadConversationState(conversationID: conversationID)
                    startStreaming(conversationID: conversationID)
                } else {
                    chatRuntime.completeOptimisticTurn(optimisticTurn, response: response.content)
                }
                composerRuntime.query = ""
                composerRuntime.clearAttachments()
            } else {
                logger.error("Query send failed before response: \(self.queryRuntime.lastError ?? "unknown error", privacy: .public)")
                chatRuntime.failOptimisticTurn(optimisticTurn, errorMessage: queryRuntime.lastError)
            }
        } catch {
            logger.error("Query send threw error: \(String(describing: error), privacy: .public)")
            queryRuntime.lastError = error.localizedDescription
        }
    }

    public func cancelActiveTurn() async {
        guard let activeTurnID = state.activeTurnID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !activeTurnID.isEmpty,
              !state.isStoppingTurn else {
            return
        }

        state.isStoppingTurn = true
        state.streamErrorMessage = nil
        do {
            try await state.client.cancelTurn(id: activeTurnID)
            logger.info("Cancelled active turn \(activeTurnID, privacy: .public)")
            if let conversationID = state.activeConversationID, !conversationID.isEmpty {
                await loadConversationState(conversationID: conversationID)
            }
        } catch {
            logger.error("Cancel turn failed: \(String(describing: error), privacy: .public)")
            state.streamErrorMessage = error.localizedDescription
            state.isStoppingTurn = false
        }
    }

    public func retryLiveUpdates() async {
        guard let conversationID = state.activeConversationID?.trimmingCharacters(in: .whitespacesAndNewlines),
              !conversationID.isEmpty else {
            return
        }

        state.streamErrorMessage = nil
        await loadConversationState(conversationID: conversationID)
        startStreaming(conversationID: conversationID)
    }

    public func decideApproval(
        _ approval: PendingToolApproval,
        action: String,
        editedFields: [String: JSONValue] = [:]
    ) async {
        await approvalRuntime.decide(id: approval.id, action: action,
                                     editedFields: editedFields)
        let conversationID = approval.conversationID ?? state.activeConversationID
        if let conversationID, !conversationID.isEmpty {
            await loadConversationState(conversationID: conversationID)
            startStreaming(conversationID: conversationID)
        } else {
            await approvalRuntime.refresh(conversationID: state.activeConversationID)
        }
    }

    public func resolvePendingElicitation(
        action: String,
        payload: [String: JSONValue] = [:]
    ) async {
        guard let pending = elicitationRuntime.pending,
              let conversationID = pending.conversationID,
              !conversationID.isEmpty else {
            return
        }
        await elicitationRuntime.resolve(
            conversationID: conversationID,
            elicitationID: pending.elicitationID,
            action: action,
            data: payload
        )
        await loadConversationState(conversationID: conversationID)
        startStreaming(conversationID: conversationID)
    }

    public func selectArtifact(_ artifact: ArtifactPreview?) {
        state.selectedArtifact = artifact
        guard let artifact else { return }
        guard artifact.text == nil else { return }
        Task {
            await hydrateArtifactPreviewIfNeeded(artifact)
        }
    }

    private func loadConversationState(conversationID: String) async {
        state.isLoadingConversation = true
        do {
            let transcriptState = try await state.client.getTranscript(
                GetTranscriptInput(
                    conversationID: conversationID,
                    includeModelCalls: true,
                    includeToolCalls: true,
                    includeFeeds: true
                )
            )
            chatRuntime.replaceTranscript(from: transcriptState)
            state.streamErrorMessage = nil
            logger.info("Loaded transcript state for conversation \(conversationID, privacy: .public)")
        } catch {
            logger.error("Transcript load failed for conversation \(conversationID, privacy: .public): \(String(describing: error), privacy: .public)")
            state.streamErrorMessage = "Failed to load conversation state. \(error.localizedDescription)"
        }
        await loadArtifacts(conversationID: conversationID)
        await approvalRuntime.refresh(conversationID: conversationID)
        await elicitationRuntime.refresh(conversationID: conversationID)
        state.isLoadingConversation = false
    }

    private func loadArtifacts(conversationID: String) async {
        state.isLoadingArtifacts = true
        do {
            async let generatedFilesTask = state.client.listGeneratedFiles(conversationID: conversationID)
            async let fileListTask = state.client.listFiles(ListFilesInput(conversationID: conversationID))
            let generatedFiles = try await generatedFilesTask
            let listedFiles = try await fileListTask
            state.artifacts = mergeArtifacts(
                conversationID: conversationID,
                generatedFiles: generatedFiles,
                listedFiles: listedFiles.files
            )
            state.artifactErrorMessage = nil
            if let selectedArtifact = state.selectedArtifact,
               !state.artifacts.contains(where: { $0.id == selectedArtifact.id }) {
                state.selectedArtifact = nil
            }
        } catch {
            logger.error("Artifact load failed for conversation \(conversationID, privacy: .public): \(String(describing: error), privacy: .public)")
            state.artifacts = []
            state.selectedArtifact = nil
            state.artifactErrorMessage = error.localizedDescription
        }
        state.isLoadingArtifacts = false
    }

    private func startStreaming(conversationID: String) {
        streamTask?.cancel()
        state.activeTurnID = nil
        state.streamErrorMessage = nil
        state.isStoppingTurn = false
        streamTask = Task { [weak self] in
            guard let self else { return }
            logger.info("Starting live stream for conversation \(conversationID, privacy: .public)")
            await streamTracker.reset(conversationID: conversationID)
            do {
                for try await event in state.client.streamEvents(conversationID: conversationID) {
                    if Task.isCancelled { return }
                    let snapshot = await streamTracker.apply(event)
                    guard state.activeConversationID == conversationID else {
                        continue
                    }
                    state.activeTurnID = snapshot.activeTurnID
                    if snapshot.activeTurnID == nil {
                        state.isStoppingTurn = false
                    }
                    chatRuntime.applyStreaming(snapshot: snapshot)
                    if let pending = snapshot.pendingElicitation {
                        elicitationRuntime.pending = pending
                    } else {
                        elicitationRuntime.dismiss()
                    }
                }
                state.activeTurnID = nil
                state.isStoppingTurn = false
                logger.info("Live stream ended for conversation \(conversationID, privacy: .public)")
            } catch {
                state.activeTurnID = nil
                state.isStoppingTurn = false
                guard !Task.isCancelled else { return }
                logger.error("Live stream failed for conversation \(conversationID, privacy: .public): \(String(describing: error), privacy: .public)")
                state.streamErrorMessage = "Live updates failed: \(error.localizedDescription)"
            }
        }
    }

    public var isQueryBusy: Bool {
        queryRuntime.isSending || state.activeTurnID != nil || state.isStoppingTurn
    }

    public var availableAgentOptions: [WorkspaceAgentOption] {
        guard let metadata = state.workspaceMetadata else { return [] }

        var options: [WorkspaceAgentOption] = metadata.agentInfos.compactMap { info in
            let id = (info.agentID ?? info.name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !id.isEmpty else { return nil }
            let displayName = (info.name ?? info.agentID ?? id).trimmingCharacters(in: .whitespacesAndNewlines)
            let model = info.modelRef?.trimmingCharacters(in: .whitespacesAndNewlines)
            return WorkspaceAgentOption(id: id, displayName: displayName.isEmpty ? id : displayName, modelRef: model?.isEmpty == true ? nil : model)
        }

        for rawAgent in metadata.agents {
            let trimmed = rawAgent.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { continue }
            if options.contains(where: { $0.id == trimmed }) { continue }
            options.append(WorkspaceAgentOption(id: trimmed, displayName: trimmed, modelRef: nil))
        }

        return options
    }

    public var selectedAgentOption: WorkspaceAgentOption? {
        guard let selectedAgentID else { return nil }
        return availableAgentOptions.first(where: { $0.id == selectedAgentID })
    }

    public func selectPreferredAgent(_ agentID: String?) {
        let trimmed = agentID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        settingsRuntime.preferredAgentID = trimmed
        settingsRuntime.save()
    }

    private var selectedAgentID: String? {
        let preferred = settingsRuntime.preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines)
        if !preferred.isEmpty,
           availableAgentOptions.contains(where: { $0.id == preferred }) {
            return preferred
        }
        return state.workspaceMetadata?.defaultAgent
    }

    private func reconcilePreferredAgentSelection() {
        let available = availableAgentOptions
        let current = settingsRuntime.preferredAgentID.trimmingCharacters(in: .whitespacesAndNewlines)

        if !current.isEmpty, available.contains(where: { $0.id == current }) {
            return
        }

        let fallback = state.workspaceMetadata?.defaultAgent?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !fallback.isEmpty, available.contains(where: { $0.id == fallback }) {
            settingsRuntime.preferredAgentID = fallback
        } else {
            settingsRuntime.preferredAgentID = ""
        }
        settingsRuntime.save()
    }

    private func rebuildClient() {
        let configuredBaseURL = settingsRuntime.apiBaseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        logger.info("Rebuilding runtime client for base URL: \(configuredBaseURL, privacy: .public)")
        let client = clientFactory(configuredBaseURL)
        state.client = client
        state.bootstrapBaseURL = configuredBaseURL
        Task {
            await state.forgeRuntime.configureWindowMetadata(baseURL: URL(string: configuredBaseURL))
        }
        authRuntime = AuthRuntime(client: client)
        queryRuntime = QueryRuntime(client: client)
        approvalRuntime = ApprovalRuntime(client: client)
        elicitationRuntime = ElicitationRuntime(client: client)
    }

    private func ensureConversationForOutgoingMessage() async throws -> String? {
        if let activeConversationID = state.activeConversationID, !activeConversationID.isEmpty {
            return activeConversationID
        }
        guard !composerRuntime.attachments.isEmpty else {
            return nil
        }
        let conversation = try await state.client.createConversation(
            CreateConversationInput(agentID: selectedAgentID)
        )
        state.activeConversationID = conversation.id
        settingsStore.saveActiveConversationID(conversation.id)
        if let index = state.conversations.firstIndex(where: { $0.id == conversation.id }) {
            state.conversations[index] = conversation
        } else {
            state.conversations.insert(conversation, at: 0)
        }
        return conversation.id
    }

    private func uploadDraftAttachments(conversationID: String?) async throws -> [QueryAttachment] {
        guard !composerRuntime.attachments.isEmpty else { return [] }
        guard let conversationID, !conversationID.isEmpty else {
            throw NSError(
                domain: "AgentlyAppRuntime",
                code: 1,
                userInfo: [NSLocalizedDescriptionKey: "Attachments require a conversation before sending."]
            )
        }

        var uploaded: [QueryAttachment] = []
        for attachment in composerRuntime.attachments {
            let output = try await state.client.uploadFile(
                UploadFileInput(
                    conversationID: conversationID,
                    name: attachment.name,
                    contentType: attachment.mimeType,
                    data: attachment.data
                )
            )
            uploaded.append(
                QueryAttachment(
                    name: attachment.name,
                    uri: output.uri,
                    size: Int64(attachment.data.count),
                    mime: attachment.mimeType
                )
            )
        }
        return uploaded
    }

    private func mergeArtifacts(
        conversationID: String,
        generatedFiles: [GeneratedFileEntry],
        listedFiles: [FileEntry]
    ) -> [ArtifactPreview] {
        let generatedPreviews = generatedFiles.map { entry in
            ArtifactPreview(
                id: "generated:\(entry.id)",
                name: entry.filename?.nonEmpty ?? "Generated File",
                contentType: entry.mimeType,
                uri: nil,
                localFilePath: nil,
                conversationID: conversationID,
                generatedFileID: entry.id,
                sourceLabel: "Generated artifact",
                text: entry.messageID.map { "Created from assistant message \($0)." }
            )
        }

        let uploadedPreviews = listedFiles.map { entry in
            ArtifactPreview(
                id: "file:\(entry.id)",
                name: entry.name?.nonEmpty ?? entry.uri?.nonEmpty ?? "Conversation File",
                contentType: entry.contentType,
                uri: entry.uri,
                localFilePath: nil,
                conversationID: conversationID,
                fileID: entry.id,
                sourceLabel: "Conversation file",
                text: nil
            )
        }

        return (generatedPreviews + uploadedPreviews).sorted { lhs, rhs in
            lhs.name.localizedCaseInsensitiveCompare(rhs.name) == .orderedAscending
        }
    }

    private func hydrateArtifactPreviewIfNeeded(_ artifact: ArtifactPreview) async {
        do {
            let hydrated = try await downloadPreview(for: artifact)
            if state.selectedArtifact?.id == artifact.id {
                state.selectedArtifact = hydrated
            }
            if let index = state.artifacts.firstIndex(where: { $0.id == artifact.id }) {
                state.artifacts[index] = hydrated
            }
            state.artifactErrorMessage = nil
        } catch {
            state.artifactErrorMessage = error.localizedDescription
        }
    }

    private func downloadPreview(for artifact: ArtifactPreview) async throws -> ArtifactPreview {
        let downloaded: DownloadFileOutput
        if let generatedFileID = artifact.generatedFileID, !generatedFileID.isEmpty {
            downloaded = try await state.client.downloadGeneratedFile(id: generatedFileID)
        } else if let conversationID = artifact.conversationID,
                  !conversationID.isEmpty,
                  let fileID = artifact.fileID,
                  !fileID.isEmpty {
            downloaded = try await state.client.downloadFile(conversationID: conversationID, fileID: fileID)
        } else {
            return artifact
        }

        let resolvedName = downloaded.name?.nonEmpty ?? artifact.name
        let resolvedType = downloaded.contentType?.nonEmpty ?? artifact.contentType
        let localFilePath = persistDownloadedArtifact(
            data: downloaded.data,
            name: resolvedName,
            contentType: resolvedType
        )
        let previewText: String?
        if Self.isPreviewableText(contentType: resolvedType, name: resolvedName) {
            previewText = String(data: downloaded.data, encoding: .utf8) ?? "This text artifact could not be decoded as UTF-8."
        } else {
            previewText = "Binary artifact downloaded. Use the share action above to open or export it."
        }

        return ArtifactPreview(
            id: artifact.id,
            name: resolvedName,
            contentType: resolvedType,
            uri: artifact.uri,
            localFilePath: localFilePath,
            conversationID: artifact.conversationID,
            generatedFileID: artifact.generatedFileID,
            fileID: artifact.fileID,
            sourceLabel: artifact.sourceLabel,
            text: previewText
        )
    }

    private func persistDownloadedArtifact(data: Data, name: String, contentType: String?) -> String? {
        let sanitizedName = sanitizeArtifactFilename(name, contentType: contentType)
        let directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("agently-artifacts", isDirectory: true)
        do {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            let fileURL = directory.appendingPathComponent("\(UUID().uuidString)-\(sanitizedName)")
            try data.write(to: fileURL, options: .atomic)
            return fileURL.path
        } catch {
            return nil
        }
    }

    private func sanitizeArtifactFilename(_ rawName: String, contentType: String?) -> String {
        let trimmed = rawName.trimmingCharacters(in: .whitespacesAndNewlines)
        let base = trimmed.isEmpty ? "artifact" : trimmed
        let cleaned = base
            .replacingOccurrences(of: "/", with: "-")
            .replacingOccurrences(of: ":", with: "-")
        if cleaned.contains(".") {
            return cleaned
        }

        let lowerType = contentType?.lowercased() ?? ""
        if lowerType.contains("json") { return cleaned + ".json" }
        if lowerType.contains("markdown") { return cleaned + ".md" }
        if lowerType.starts(with: "text/") { return cleaned + ".txt" }
        if lowerType.contains("png") { return cleaned + ".png" }
        if lowerType.contains("jpeg") || lowerType.contains("jpg") { return cleaned + ".jpg" }
        if lowerType.contains("pdf") { return cleaned + ".pdf" }
        return cleaned
    }

    private static func isPreviewableText(contentType: String?, name: String?) -> Bool {
        let normalizedType = contentType?.lowercased() ?? ""
        let normalizedName = name?.lowercased() ?? ""
        return normalizedType.starts(with: "text/") ||
            normalizedType.contains("json") ||
            normalizedType.contains("xml") ||
            normalizedType.contains("javascript") ||
            normalizedName.hasSuffix(".md") ||
            normalizedName.hasSuffix(".txt") ||
            normalizedName.hasSuffix(".json") ||
            normalizedName.hasSuffix(".yaml") ||
            normalizedName.hasSuffix(".yml") ||
            normalizedName.hasSuffix(".xml") ||
            normalizedName.hasSuffix(".csv")
    }

    private func isAuthenticationError(_ error: Error) -> Bool {
        if case AgentlySDKError.httpStatus(let statusCode, _) = error {
            return statusCode == 401 || statusCode == 403
        }
        return false
    }

    private func bootstrapErrorMessage(for error: Error) -> String {
        if let sdkError = error as? AgentlySDKError {
            switch sdkError {
            case .httpStatus(let statusCode, _):
                if statusCode == 401 || statusCode == 403 {
                    return "The server is reachable, but this device still needs sign-in."
                }
            default:
                break
            }
        }
        if let urlError = error as? URLError {
            switch urlError.code {
            case .timedOut:
                return "Timed out reaching \(displayBaseURL). Check the API base URL or confirm the backend is running."
            case .cannotConnectToHost, .cannotFindHost, .networkConnectionLost, .dnsLookupFailed, .notConnectedToInternet:
                return "Could not connect to \(displayBaseURL). Check the API base URL or confirm the backend is running."
            default:
                break
            }
        }
        return error.localizedDescription
    }

    private var displayBaseURL: String {
        let baseURL = state.bootstrapBaseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        return baseURL.isEmpty ? "the configured server" : baseURL
    }

    deinit {
        bootstrapTimeoutTask?.cancel()
        streamTask?.cancel()
    }
}

public struct WorkspaceAgentOption: Identifiable, Hashable {
    public let id: String
    public let displayName: String
    public let modelRef: String?
}

private extension String {
    var nonEmpty: String? {
        let trimmed = trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
