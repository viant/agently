import SwiftUI
import ForgeIOSRuntime
import ForgeIOSUI
import AgentlySDK
#if os(iOS) && canImport(QuickLook)
import QuickLook
#endif

typealias TranscriptContentPart = TranscriptEnvelopePart
typealias ForgeUIPayload = TranscriptForgeUIPayload
typealias MaterializedForgeDataBlock = TranscriptForgeDataStore

struct TranscriptMessageContent: View {
    let markdown: String
    let renderedParts: [TranscriptCanonicalPart]?
    let renderedReports: [TranscriptCanonicalReport]?
    let diagnosticMessages: [String]
	let client: AgentlyClient?
    let conversationID: String?
    @State private var artifactLinkError: String?
    #if os(iOS) && canImport(QuickLook)
    @State private var artifactLinkPreviewURL: URL?
    #endif

    var body: some View {
        let sourceParts = renderedParts
            .map(TranscriptEnvelope.fromCanonical)
            ?? parseTranscriptContentParts(markdown)
        let parts = renderedReports?.isEmpty == false
            ? sourceParts.map { part in
                if case .markdown(let text) = part {
                    return .markdown(TranscriptEnvelope.suppressProgressiveTransport(in: text))
                }
                return part
            }
            : sourceParts
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(parts.enumerated()), id: \.offset) { _, part in
                switch part {
                case .markdown(let text):
                    if !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        MarkdownRenderer(markdown: text).textSelection(.enabled)
                    }
                case .forgeUI(let payload, let dataStore):
                    TranscriptForgeUIView(payload: payload, dataStore: dataStore)
                }
            }
            ForEach(renderedReports ?? [], id: \.stableIdentity) { report in
                TranscriptInlineReportView(report: report, client: client, conversationID: conversationID)
            }
            if let artifactLinkError, !artifactLinkError.isEmpty {
                Text(artifactLinkError)
                    .font(.caption)
                    .foregroundStyle(.red)
            }
            if !diagnosticMessages.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    Label("Report assembled with diagnostics", systemImage: "exclamationmark.triangle.fill")
                        .font(.footnote.weight(.semibold))
                    ForEach(Array(diagnosticMessages.enumerated()), id: \.offset) { _, message in
                        Text(message).font(.caption)
                    }
                }
                .foregroundStyle(.orange)
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.orange.opacity(0.10), in: RoundedRectangle(cornerRadius: 10))
            }
        }
        .environment(\.openURL, OpenURLAction { url in
            guard let artifactID = reportArtifactID(from: url) else {
                return .systemAction
            }
            guard let client else {
                artifactLinkError = "Sign in before opening this report."
                return .discarded
            }
            Task {
                do {
                    let artifact = try await downloadReportRuntimeArtifact(
                        client: client,
                        artifactID: artifactID,
                        conversationID: conversationID ?? ""
                    )
                    let fileURL = try persistInlineReportExportArtifact(artifact)
                    await MainActor.run {
                        artifactLinkError = nil
                        #if os(iOS) && canImport(QuickLook)
                        artifactLinkPreviewURL = fileURL
                        #endif
                    }
                } catch {
                    await MainActor.run {
                        artifactLinkError = reportRuntimeExportErrorMessage(error)
                    }
                }
            }
            return .handled
        })
        #if os(iOS) && canImport(QuickLook)
        .quickLookPreview($artifactLinkPreviewURL)
        #endif
    }
}

internal func reportArtifactID(from url: URL) -> String? {
    if url.scheme?.lowercased() == "scratchpad", url.host?.lowercased() == "artifact" {
        let value = url.pathComponents.last?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return value.isEmpty ? nil : value
    }
    if url.path.contains("/__report_artifact__/") {
        let value = url.pathComponents.last?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return value.isEmpty ? nil : value
    }
    return nil
}

private extension TranscriptCanonicalReport {
    var stableIdentity: String {
        "\(scope):\(id):\(resetVersion)"
    }

    var refreshIdentity: String {
        "\(stableIdentity):\(sequence ?? 0):\(status)"
    }
}

internal func isPendingInlineReport(_ report: TranscriptCanonicalReport) -> Bool {
    ["rendering", "pending", "incomplete", "building"].contains(
        report.status.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    )
}

internal func inlineReportBuildStatus(_ report: TranscriptCanonicalReport) -> String {
    let dataSourceCount = report.dataSources.count
    let blockCount = report.source.objectValue?["blocks"]?.arrayValue?.count ?? 0
    var parts = ["Building report"]
    if dataSourceCount > 0 {
        parts.append("\(dataSourceCount) \(dataSourceCount == 1 ? "data source" : "data sources")")
    }
    if blockCount > 0 {
        parts.append("\(blockCount) \(blockCount == 1 ? "block" : "blocks")")
    }
    return parts.joined(separator: " · ")
}

private struct TranscriptInlineReportView: View {
    let report: TranscriptCanonicalReport
	let client: AgentlyClient?
    let conversationID: String?

    @State private var metadata: WindowMetadata?
    @State private var windowContext: WindowContext?
    @State private var errorMessage: String?
    @State private var exportErrorMessage: String?
    @State private var isExportingPDF = false
    @State private var isReportPresented = false
    #if os(iOS) && canImport(QuickLook)
    @State private var quickLookURL: URL?
    #endif
    @State private var runtime = ForgeRuntime()

    var body: some View {
        inlineReportPreviewCard
        .task(id: report.refreshIdentity) {
            if isPendingInlineReport(report) {
                metadata = nil
                windowContext = nil
                errorMessage = nil
                isReportPresented = false
                return
            }
            do {
				if let client {
                    let scopedConversationID = conversationID
                    await runtime.registerDataSourceLoader(
                        makeForgeAgentlyDataSourceLoader(
                            client: client,
                            conversationIDProvider: { scopedConversationID }
                        )
                    )
                }
				let hydrated = try await hydrateInlineReport(
                    report,
                    client: client,
                    conversationID: conversationID
                )
                let artifact = try InlineReportRuntimeCompiler.compile(hydrated)
                let state = await runtime.openWindowInline(
                    key: "inline-report-\(report.scope)-\(report.id)",
                    title: artifact.reportSpec.objectValue?["title"]?.stringValue ?? report.id,
                    metadata: artifact.metadata
                )
                metadata = artifact.metadata
                windowContext = await runtime.windowContext(id: state.id)
                errorMessage = nil
            } catch {
                metadata = nil
                windowContext = nil
                errorMessage = error.localizedDescription
            }
        }
        .transcriptReportPresentation(isPresented: $isReportPresented) {
            inlineReportDestination
        }
        #if os(iOS) && canImport(QuickLook)
        .quickLookPreview($quickLookURL)
        #endif
    }

    private var previewTitle: String {
        let title = report.source.objectValue?["title"]?.stringValue?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return title.isEmpty ? report.id : title
    }

    private var previewSubtitle: String? {
        let subtitle = report.source.objectValue?["subtitle"]?.stringValue?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return subtitle.isEmpty ? nil : subtitle
    }

    private var destinationNavigation: [String: ForgeIOSRuntime.JSONValue] {
        report.source.objectValue?["navigation"]?.objectValue ?? [:]
    }

    private var destinationTitle: String {
        let label = destinationNavigation["label"]?.stringValue?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return label.isEmpty ? previewTitle : label
    }

    private var destinationSupportingText: String {
        let detail = destinationNavigation["supportingText"]?.stringValue?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return detail.isEmpty ? "Open \(destinationTitle)." : detail
    }

    private var inlineReportPreviewCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(previewTitle)
                .font(.headline.weight(.semibold))
            if let previewSubtitle {
                Text(previewSubtitle)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if isPendingInlineReport(report) {
                HStack(spacing: 8) {
                    ProgressView()
                    Text(inlineReportBuildStatus(report))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            } else if metadata != nil, windowContext != nil {
                VStack(alignment: .leading, spacing: 8) {
                    AssistantDestinationLink(
                        title: destinationTitle,
                        supportingText: destinationSupportingText,
                        systemImage: assistantDestinationSystemImage(destinationNavigation["icon"]?.stringValue),
                        accessibilityIdentifier: "agently-open-report",
                        onOpen: { isReportPresented = true }
                    )
                    inlineReportExportButton
                }
                exportErrorText
            } else {
                HStack(spacing: 8) {
                    ProgressView()
                    Text(errorMessage ?? "Loading report…")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.06), in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(Color.secondary.opacity(0.10)))
    }

    @ViewBuilder
    private var inlineReportDestination: some View {
        NavigationStack {
            Group {
                if let metadata, let windowContext {
                    WindowContentView(
                        runtime: runtime,
                        window: windowContext,
                        metadata: metadata,
                        scrollEnabled: true,
                        contentPadding: 12
                    )
                    .environment(\.forgePresentationDensity, .compact)
                    .environment(\.forgeDedicatedReportScreen, true)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    ProgressView("Loading report…")
                }
            }
            .navigationTitle(previewTitle)
            .agentlyInlineTitleMode()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button { isReportPresented = false } label: {
                        AppleToolbarActionIcon(
                            systemImage: "chevron.left",
                            color: Color(red: 0.35, green: 0.40, blue: 0.85)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Back to conversation")
                }
                ToolbarItem(placement: .primaryAction) {
                    inlineReportExportButton
                }
            }
            .safeAreaInset(edge: .bottom) {
                exportErrorText
                    .padding(.horizontal, 12)
            }
        }
    }

    @ViewBuilder
    private var inlineReportExportButton: some View {
        if report.status.trimmingCharacters(in: .whitespacesAndNewlines).lowercased().isEmpty ||
            ["committed", "ready"].contains(report.status.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()) {
            Button {
                exportInlineReportPDF()
            } label: {
                AppleToolbarActionIcon(
                    systemImage: "doc.richtext.fill",
                    color: Color(red: 0.82, green: 0.25, blue: 0.34),
                    isLoading: isExportingPDF
                )
            }
                .accessibilityLabel(isExportingPDF ? "Preparing PDF" : "Open PDF")
                .accessibilityIdentifier("forge-report-runtime-open-pdf")
                .buttonStyle(.plain)
                .disabled(isExportingPDF)

        }
    }

    @ViewBuilder
    private var exportErrorText: some View {
        if let exportErrorMessage, !exportErrorMessage.isEmpty {
            Text(exportErrorMessage)
                .font(.caption)
                .foregroundStyle(.red)
                .lineLimit(3)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func exportInlineReportPDF() {
        guard !isExportingPDF else { return }
        guard let client else {
            exportErrorMessage = "Sign in before opening the report PDF."
            return
        }
        isExportingPDF = true
        exportErrorMessage = nil
        Task {
            do {
                let hydrated = try await hydrateInlineReport(
                    report,
                    client: client,
                    conversationID: conversationID
                )
                let artifact = try InlineReportRuntimeCompiler.compile(hydrated)
                let title = artifact.reportSpec.objectValue?["title"]?.stringValue ?? report.id
                let exportRequest: [String: ForgeIOSRuntime.JSONValue] = [
                    "title": .string(title),
                    "artifactRef": .string("report://inline/\(report.scope)/\(report.id)"),
                    "reportId": .string(report.id),
                    "fences": .array(try InlineReportRuntimeCompiler.exportFences(hydrated))
                ]
                let exported = try await exportReportRuntimePDF(
                    client: client,
                    exportRequest: exportRequest
                )
                let fileURL = try persistInlineReportExportArtifact(exported)
                await MainActor.run {
                    isExportingPDF = false
                    #if os(iOS) && canImport(QuickLook)
                    quickLookURL = fileURL
                    #endif
                }
            } catch {
                await MainActor.run {
                    isExportingPDF = false
                    exportErrorMessage = error.localizedDescription
                }
            }
        }
    }
}

private extension View {
    @ViewBuilder
    func transcriptReportPresentation<Content: View>(
        isPresented: Binding<Bool>,
        @ViewBuilder content: @escaping () -> Content
    ) -> some View {
        #if os(iOS)
        fullScreenCover(isPresented: isPresented, content: content)
        #else
        sheet(isPresented: isPresented, content: content)
        #endif
    }
}

private func persistInlineReportExportArtifact(_ artifact: ReportRuntimeExportArtifact) throws -> URL {
    let directory = FileManager.default.temporaryDirectory
        .appendingPathComponent("agently-inline-reports", isDirectory: true)
    try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    let fileURL = directory.appendingPathComponent("\(UUID().uuidString)-\(artifact.name)")
    try artifact.data.write(to: fileURL, options: .atomic)
    return fileURL
}

private func hydrateInlineReport(
	_ report: TranscriptCanonicalReport,
	client: AgentlyClient?,
    conversationID: String?
) async throws -> TranscriptCanonicalReport {
	let requests = InlineReportRuntimeCompiler.workspaceDatasetRequests(report)
	guard !requests.isEmpty, let client else { return report }
	let trimmedConversationID = conversationID?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
	var dataSources = report.dataSources
	for request in requests {
		let inputData = try JSONEncoder().encode(request.inputs)
		let inputs = try JSONDecoder().decode([String: AgentlySDK.JSONValue].self, from: inputData)
		let output = try await client.fetchDatasource(FetchDatasourceInput(
			id: request.dataSourceRef,
			inputs: inputs,
            conversationId: trimmedConversationID.isEmpty ? nil : trimmedConversationID
		))
		let rowsData = try JSONEncoder().encode(output.rows)
		let rows = try JSONDecoder().decode(ForgeIOSRuntime.JSONValue.self, from: rowsData)
		dataSources[request.id] = TranscriptCanonicalData(
			version: 2,
			reportRef: report.id,
			id: request.id,
			format: "json",
			mode: "replace",
			payload: rows
		)
	}
	return TranscriptCanonicalReport(
		scope: report.scope,
		id: report.id,
		grammar: report.grammar,
		status: report.status,
		sequence: report.sequence,
		resetVersion: report.resetVersion,
		source: report.source,
		dataSources: dataSources
	)
}

private struct TranscriptForgeUIView: View {
    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    let payload: ForgeUIPayload
    let dataStore: [String: MaterializedForgeDataBlock]

    @State private var windowContext: WindowContext?
    @State private var windowID: String?
    @State private var presentationError: String?
    @State private var runtime = ForgeRuntime()

    var body: some View {
        let presentation = try? TranscriptWindowBuilder.presentation(payload: payload, dataStore: dataStore)
        Group {
            if let presentation, let windowContext {
                WindowContentView(
                    runtime: runtime,
                    window: windowContext,
                    metadata: presentation.metadata,
                    scrollEnabled: true,
                    contentPadding: 4
                )
                .environment(\.forgePresentationDensity, .compact)
                .frame(maxHeight: TranscriptInlinePresentationPolicy.resolve(
                    metadata: presentation.metadata,
                    horizontalSizeClass: horizontalSizeClass
                ).maximumHeight)
                .clipped()
            } else if let presentationError {
                unavailableView(message: presentationError)
            } else {
                loadingView
            }
        }
        .task(id: renderTaskKey(for: presentation?.dataStore ?? dataStore)) {
            presentationError = nil
            guard let presentation else {
                windowContext = nil
                presentationError = "This interactive response has no supported Forge content."
                return
            }
            await openInlineWindow(presentation)
        }
    }

    private var loadingView: some View {
        unavailableView(message: "Loading interactive content…")
    }

    private func unavailableView(message: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if let title = payload.title, !title.isEmpty {
                Text(title).font(.headline)
            }
            Text(message)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(Color.secondary.opacity(0.08), in: RoundedRectangle(cornerRadius: 12))
    }

    private func renderTaskKey(for store: [String: MaterializedForgeDataBlock]) -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys]
        let dataFingerprint = store.keys.sorted().map { key in
            let rows = store[key]?.rows ?? .null
            let encoded = (try? encoder.encode(rows)).flatMap { String(data: $0, encoding: .utf8) } ?? "null"
            return "\(key):\(encoded)"
        }.joined(separator: "|")
        let payloadFingerprint = (try? encoder.encode(payload)).flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
        return "\(payloadFingerprint):\(dataFingerprint)"
    }

    private func openInlineWindow(_ presentation: TranscriptWindowPresentation) async {
        let title = payload.title ?? "Forge content"
        let state: ForgeRuntime.WindowState
        if let windowID,
           let updated = await runtime.updateWindowInline(id: windowID, title: title, metadata: presentation.metadata) {
            state = updated
        } else {
            state = await runtime.openWindowInline(
                key: "transcript-\(UUID().uuidString)",
                title: title,
                metadata: presentation.metadata
            )
        }
        windowID = state.id
        await hydrateTranscriptDataSources(windowID: state.id, dataStore: presentation.dataStore)
        windowContext = await runtime.windowContext(id: state.id)
    }

    private func hydrateTranscriptDataSources(windowID: String, dataStore: [String: MaterializedForgeDataBlock]) async {
        for (dataSourceRef, block) in dataStore {
            let rows = TranscriptEnvelope.rows(from: block.rows)
            await runtime.setDataSourceCollection(windowID: windowID, dataSourceRef: dataSourceRef, rows: rows)
            if rows.isEmpty {
                await runtime.setDataSourceMetrics(windowID: windowID, dataSourceRef: dataSourceRef, values: [:])
                await runtime.setDataSourceSelection(windowID: windowID, dataSourceRef: dataSourceRef, selected: nil)
                await runtime.setDataSourceForm(windowID: windowID, dataSourceRef: dataSourceRef, values: [:])
            } else if rows.count == 1, let first = rows.first {
                await runtime.setDataSourceMetrics(windowID: windowID, dataSourceRef: dataSourceRef, values: first)
            }
        }
    }
}

func parseTranscriptContentParts(_ markdown: String) -> [TranscriptContentPart] {
    TranscriptEnvelope.parse(markdown)
}


/// Compatibility seam for app tests. Generic adaptation belongs to Forge.
func buildTranscriptForgeWindowMetadata(
    payload: ForgeUIPayload,
    dataStore: [String: MaterializedForgeDataBlock]
) throws -> WindowMetadata {
    try TranscriptWindowBuilder.presentation(payload: payload, dataStore: dataStore).metadata
}
